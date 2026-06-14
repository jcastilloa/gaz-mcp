// Package snapshot — white-box tests that need access to unexported helpers.
package snapshot

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/jenkins"
)

// ---------------------------------------------------------------------------
// applySchema — error path (closed DB)
// ---------------------------------------------------------------------------

func TestApplySchema_ClosedDB_ReturnsError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = db.Close()

	if err := applySchema(db); err == nil {
		t.Fatal("expected error from applySchema on closed DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// NewRepository — openDB error propagated (schema fails on closed DB)
// We test this via applySchema since sql.Open never fails with modernc/sqlite.
// The NewRepository error branch (L63-65) is covered by triggering applySchema
// to fail, which NewRepository calls internally.
// ---------------------------------------------------------------------------

// TestNewRepository_ApplySchemaError covers the applySchema error branch inside
// NewRepository (L69-72). We place a directory where the DB file should be,
// which causes SQLite to fail on the first Exec (cannot open a directory as DB).
func TestNewRepository_ApplySchemaError(t *testing.T) {
	dir := t.TempDir()

	// Create a sub-directory at the exact path where the DB file would be.
	// SQLite will open the path but fail on the first Exec because it's a dir.
	dbPath := dir + "/snap.db"
	if err := os.MkdirAll(dbPath, 0o700); err != nil {
		t.Fatalf("mkdir at db path: %v", err)
	}

	_, err := NewRepository(dbPath)
	if err == nil {
		t.Fatal("expected error when DB path is a directory, got nil")
	}
}

// ---------------------------------------------------------------------------
// Snapshot — ExecContext insert error via BEFORE INSERT trigger
// ---------------------------------------------------------------------------

func TestRepository_Snapshot_InsertError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := applySchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	// Add a BEFORE INSERT trigger that always raises an error.
	_, err = db.Exec(`
		CREATE TRIGGER block_insert
		BEFORE INSERT ON object_snapshots
		BEGIN
			SELECT RAISE(ABORT, 'insert blocked by trigger');
		END;
	`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	repo := newRepositoryWithDB(db)
	ctx := context.Background()

	_, insertErr := repo.Snapshot(ctx, "env", domain.SnapshotJob, "job", "<xml/>", domain.OpCreated)
	if insertErr == nil {
		t.Fatal("expected insert error from trigger, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListSnapshots — rows.Scan error
// We create a view that shadows the table and returns a blob where an integer
// is expected, forcing a scan type mismatch.
// ---------------------------------------------------------------------------

func TestRepository_ListSnapshots_ScanError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := applySchema(db); err != nil {
		t.Fatalf("schema: %v", err)
	}

	// Insert a valid row.
	_, err = db.Exec(`
		INSERT INTO object_snapshots
			(environment, object_type, object_name, version, config_xml, operation, checksum)
		VALUES ('env', 'job', 'my-job', 1, '<xml/>', 'created', 'abc123')`,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Drop the table and replace with a view that returns 'not-an-int' for the
	// id column (first column scanned into s.ID which is int64).
	// SQLite allows DROP TABLE even with data when we recreate as a view.
	// We use a different approach: create a second table with wrong types and
	// make the repo query it via a renamed view trick.
	//
	// Actually the cleanest approach: use a INSTEAD OF trigger on a view.
	// But ListSnapshots queries object_snapshots directly.
	//
	// Simplest: rename the real table, create a view with wrong column types.
	_, err = db.Exec(`ALTER TABLE object_snapshots RENAME TO object_snapshots_real`)
	if err != nil {
		t.Fatalf("rename table: %v", err)
	}

	// Create a view with the same name but returning 'bad' (text) for id (int64).
	_, err = db.Exec(`
		CREATE VIEW object_snapshots AS
		SELECT
			'not-an-int-id' AS id,
			environment,
			object_type,
			object_name,
			version,
			operation,
			checksum,
			created_at
		FROM object_snapshots_real
	`)
	if err != nil {
		t.Fatalf("create view: %v", err)
	}

	repo := newRepositoryWithDB(db)
	ctx := context.Background()

	_, scanErr := repo.ListSnapshots(ctx, "env", domain.SnapshotJob, "my-job", 10, 0)
	if scanErr == nil {
		t.Fatal("expected scan error for incompatible id type, got nil")
	}
}

// ---------------------------------------------------------------------------
// newRepositoryWithDB — verify the helper works correctly
// ---------------------------------------------------------------------------

func TestNewRepositoryWithDB_ClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = db.Close()

	repo := newRepositoryWithDB(db)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}

	// Any operation on the closed DB should fail.
	ctx := context.Background()
	_, _, repoErr := repo.LatestSnapshot(ctx, "env", domain.SnapshotJob, "job")
	if repoErr == nil {
		t.Fatal("expected error from closed DB, got nil")
	}
}
