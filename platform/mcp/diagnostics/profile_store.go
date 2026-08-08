package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

type ProfileStore struct {
	directory     string
	writeMetadata func(string, []byte) error
}

func NewProfileStore(directory string) *ProfileStore {
	return &ProfileStore{directory: directory, writeMetadata: writePrivateFile}
}

func DefaultProfileDirectory() string {
	return filepath.Join(os.TempDir(), "gaz-mcp", "pprof")
}

func (s *ProfileStore) Save(_ context.Context, profile domain.CapturedProfile) (domain.ProfileMetadata, error) {
	if err := os.MkdirAll(s.directory, 0o700); err != nil {
		return domain.ProfileMetadata{}, fmt.Errorf("create profile directory: %w", err)
	}
	if err := os.Chmod(s.directory, 0o700); err != nil {
		return domain.ProfileMetadata{}, fmt.Errorf("secure profile directory: %w", err)
	}
	metadata := profile.Metadata
	metadata.ID = uuid.NewString()
	metadata.Size = len(profile.Data)
	dataPath := s.dataPath(metadata.ID)
	if err := writePrivateFile(dataPath, profile.Data); err != nil {
		return domain.ProfileMetadata{}, fmt.Errorf("write profile data: %w", err)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return domain.ProfileMetadata{}, fmt.Errorf("marshal profile metadata: %w", err)
	}
	writeMetadata := s.writeMetadata
	if writeMetadata == nil {
		writeMetadata = writePrivateFile
	}
	if err := writeMetadata(s.metadataPath(metadata.ID), encoded); err != nil {
		_ = os.Remove(dataPath)
		return domain.ProfileMetadata{}, fmt.Errorf("write profile metadata: %w", err)
	}
	return metadata, nil
}

func (s *ProfileStore) Load(_ context.Context, id string) (domain.CapturedProfile, error) {
	if strings.TrimSpace(id) == "" || filepath.Base(id) != id {
		return domain.CapturedProfile{}, fmt.Errorf("invalid profile ID")
	}
	metadataData, err := os.ReadFile(s.metadataPath(id))
	if err != nil {
		return domain.CapturedProfile{}, fmt.Errorf("read profile metadata: %w", err)
	}
	var metadata domain.ProfileMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		return domain.CapturedProfile{}, fmt.Errorf("parse profile metadata: %w", err)
	}
	data, err := os.ReadFile(s.dataPath(id))
	if err != nil {
		return domain.CapturedProfile{}, fmt.Errorf("read profile data: %w", err)
	}
	return domain.CapturedProfile{Metadata: metadata, Data: data}, nil
}

func (s *ProfileStore) dataPath(id string) string { return filepath.Join(s.directory, id+".pprof") }

func (s *ProfileStore) metadataPath(id string) string { return filepath.Join(s.directory, id+".json") }

func writePrivateFile(name string, data []byte) error {
	return os.WriteFile(name, data, 0o600)
}
