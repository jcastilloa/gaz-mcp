// Package jenkins implements the use cases for the Jenkins MCP proxy.
// It wraps a Jenkins repository with automatic snapshot/versioning on write operations.
package jenkins

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/jenkins"
)

// Service wraps a Jenkins repository with snapshot/versioning capabilities.
type Service struct {
	jenkinsRepo  domain.Repository
	snapshotRepo domain.SnapshotRepository
	maxVersions  int
}

// NewService creates a new Jenkins service with optional snapshot support.
// Pass a NoopSnapshotRepository to disable versioning.
func NewService(jenkinsRepo domain.Repository, snapshotRepo domain.SnapshotRepository, maxVersions int) *Service {
	return &Service{
		jenkinsRepo:  jenkinsRepo,
		snapshotRepo: snapshotRepo,
		maxVersions:  maxVersions,
	}
}

// --- Snapshot helpers ---

func sha256Hex(data string) string {
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h)
}

// snapshotXML stores a config XML snapshot and optionally prunes old versions.
func (s *Service) snapshotXML(ctx context.Context, env string, objType domain.SnapshotType, objName, configXML string, op domain.SnapshotOperation) (int, error) {
	version, err := s.snapshotRepo.Snapshot(ctx, env, objType, objName, configXML, op)
	if err != nil {
		return 0, err
	}
	if s.maxVersions > 0 {
		// Fire-and-forget prune — best effort, non-blocking
		go func() {
			_, _ = s.snapshotRepo.Prune(context.Background(), env, objType, objName, s.maxVersions)
		}()
	}
	return version, nil
}

// getConfigXML retrieves the current XML config for any supported object type.
// Returns ("", nil) when the object type has no XML config (e.g. nodes via gojenkins).
func (s *Service) getConfigXML(ctx context.Context, objType domain.SnapshotType, objName string) (string, error) {
	switch objType {
	case domain.SnapshotJob, domain.SnapshotFolder:
		return s.jenkinsRepo.JobConfig(ctx, objName)
	default:
		// Nodes, views, credentials don't expose a GetConfig via our domain interface yet.
		return "", nil
	}
}

// setConfigXML applies a config XML to the appropriate object type.
func (s *Service) setConfigXML(ctx context.Context, objType domain.SnapshotType, objName, configXML string) error {
	switch objType {
	case domain.SnapshotJob, domain.SnapshotFolder:
		return s.jenkinsRepo.JobSetConfig(ctx, objName, configXML)
	default:
		return fmt.Errorf("restore not supported for object type %q", objType)
	}
}

// --- System ---

func (s *Service) Info(ctx context.Context) (*domain.JenkinsInfo, error) {
	return s.jenkinsRepo.Info(ctx)
}

func (s *Service) QuietDown(ctx context.Context) error {
	return s.jenkinsRepo.QuietDown(ctx)
}

func (s *Service) CancelQuietDown(ctx context.Context) error {
	return s.jenkinsRepo.CancelQuietDown(ctx)
}

// --- Jobs ---

func (s *Service) JobList(ctx context.Context, filter string) ([]domain.JobInfo, error) {
	return s.jenkinsRepo.JobList(ctx, filter)
}

func (s *Service) JobGet(ctx context.Context, name string) (*domain.JobInfo, error) {
	return s.jenkinsRepo.JobGet(ctx, name)
}

func (s *Service) JobConfig(ctx context.Context, name string) (string, error) {
	return s.jenkinsRepo.JobConfig(ctx, name)
}

// JobSetConfig updates a job's config XML, snapshotting the previous state first.
// Returns the snapshot version number (0 if no change was needed).
func (s *Service) JobSetConfig(ctx context.Context, env, name, configXML string) (int, error) {
	currentCfg, err := s.jenkinsRepo.JobConfig(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("get current config for snapshot: %w", err)
	}

	// Skip if content is identical (SHA-256 dedup)
	if sha256Hex(currentCfg) == sha256Hex(configXML) {
		return 0, nil
	}

	// Snapshot current state before applying change
	version, err := s.snapshotXML(ctx, env, domain.SnapshotJob, name, currentCfg, domain.OpUpdated)
	if err != nil {
		return 0, fmt.Errorf("snapshot before update: %w", err)
	}

	if err := s.jenkinsRepo.JobSetConfig(ctx, name, configXML); err != nil {
		return version, err
	}

	return version, nil
}

