package diagnostics

import (
	"context"
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
