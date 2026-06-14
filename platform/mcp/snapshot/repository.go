// Package snapshot implements the domain.SnapshotRepository interface using SQLite.
// It uses modernc.org/sqlite (pure Go, no CGo required).
package snapshot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // register "sqlite" driver

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/jenkins"
)

const schema = `
CREATE TABLE IF NOT EXISTS object_snapshots (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    environment  TEXT    NOT NULL,
    object_type  TEXT    NOT NULL,
    object_name  TEXT    NOT NULL,
    version      INTEGER NOT NULL,
    config_xml   TEXT    NOT NULL,
    operation    TEXT    NOT NULL,
    checksum     TEXT    NOT NULL,
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_snapshots_lookup
    ON object_snapshots (environment, object_type, object_name, version);

CREATE INDEX IF NOT EXISTS idx_snapshots_checksum
    ON object_snapshots (environment, object_type, object_name, checksum);
`

// Repository implements domain.SnapshotRepository backed by SQLite.
type Repository struct {
	db *sql.DB
}

// newRepositoryWithDB creates a Repository from an existing *sql.DB.
// Used in tests to inject a pre-configured or deliberately broken DB.
func newRepositoryWithDB(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// applySchema executes the DDL schema on db. Extracted for testability.
func applySchema(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("apply snapshot schema: %w", err)
	}
	return nil
}

// NewRepository opens (or creates) the SQLite database at dbPath and applies the schema.
func NewRepository(dbPath string) (*Repository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("create snapshot db dir: %w", err)
	}

	// modernc/sqlite's sql.Open never returns an error (defers to first use).
	db, _ := sql.Open("sqlite", dbPath) //nolint:errcheck

	// SQLite performs best with a single writer connection.
	db.SetMaxOpenConns(1)

	if err := applySchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Repository{db: db}, nil
}

// Close closes the underlying database connection.
func (r *Repository) Close() error {
	return r.db.Close()
}

// Snapshot stores the state of an object. It deduplicates by checksum — if the
// latest snapshot for this object already has the same content, it is a no-op
// and returns the existing version number.
func (r *Repository) Snapshot(ctx context.Context, env string, objType domain.SnapshotType, objName, configXML string, operation domain.SnapshotOperation) (int, error) {
	checksum := sha256Hex(configXML)

	// Dedup: skip if latest snapshot has identical content
	latest, _, err := r.LatestSnapshot(ctx, env, objType, objName)
	if err == nil && latest != nil && latest.Checksum == checksum {
		return latest.Version, nil
	}

	// Determine next version number
	var maxVersion sql.NullInt64
	row := r.db.QueryRowContext(ctx,
		`SELECT MAX(version) FROM object_snapshots WHERE environment=? AND object_type=? AND object_name=?`,
		env, string(objType), objName,
	)
	if err := row.Scan(&maxVersion); err != nil {
		return 0, fmt.Errorf("get max version: %w", err)
	}

	nextVersion := 1
	if maxVersion.Valid {
		nextVersion = int(maxVersion.Int64) + 1
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO object_snapshots (environment, object_type, object_name, version, config_xml, operation, checksum)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		env, string(objType), objName, nextVersion, configXML, string(operation), checksum,
	)
	if err != nil {
		return 0, fmt.Errorf("insert snapshot: %w", err)
	}

	return nextVersion, nil
}

// ListSnapshots returns snapshot metadata, most recent first.
func (r *Repository) ListSnapshots(ctx context.Context, env string, objType domain.SnapshotType, objName string, limit, offset int) ([]domain.SnapshotInfo, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, environment, object_type, object_name, version, operation, checksum, created_at
		 FROM object_snapshots
		 WHERE environment=? AND object_type=? AND object_name=?
		 ORDER BY version DESC
		 LIMIT ? OFFSET ?`,
		env, string(objType), objName, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()

	var result []domain.SnapshotInfo
	for rows.Next() {
		var s domain.SnapshotInfo
		var objTypeStr, opStr string
		if err := rows.Scan(&s.ID, &s.Environment, &objTypeStr, &s.ObjectName, &s.Version, &opStr, &s.Checksum, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan snapshot row: %w", err)
		}
		s.ObjectType = domain.SnapshotType(objTypeStr)
		s.Operation = domain.SnapshotOperation(opStr)
		result = append(result, s)
	}

	return result, nil
}

// GetSnapshot returns the full config XML for a specific version.
func (r *Repository) GetSnapshot(ctx context.Context, env string, objType domain.SnapshotType, objName string, version int) (string, error) {
	var configXML string
	err := r.db.QueryRowContext(ctx,
		`SELECT config_xml FROM object_snapshots
		 WHERE environment=? AND object_type=? AND object_name=? AND version=?`,
		env, string(objType), objName, version,
	).Scan(&configXML)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("snapshot not found: env=%s type=%s name=%s version=%d", env, objType, objName, version)
	}
	if err != nil {
		return "", fmt.Errorf("get snapshot: %w", err)
	}
	return configXML, nil
}

// LatestSnapshot returns the most recent snapshot info and config XML.
func (r *Repository) LatestSnapshot(ctx context.Context, env string, objType domain.SnapshotType, objName string) (*domain.SnapshotInfo, string, error) {
	var s domain.SnapshotInfo
	var configXML string
	var objTypeStr, opStr string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, environment, object_type, object_name, version, operation, checksum, created_at, config_xml
		 FROM object_snapshots
		 WHERE environment=? AND object_type=? AND object_name=?
		 ORDER BY version DESC
		 LIMIT 1`,
		env, string(objType), objName,
	).Scan(&s.ID, &s.Environment, &objTypeStr, &s.ObjectName, &s.Version, &opStr, &s.Checksum, &s.CreatedAt, &configXML)

	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("latest snapshot: %w", err)
	}

	s.ObjectType = domain.SnapshotType(objTypeStr)
	s.Operation = domain.SnapshotOperation(opStr)
	return &s, configXML, nil
}

// Prune removes all but the most recent `keep` versions for a given object.
// Returns the number of deleted rows.
func (r *Repository) Prune(ctx context.Context, env string, objType domain.SnapshotType, objName string, keep int) (int, error) {
	if keep <= 0 {
		return 0, nil
	}

	result, err := r.db.ExecContext(ctx,
		`DELETE FROM object_snapshots
		 WHERE environment=? AND object_type=? AND object_name=?
		   AND version NOT IN (
		       SELECT version FROM object_snapshots
		       WHERE environment=? AND object_type=? AND object_name=?
		       ORDER BY version DESC
		       LIMIT ?
		   )`,
		env, string(objType), objName,
		env, string(objType), objName, keep,
	)
	if err != nil {
		return 0, fmt.Errorf("prune snapshots: %w", err)
	}

	// RowsAffected never errors with modernc/sqlite.
	deleted, _ := result.RowsAffected()
	return int(deleted), nil
}

// Count returns the total number of snapshots for a given object.
func (r *Repository) Count(ctx context.Context, env string, objType domain.SnapshotType, objName string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM object_snapshots WHERE environment=? AND object_type=? AND object_name=?`,
		env, string(objType), objName,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count snapshots: %w", err)
	}
	return count, nil
}

// sha256Hex returns the hex-encoded SHA-256 hash of the input string.
func sha256Hex(data string) string {
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h)
}

// Ensure Repository implements the interface at compile time.
var _ domain.SnapshotRepository = (*Repository)(nil)