// JobCreate creates a new job and snapshots its initial config.
func (s *Service) JobCreate(ctx context.Context, env, name, configXML string) (int, error) {
	if err := s.jenkinsRepo.JobCreate(ctx, name, configXML); err != nil {
		return 0, err
	}
	version, err := s.snapshotXML(ctx, env, domain.SnapshotJob, name, configXML, domain.OpCreated)
	if err != nil {
		return 0, fmt.Errorf("snapshot after create: %w", err)
	}
	return version, nil
}

// JobCopy copies a job and snapshots the new copy's config.
func (s *Service) JobCopy(ctx context.Context, env, from, to string) (int, error) {
	cfg, err := s.jenkinsRepo.JobConfig(ctx, from)
	if err != nil {
		return 0, fmt.Errorf("get source config for snapshot: %w", err)
	}

	if err := s.jenkinsRepo.JobCopy(ctx, from, to); err != nil {
		return 0, err
	}

	version, err := s.snapshotXML(ctx, env, domain.SnapshotJob, to, cfg, domain.OpCopied)
	if err != nil {
		return 0, fmt.Errorf("snapshot after copy: %w", err)
	}
	return version, nil
}

// JobDelete snapshots the job config before deleting it.
func (s *Service) JobDelete(ctx context.Context, env, name string) (int, error) {
	cfg, err := s.jenkinsRepo.JobConfig(ctx, name)
	if err != nil {
		return 0, fmt.Errorf("get config for snapshot: %w", err)
	}

	version, err := s.snapshotXML(ctx, env, domain.SnapshotJob, name, cfg, domain.OpDeleted)
	if err != nil {
		return 0, fmt.Errorf("snapshot before delete: %w", err)
	}

	if err := s.jenkinsRepo.JobDelete(ctx, name); err != nil {
		return version, err
	}
	return version, nil
}

func (s *Service) JobEnable(ctx context.Context, name string) error {
	return s.jenkinsRepo.JobEnable(ctx, name)
}

func (s *Service) JobDisable(ctx context.Context, name string) error {
	return s.jenkinsRepo.JobDisable(ctx, name)
}

func (s *Service) JobBuild(ctx context.Context, name string, params map[string]string) (int64, error) {
	return s.jenkinsRepo.JobBuild(ctx, name, params)
}

// --- Builds ---

func (s *Service) BuildInfo(ctx context.Context, jobName string, buildNum int) (*domain.BuildInfo, error) {
	return s.jenkinsRepo.BuildInfo(ctx, jobName, buildNum)
}

func (s *Service) BuildLog(ctx context.Context, jobName string, buildNum int, startLine int) (string, int, error) {
	return s.jenkinsRepo.BuildLog(ctx, jobName, buildNum, startLine)
}

func (s *Service) BuildLogProgressive(ctx context.Context, jobName string, buildNum int) (string, error) {
	return s.jenkinsRepo.BuildLogProgressive(ctx, jobName, buildNum)
}

func (s *Service) BuildStop(ctx context.Context, jobName string, buildNum int) error {
	return s.jenkinsRepo.BuildStop(ctx, jobName, buildNum)
}

func (s *Service) BuildDelete(ctx context.Context, jobName string, buildNum int) error {
	return s.jenkinsRepo.BuildDelete(ctx, jobName, buildNum)
}

func (s *Service) BuildArtifacts(ctx context.Context, jobName string, buildNum int) ([]domain.Artifact, error) {
	return s.jenkinsRepo.BuildArtifacts(ctx, jobName, buildNum)
}

// --- Nodes ---

func (s *Service) NodeList(ctx context.Context) ([]domain.NodeInfo, error) {
	return s.jenkinsRepo.NodeList(ctx)
}

func (s *Service) NodeGet(ctx context.Context, name string) (*domain.NodeInfo, error) {
	return s.jenkinsRepo.NodeGet(ctx, name)
}

// NodeCreate creates a node and snapshots its config.
func (s *Service) NodeCreate(ctx context.Context, env, name, configXML string) (int, error) {
	if err := s.jenkinsRepo.NodeCreate(ctx, name, configXML); err != nil {
		return 0, err
	}
	version, err := s.snapshotXML(ctx, env, domain.SnapshotNode, name, configXML, domain.OpCreated)
	if err != nil {
		return 0, fmt.Errorf("snapshot after node create: %w", err)
	}
	return version, nil
}

