package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

func TestProfileStoreSavesAndLoadsCapturedProfile(t *testing.T) {
	store := NewProfileStore(t.TempDir())
	metadata, err := store.Save(context.Background(), domain.CapturedProfile{
		Metadata: domain.ProfileMetadata{Target: "http://service:6060", Type: "heap", CapturedAt: time.Now().UTC()},
		Data:     []byte("profile data"),
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load(context.Background(), metadata.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if string(loaded.Data) != "profile data" || loaded.Metadata.ID != metadata.ID {
		t.Fatalf("loaded = %#v", loaded)
	}
}

func TestProfileStoreRejectsPathTraversal(t *testing.T) {
	_, err := NewProfileStore(t.TempDir()).Load(context.Background(), "../profile")
	if err == nil {
		t.Fatal("expected invalid profile ID error")
	}
}

func TestProfileStoreRemovesDataWhenMetadataWriteFails(t *testing.T) {
	directory := t.TempDir()
	store := NewProfileStore(directory)
	store.writeMetadata = func(string, []byte) error { return os.ErrPermission }
	if _, err := store.Save(context.Background(), domain.CapturedProfile{Data: []byte("profile data")}); err == nil {
		t.Fatal("save should fail")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".pprof" {
			t.Fatalf("orphaned profile data %q", entry.Name())
		}
	}
}

func TestProfileStoreTightensExistingDirectoryPermissions(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "profiles")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewProfileStore(directory)
	if _, err := store.Save(context.Background(), domain.CapturedProfile{Data: []byte("profile data")}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", permissions)
	}
}
