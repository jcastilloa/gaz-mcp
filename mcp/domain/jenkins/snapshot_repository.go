package jenkins

import "context"

// SnapshotType represents the kind of object being snapshotted.
type SnapshotType string

const (
	SnapshotJob        SnapshotType = "job"
	SnapshotView       SnapshotType = "view"
	SnapshotNode       SnapshotType = "node"
	SnapshotCredential SnapshotType = "credential"
	SnapshotFolder     SnapshotType = "folder"
)

// SnapshotOperation describes what triggered the snapshot.
type SnapshotOperation string

const (
	OpCreated       SnapshotOperation = "created"
	OpUpdated       SnapshotOperation = "updated"
	OpDeleted       SnapshotOperation = "deleted"
	OpCopied        SnapshotOperation = "copied"
	OpRestored      SnapshotOperation = "restored"
	OpRestoreSafety SnapshotOperation = "restore-safety"
)

// SnapshotInfo holds metadata about a stored snapshot.
type SnapshotInfo struct {
	ID          int64             `json:"id"`
	Environment string            `json:"environment"`
	ObjectType  SnapshotType      `json:"object_type"`
	ObjectName  string            `json:"object_name"`
	Version     int               `json:"version"`
	Operation   SnapshotOperation `json:"operation"`
	Checksum    string            `json:"checksum"`
	CreatedAt   string            `json:"created_at"`
}

// SnapshotRepository defines the contract for storing and retrieving snapshots.
type SnapshotRepository interface {
	// Snapshot stores the state of an object before a write operation.
	// Returns the version number assigned.
	Snapshot(ctx context.Context, env string, objType SnapshotType, objName, configXML string, operation SnapshotOperation) (int, error)

	// ListSnapshots returns metadata for stored snapshots, most recent first.
	ListSnapshots(ctx context.Context, env string, objType SnapshotType, objName string, limit, offset int) ([]SnapshotInfo, error)

	// GetSnapshot returns the full config XML for a specific version.
	GetSnapshot(ctx context.Context, env string, objType SnapshotType, objName string, version int) (string, error)

	// LatestSnapshot returns the most recent snapshot info and config.
	LatestSnapshot(ctx context.Context, env string, objType SnapshotType, objName string) (*SnapshotInfo, string, error)

	// Prune removes all but the most recent N versions for a given object.
	// Returns the number of deleted entries.
	Prune(ctx context.Context, env string, objType SnapshotType, objName string, keep int) (int, error)

	// Count returns total snapshots for a given object.
	Count(ctx context.Context, env string, objType SnapshotType, objName string) (int, error)

	// Close closes the underlying database connection.
	Close() error
}