// NodeDelete deletes a node. Snapshot is best-effort (nodes don't expose XML config yet).
func (s *Service) NodeDelete(ctx context.Context, name string) error {
	return s.jenkinsRepo.NodeDelete(ctx, name)
}

func (s *Service) NodeEnable(ctx context.Context, name string) error {
	return s.jenkinsRepo.NodeEnable(ctx, name)
}

func (s *Service) NodeDisable(ctx context.Context, name string, message string) error {
	return s.jenkinsRepo.NodeDisable(ctx, name, message)
}

func (s *Service) NodeDisconnect(ctx context.Context, name string, message string) error {
	return s.jenkinsRepo.NodeDisconnect(ctx, name, message)
}

// --- Views ---

func (s *Service) ViewList(ctx context.Context) ([]domain.ViewInfo, error) {
	return s.jenkinsRepo.ViewList(ctx)
}

func (s *Service) ViewGet(ctx context.Context, name string) (*domain.ViewInfo, error) {
	return s.jenkinsRepo.ViewGet(ctx, name)
}

// ViewCreate creates a view and snapshots its config.
func (s *Service) ViewCreate(ctx context.Context, env, name, configXML string) (int, error) {
	if err := s.jenkinsRepo.ViewCreate(ctx, name, configXML); err != nil {
		return 0, err
	}
	version, err := s.snapshotXML(ctx, env, domain.SnapshotView, name, configXML, domain.OpCreated)
	if err != nil {
		return 0, fmt.Errorf("snapshot after view create: %w", err)
	}
	return version, nil
}

func (s *Service) ViewDelete(ctx context.Context, name string) error {
	return s.jenkinsRepo.ViewDelete(ctx, name)
}

func (s *Service) ViewAddJob(ctx context.Context, viewName, jobName string) error {
	return s.jenkinsRepo.ViewAddJob(ctx, viewName, jobName)
}

func (s *Service) ViewRemoveJob(ctx context.Context, viewName, jobName string) error {
	return s.jenkinsRepo.ViewRemoveJob(ctx, viewName, jobName)
}

// --- Queue ---

func (s *Service) QueueList(ctx context.Context) ([]domain.QueueItem, error) {
	return s.jenkinsRepo.QueueList(ctx)
}

func (s *Service) QueueCancel(ctx context.Context, id int64) error {
	return s.jenkinsRepo.QueueCancel(ctx, id)
}

// --- Plugins ---

func (s *Service) PluginList(ctx context.Context) ([]domain.PluginInfo, error) {
	return s.jenkinsRepo.PluginList(ctx)
}

// --- Credentials ---

func (s *Service) CredentialList(ctx context.Context, store, storeDomain string) ([]domain.CredentialInfo, error) {
	return s.jenkinsRepo.CredentialList(ctx, store, storeDomain)
}

// CredentialCreate creates a credential and snapshots its config.
func (s *Service) CredentialCreate(ctx context.Context, env, store, storeDomain, id, configXML string) (int, error) {
	if err := s.jenkinsRepo.CredentialCreate(ctx, store, storeDomain, id, configXML); err != nil {
		return 0, err
	}
	version, err := s.snapshotXML(ctx, env, domain.SnapshotCredential, id, configXML, domain.OpCreated)
	if err != nil {
		return 0, fmt.Errorf("snapshot after credential create: %w", err)
	}
	return version, nil
}

func (s *Service) CredentialDelete(ctx context.Context, store, storeDomain, id string) error {
	return s.jenkinsRepo.CredentialDelete(ctx, store, storeDomain, id)
}

// --- Script Console ---

func (s *Service) ScriptConsole(ctx context.Context, script string) (string, error) {
	return s.jenkinsRepo.ScriptConsole(ctx, script)
}

// --- Snapshot operations ---

// SnapshotList returns metadata for stored snapshots, most recent first.
func (s *Service) SnapshotList(ctx context.Context, env string, objType domain.SnapshotType, objName string, limit, offset int) ([]domain.SnapshotInfo, error) {
	return s.snapshotRepo.ListSnapshots(ctx, env, objType, objName, limit, offset)
}

// SnapshotGet returns the full config XML for a specific version.
func (s *Service) SnapshotGet(ctx context.Context, env string, objType domain.SnapshotType, objName string, version int) (string, error) {
	return s.snapshotRepo.GetSnapshot(ctx, env, objType, objName, version)
}

// SnapshotRestore restores an object to a previous snapshot version.
// It takes a safety snapshot of the current state before restoring.
func (s *Service) SnapshotRestore(ctx context.Context, env string, objType domain.SnapshotType, objName string, version int) error {
	// 1. Get the target snapshot config
	configXML, err := s.snapshotRepo.GetSnapshot(ctx, env, objType, objName, version)
	if err != nil {
		return fmt.Errorf("get snapshot v%d: %w", version, err)
	}

	// 2. Safety snapshot of current state before restoring
	currentCfg, err := s.getConfigXML(ctx, objType, objName)
	if err == nil && currentCfg != "" {
		if _, snapErr := s.snapshotXML(ctx, env, objType, objName, currentCfg, domain.OpRestoreSafety); snapErr != nil {
			return fmt.Errorf("safety snapshot before restore: %w (restore aborted)", snapErr)
		}
	}

	// 3. Apply the restored config
	if err := s.setConfigXML(ctx, objType, objName, configXML); err != nil {
		return fmt.Errorf("restore config: %w", err)
	}

	// 4. Record the restore operation
	if _, err := s.snapshotXML(ctx, env, objType, objName, configXML, domain.OpRestored); err != nil {
		return fmt.Errorf("snapshot after restore: %w", err)
	}

	return nil
}

// SnapshotDiff returns the config XML for two versions for comparison.
func (s *Service) SnapshotDiff(ctx context.Context, env string, objType domain.SnapshotType, objName string, versionA, versionB int) (string, string, error) {
	cfgA, err := s.snapshotRepo.GetSnapshot(ctx, env, objType, objName, versionA)
	if err != nil {
		return "", "", fmt.Errorf("get snapshot v%d: %w", versionA, err)
	}
	cfgB, err := s.snapshotRepo.GetSnapshot(ctx, env, objType, objName, versionB)
	if err != nil {
		return "", "", fmt.Errorf("get snapshot v%d: %w", versionB, err)
	}
	return cfgA, cfgB, nil
}

// SnapshotPrune removes all but the most recent N versions for a given object.
func (s *Service) SnapshotPrune(ctx context.Context, env string, objType domain.SnapshotType, objName string, keep int) (int, error) {
	return s.snapshotRepo.Prune(ctx, env, objType, objName, keep)
}

// SnapshotCount returns the total number of snapshots for a given object.
func (s *Service) SnapshotCount(ctx context.Context, env string, objType domain.SnapshotType, objName string) (int, error) {
	return s.snapshotRepo.Count(ctx, env, objType, objName)
}

// --- NoopSnapshotRepository ---

// NoopSnapshotRepository is a SnapshotRepository that does nothing.
// Used when snapshot is disabled in config.
type NoopSnapshotRepository struct{}

var _ domain.SnapshotRepository = (*NoopSnapshotRepository)(nil)

func (r *NoopSnapshotRepository) Snapshot(_ context.Context, _ string, _ domain.SnapshotType, _, _ string, _ domain.SnapshotOperation) (int, error) {
	return 0, nil
}

func (r *NoopSnapshotRepository) ListSnapshots(_ context.Context, _ string, _ domain.SnapshotType, _ string, _, _ int) ([]domain.SnapshotInfo, error) {
	return []domain.SnapshotInfo{}, nil
}

func (r *NoopSnapshotRepository) GetSnapshot(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (string, error) {
	return "", fmt.Errorf("snapshots are disabled")
}

func (r *NoopSnapshotRepository) LatestSnapshot(_ context.Context, _ string, _ domain.SnapshotType, _ string) (*domain.SnapshotInfo, string, error) {
	return nil, "", fmt.Errorf("snapshots are disabled")
}

func (r *NoopSnapshotRepository) Prune(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (int, error) {
	return 0, nil
}

func (r *NoopSnapshotRepository) Count(_ context.Context, _ string, _ domain.SnapshotType, _ string) (int, error) {
	return 0, nil
}

func (r *NoopSnapshotRepository) Close() error {
	return nil
}

// --- Utilities ---

// SortedEnvNames returns the sorted list of keys from a string-keyed map.
// Useful for building tool enum descriptions.
func SortedEnvNames[V any](envs map[string]V) []string {
	names := make([]string, 0, len(envs))
	for name := range envs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
