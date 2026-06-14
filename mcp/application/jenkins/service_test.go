package jenkins_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jcastillo/gaz-mcp/mcp/application/jenkins"
	domain "github.com/jcastillo/gaz-mcp/mcp/domain/jenkins"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeRepo implements domain.Repository with controllable responses.
type fakeRepo struct {
	// Job
	jobListFn      func(ctx context.Context, filter string) ([]domain.JobInfo, error)
	jobGetFn       func(ctx context.Context, name string) (*domain.JobInfo, error)
	jobConfigFn    func(ctx context.Context, name string) (string, error)
	jobSetConfigFn func(ctx context.Context, name, configXML string) error
	jobCreateFn    func(ctx context.Context, name, configXML string) error
	jobCopyFn      func(ctx context.Context, from, to string) error
	jobDeleteFn    func(ctx context.Context, name string) error
	jobEnableFn    func(ctx context.Context, name string) error
	jobDisableFn   func(ctx context.Context, name string) error
	jobBuildFn     func(ctx context.Context, name string, params map[string]string) (int64, error)

	// Build
	buildInfoFn           func(ctx context.Context, jobName string, buildNum int) (*domain.BuildInfo, error)
	buildLogFn            func(ctx context.Context, jobName string, buildNum int, startLine int) (string, int, error)
	buildLogProgressiveFn func(ctx context.Context, jobName string, buildNum int) (string, error)
	buildStopFn           func(ctx context.Context, jobName string, buildNum int) error
	buildDeleteFn         func(ctx context.Context, jobName string, buildNum int) error
	buildArtifactsFn      func(ctx context.Context, jobName string, buildNum int) ([]domain.Artifact, error)

	// Node
	nodeListFn       func(ctx context.Context) ([]domain.NodeInfo, error)
	nodeGetFn        func(ctx context.Context, name string) (*domain.NodeInfo, error)
	nodeCreateFn     func(ctx context.Context, name, configXML string) error
	nodeDeleteFn     func(ctx context.Context, name string) error
	nodeEnableFn     func(ctx context.Context, name string) error
	nodeDisableFn    func(ctx context.Context, name string, message string) error
	nodeDisconnectFn func(ctx context.Context, name string, message string) error

	// View
	viewListFn      func(ctx context.Context) ([]domain.ViewInfo, error)
	viewGetFn       func(ctx context.Context, name string) (*domain.ViewInfo, error)
	viewCreateFn    func(ctx context.Context, name, configXML string) error
	viewDeleteFn    func(ctx context.Context, name string) error
	viewAddJobFn    func(ctx context.Context, viewName, jobName string) error
	viewRemoveJobFn func(ctx context.Context, viewName, jobName string) error

	// Queue
	queueListFn   func(ctx context.Context) ([]domain.QueueItem, error)
	queueCancelFn func(ctx context.Context, id int64) error

	// Plugins
	pluginListFn func(ctx context.Context) ([]domain.PluginInfo, error)

	// Credentials
	credentialListFn   func(ctx context.Context, store, d string) ([]domain.CredentialInfo, error)
	credentialCreateFn func(ctx context.Context, store, d, id, configXML string) error
	credentialDeleteFn func(ctx context.Context, store, d, id string) error

	// Script
	scriptConsoleFn func(ctx context.Context, script string) (string, error)

	// Info
	infoFn        func(ctx context.Context) (*domain.JenkinsInfo, error)
	quietDownFn   func(ctx context.Context) error
	cancelQuietFn func(ctx context.Context) error
}

func (f *fakeRepo) Info(ctx context.Context) (*domain.JenkinsInfo, error) {
	if f.infoFn != nil {
		return f.infoFn(ctx)
	}
	return &domain.JenkinsInfo{Version: "2.400"}, nil
}
func (f *fakeRepo) QuietDown(ctx context.Context) error {
	if f.quietDownFn != nil {
		return f.quietDownFn(ctx)
	}
	return nil
}
func (f *fakeRepo) CancelQuietDown(ctx context.Context) error {
	if f.cancelQuietFn != nil {
		return f.cancelQuietFn(ctx)
	}
	return nil
}
func (f *fakeRepo) JobList(ctx context.Context, filter string) ([]domain.JobInfo, error) {
	if f.jobListFn != nil {
		return f.jobListFn(ctx, filter)
	}
	return nil, nil
}
func (f *fakeRepo) JobGet(ctx context.Context, name string) (*domain.JobInfo, error) {
	if f.jobGetFn != nil {
		return f.jobGetFn(ctx, name)
	}
	return &domain.JobInfo{Name: name}, nil
}
func (f *fakeRepo) JobConfig(ctx context.Context, name string) (string, error) {
	if f.jobConfigFn != nil {
		return f.jobConfigFn(ctx, name)
	}
	return "<project/>", nil
}
func (f *fakeRepo) JobSetConfig(ctx context.Context, name, configXML string) error {
	if f.jobSetConfigFn != nil {
		return f.jobSetConfigFn(ctx, name, configXML)
	}
	return nil
}
func (f *fakeRepo) JobCreate(ctx context.Context, name, configXML string) error {
	if f.jobCreateFn != nil {
		return f.jobCreateFn(ctx, name, configXML)
	}
	return nil
}
func (f *fakeRepo) JobCopy(ctx context.Context, from, to string) error {
	if f.jobCopyFn != nil {
		return f.jobCopyFn(ctx, from, to)
	}
	return nil
}
func (f *fakeRepo) JobDelete(ctx context.Context, name string) error {
	if f.jobDeleteFn != nil {
		return f.jobDeleteFn(ctx, name)
	}
	return nil
}
func (f *fakeRepo) JobEnable(ctx context.Context, name string) error {
	if f.jobEnableFn != nil {
		return f.jobEnableFn(ctx, name)
	}
	return nil
}
func (f *fakeRepo) JobDisable(ctx context.Context, name string) error {
	if f.jobDisableFn != nil {
		return f.jobDisableFn(ctx, name)
	}
	return nil
}
func (f *fakeRepo) JobBuild(ctx context.Context, name string, params map[string]string) (int64, error) {
	if f.jobBuildFn != nil {
		return f.jobBuildFn(ctx, name, params)
	}
	return 42, nil
}
func (f *fakeRepo) BuildInfo(ctx context.Context, jobName string, buildNum int) (*domain.BuildInfo, error) {
	if f.buildInfoFn != nil {
		return f.buildInfoFn(ctx, jobName, buildNum)
	}
	return &domain.BuildInfo{Number: buildNum}, nil
}
func (f *fakeRepo) BuildLog(ctx context.Context, jobName string, buildNum int, startLine int) (string, int, error) {
	if f.buildLogFn != nil {
		return f.buildLogFn(ctx, jobName, buildNum, startLine)
	}
	return "log output", 10, nil
}
func (f *fakeRepo) BuildLogProgressive(ctx context.Context, jobName string, buildNum int) (string, error) {
	if f.buildLogProgressiveFn != nil {
		return f.buildLogProgressiveFn(ctx, jobName, buildNum)
	}
	return "progressive log", nil
}
func (f *fakeRepo) BuildStop(ctx context.Context, jobName string, buildNum int) error {
	if f.buildStopFn != nil {
		return f.buildStopFn(ctx, jobName, buildNum)
	}
	return nil
}
func (f *fakeRepo) BuildDelete(ctx context.Context, jobName string, buildNum int) error {
	if f.buildDeleteFn != nil {
		return f.buildDeleteFn(ctx, jobName, buildNum)
	}
	return nil
}
func (f *fakeRepo) BuildArtifacts(ctx context.Context, jobName string, buildNum int) ([]domain.Artifact, error) {
	if f.buildArtifactsFn != nil {
		return f.buildArtifactsFn(ctx, jobName, buildNum)
	}
	return []domain.Artifact{{Name: "artifact.jar", Path: "artifact.jar"}}, nil
}
func (f *fakeRepo) NodeList(ctx context.Context) ([]domain.NodeInfo, error) {
	if f.nodeListFn != nil {
		return f.nodeListFn(ctx)
	}
	return nil, nil
}
func (f *fakeRepo) NodeGet(ctx context.Context, name string) (*domain.NodeInfo, error) {
	if f.nodeGetFn != nil {
		return f.nodeGetFn(ctx, name)
	}
	return &domain.NodeInfo{Name: name}, nil
}
func (f *fakeRepo) NodeCreate(ctx context.Context, name, configXML string) error {
	if f.nodeCreateFn != nil {
		return f.nodeCreateFn(ctx, name, configXML)
	}
	return nil
}
func (f *fakeRepo) NodeDelete(ctx context.Context, name string) error {
	if f.nodeDeleteFn != nil {
		return f.nodeDeleteFn(ctx, name)
	}
	return nil
}
func (f *fakeRepo) NodeEnable(ctx context.Context, name string) error {
	if f.nodeEnableFn != nil {
		return f.nodeEnableFn(ctx, name)
	}
	return nil
}
func (f *fakeRepo) NodeDisable(ctx context.Context, name string, message string) error {
	if f.nodeDisableFn != nil {
		return f.nodeDisableFn(ctx, name, message)
	}
	return nil
}
func (f *fakeRepo) NodeDisconnect(ctx context.Context, name string, message string) error {
	if f.nodeDisconnectFn != nil {
		return f.nodeDisconnectFn(ctx, name, message)
	}
	return nil
}
func (f *fakeRepo) ViewList(ctx context.Context) ([]domain.ViewInfo, error) {
	if f.viewListFn != nil {
		return f.viewListFn(ctx)
	}
	return nil, nil
}
func (f *fakeRepo) ViewGet(ctx context.Context, name string) (*domain.ViewInfo, error) {
	if f.viewGetFn != nil {
		return f.viewGetFn(ctx, name)
	}
	return &domain.ViewInfo{Name: name}, nil
}
func (f *fakeRepo) ViewCreate(ctx context.Context, name, configXML string) error {
	if f.viewCreateFn != nil {
		return f.viewCreateFn(ctx, name, configXML)
	}
	return nil
}
func (f *fakeRepo) ViewDelete(ctx context.Context, name string) error {
	if f.viewDeleteFn != nil {
		return f.viewDeleteFn(ctx, name)
	}
	return nil
}
func (f *fakeRepo) ViewAddJob(ctx context.Context, viewName, jobName string) error {
	if f.viewAddJobFn != nil {
		return f.viewAddJobFn(ctx, viewName, jobName)
	}
	return nil
}
func (f *fakeRepo) ViewRemoveJob(ctx context.Context, viewName, jobName string) error {
	if f.viewRemoveJobFn != nil {
		return f.viewRemoveJobFn(ctx, viewName, jobName)
	}
	return nil
}
func (f *fakeRepo) QueueList(ctx context.Context) ([]domain.QueueItem, error) {
	if f.queueListFn != nil {
		return f.queueListFn(ctx)
	}
	return nil, nil
}
func (f *fakeRepo) QueueCancel(ctx context.Context, id int64) error {
	if f.queueCancelFn != nil {
		return f.queueCancelFn(ctx, id)
	}
	return nil
}
func (f *fakeRepo) PluginList(ctx context.Context) ([]domain.PluginInfo, error) {
	if f.pluginListFn != nil {
		return f.pluginListFn(ctx)
	}
	return nil, nil
}
func (f *fakeRepo) CredentialList(ctx context.Context, store, d string) ([]domain.CredentialInfo, error) {
	if f.credentialListFn != nil {
		return f.credentialListFn(ctx, store, d)
	}
	return nil, nil
}
func (f *fakeRepo) CredentialCreate(ctx context.Context, store, d, id, configXML string) error {
	if f.credentialCreateFn != nil {
		return f.credentialCreateFn(ctx, store, d, id, configXML)
	}
	return nil
}
func (f *fakeRepo) CredentialDelete(ctx context.Context, store, d, id string) error {
	if f.credentialDeleteFn != nil {
		return f.credentialDeleteFn(ctx, store, d, id)
	}
	return nil
}
func (f *fakeRepo) ScriptConsole(ctx context.Context, script string) (string, error) {
	if f.scriptConsoleFn != nil {
		return f.scriptConsoleFn(ctx, script)
	}
	return "output", nil
}

// fakeSnapshotRepo implements domain.SnapshotRepository with call tracking.
type fakeSnapshotRepo struct {
	snapshots []snapshotCall
	listFn    func(ctx context.Context, env string, objType domain.SnapshotType, objName string, limit, offset int) ([]domain.SnapshotInfo, error)
	getFn     func(ctx context.Context, env string, objType domain.SnapshotType, objName string, version int) (string, error)
	latestFn  func(ctx context.Context, env string, objType domain.SnapshotType, objName string) (*domain.SnapshotInfo, string, error)
	pruneFn   func(ctx context.Context, env string, objType domain.SnapshotType, objName string, keep int) (int, error)
	countFn   func(ctx context.Context, env string, objType domain.SnapshotType, objName string) (int, error)
	nextVer   int
}

type snapshotCall struct {
	env       string
	objType   domain.SnapshotType
	objName   string
	configXML string
	op        domain.SnapshotOperation
}

func (f *fakeSnapshotRepo) Snapshot(_ context.Context, env string, objType domain.SnapshotType, objName, configXML string, op domain.SnapshotOperation) (int, error) {
	f.snapshots = append(f.snapshots, snapshotCall{env, objType, objName, configXML, op})
	f.nextVer++
	return f.nextVer, nil
}
func (f *fakeSnapshotRepo) ListSnapshots(ctx context.Context, env string, objType domain.SnapshotType, objName string, limit, offset int) ([]domain.SnapshotInfo, error) {
	if f.listFn != nil {
		return f.listFn(ctx, env, objType, objName, limit, offset)
	}
	return nil, nil
}
func (f *fakeSnapshotRepo) GetSnapshot(ctx context.Context, env string, objType domain.SnapshotType, objName string, version int) (string, error) {
	if f.getFn != nil {
		return f.getFn(ctx, env, objType, objName, version)
	}
	return "<project/>", nil
}
func (f *fakeSnapshotRepo) LatestSnapshot(ctx context.Context, env string, objType domain.SnapshotType, objName string) (*domain.SnapshotInfo, string, error) {
	if f.latestFn != nil {
		return f.latestFn(ctx, env, objType, objName)
	}
	return nil, "", nil
}
func (f *fakeSnapshotRepo) Prune(ctx context.Context, env string, objType domain.SnapshotType, objName string, keep int) (int, error) {
	if f.pruneFn != nil {
		return f.pruneFn(ctx, env, objType, objName, keep)
	}
	return 0, nil
}
func (f *fakeSnapshotRepo) Count(ctx context.Context, env string, objType domain.SnapshotType, objName string) (int, error) {
	if f.countFn != nil {
		return f.countFn(ctx, env, objType, objName)
	}
	return 0, nil
}
func (f *fakeSnapshotRepo) Close() error { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newService(repo domain.Repository, snap domain.SnapshotRepository, maxVer int) *jenkins.Service {
	return jenkins.NewService(repo, snap, maxVer)
}

// ---------------------------------------------------------------------------
// Tests — read-only pass-through
// ---------------------------------------------------------------------------

func TestService_Info(t *testing.T) {
	repo := &fakeRepo{
		infoFn: func(_ context.Context) (*domain.JenkinsInfo, error) {
			return &domain.JenkinsInfo{Version: "2.414", JobCount: 5}, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	info, err := svc.Info(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Version != "2.414" {
		t.Errorf("expected version 2.414, got %s", info.Version)
	}
	if info.JobCount != 5 {
		t.Errorf("expected job count 5, got %d", info.JobCount)
	}
}

func TestService_Info_Error(t *testing.T) {
	repo := &fakeRepo{
		infoFn: func(_ context.Context) (*domain.JenkinsInfo, error) {
			return nil, errors.New("connection refused")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.Info(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_JobList(t *testing.T) {
	expected := []domain.JobInfo{{Name: "build-app"}, {Name: "deploy-prod"}}
	repo := &fakeRepo{
		jobListFn: func(_ context.Context, filter string) ([]domain.JobInfo, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	jobs, err := svc.JobList(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestService_BuildInfo(t *testing.T) {
	repo := &fakeRepo{
		buildInfoFn: func(_ context.Context, jobName string, buildNum int) (*domain.BuildInfo, error) {
			return &domain.BuildInfo{Number: buildNum, Result: "SUCCESS"}, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	info, err := svc.BuildInfo(context.Background(), "my-job", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Number != 42 {
		t.Errorf("expected build 42, got %d", info.Number)
	}
	if info.Result != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", info.Result)
	}
}

// ---------------------------------------------------------------------------
// Tests — JobSetConfig with snapshot
// ---------------------------------------------------------------------------

func TestService_JobSetConfig_SnapshotsBeforeUpdate(t *testing.T) {
	const currentXML = "<project><old/></project>"
	const newXML = "<project><new/></project>"

	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return currentXML, nil
		},
	}
	svc := newService(repo, snap, 0)

	ver, err := svc.JobSetConfig(context.Background(), "prod", "my-job", newXML)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}
	if len(snap.snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snap.snapshots))
	}
	s := snap.snapshots[0]
	if s.env != "prod" {
		t.Errorf("expected env prod, got %s", s.env)
	}
	if s.objType != domain.SnapshotJob {
		t.Errorf("expected SnapshotJob, got %s", s.objType)
	}
	if s.configXML != currentXML {
		t.Errorf("expected current XML to be snapshotted, got %s", s.configXML)
	}
	if s.op != domain.OpUpdated {
		t.Errorf("expected OpUpdated, got %s", s.op)
	}
}

func TestService_JobSetConfig_SkipsWhenIdentical(t *testing.T) {
	const xml = "<project/>"
	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return xml, nil
		},
	}
	svc := newService(repo, snap, 0)

	ver, err := svc.JobSetConfig(context.Background(), "prod", "my-job", xml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 0 {
		t.Errorf("expected version 0 (no change), got %d", ver)
	}
	if len(snap.snapshots) != 0 {
		t.Errorf("expected no snapshots when content is identical, got %d", len(snap.snapshots))
	}
}

func TestService_JobSetConfig_RepoError(t *testing.T) {
	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return "", errors.New("job not found")
		},
	}
	svc := newService(repo, snap, 0)

	_, err := svc.JobSetConfig(context.Background(), "prod", "missing-job", "<project/>")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(snap.snapshots) != 0 {
		t.Errorf("expected no snapshots on error, got %d", len(snap.snapshots))
	}
}

// ---------------------------------------------------------------------------
// Tests — JobCreate with snapshot
// ---------------------------------------------------------------------------

func TestService_JobCreate_SnapshotsAfterCreate(t *testing.T) {
	const xml = "<project/>"
	snap := &fakeSnapshotRepo{}
	svc := newService(&fakeRepo{}, snap, 0)

	ver, err := svc.JobCreate(context.Background(), "staging", "new-job", xml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}
	if len(snap.snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snap.snapshots))
	}
	if snap.snapshots[0].op != domain.OpCreated {
		t.Errorf("expected OpCreated, got %s", snap.snapshots[0].op)
	}
}

func TestService_JobCreate_RepoError(t *testing.T) {
	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobCreateFn: func(_ context.Context, name, configXML string) error {
			return errors.New("already exists")
		},
	}
	svc := newService(repo, snap, 0)

	_, err := svc.JobCreate(context.Background(), "prod", "dup-job", "<project/>")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(snap.snapshots) != 0 {
		t.Errorf("expected no snapshots on repo error, got %d", len(snap.snapshots))
	}
}

// ---------------------------------------------------------------------------
// Tests — JobCopy with snapshot
// ---------------------------------------------------------------------------

func TestService_JobCopy_SnapshotsNewJob(t *testing.T) {
	const srcXML = "<project><src/></project>"
	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return srcXML, nil
		},
	}
	svc := newService(repo, snap, 0)

	ver, err := svc.JobCopy(context.Background(), "prod", "src-job", "dst-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}
	if len(snap.snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snap.snapshots))
	}
	s := snap.snapshots[0]
	if s.objName != "dst-job" {
		t.Errorf("expected snapshot for dst-job, got %s", s.objName)
	}
	if s.op != domain.OpCopied {
		t.Errorf("expected OpCopied, got %s", s.op)
	}
}

// ---------------------------------------------------------------------------
// Tests — JobDelete with snapshot
// ---------------------------------------------------------------------------

func TestService_JobDelete_SnapshotsBeforeDelete(t *testing.T) {
	const xml = "<project/>"
	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return xml, nil
		},
	}
	svc := newService(repo, snap, 0)

	ver, err := svc.JobDelete(context.Background(), "prod", "old-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}
	if snap.snapshots[0].op != domain.OpDeleted {
		t.Errorf("expected OpDeleted, got %s", snap.snapshots[0].op)
	}
}

// ---------------------------------------------------------------------------
// Tests — SnapshotRestore
// ---------------------------------------------------------------------------

func TestService_SnapshotRestore_AppliesConfig(t *testing.T) {
	const targetXML = "<project><v1/></project>"
	const currentXML = "<project><current/></project>"

	var appliedXML string
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, env string, objType domain.SnapshotType, objName string, version int) (string, error) {
			return targetXML, nil
		},
	}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return currentXML, nil
		},
		jobSetConfigFn: func(_ context.Context, name, configXML string) error {
			appliedXML = configXML
			return nil
		},
	}
	svc := newService(repo, snap, 0)

	err := svc.SnapshotRestore(context.Background(), "prod", domain.SnapshotJob, "my-job", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appliedXML != targetXML {
		t.Errorf("expected target XML to be applied, got %s", appliedXML)
	}
	// Expect: 1 safety snapshot + 1 restore snapshot = 2 total
	if len(snap.snapshots) != 2 {
		t.Errorf("expected 2 snapshots (safety + restore), got %d", len(snap.snapshots))
	}
	if snap.snapshots[0].op != domain.OpRestoreSafety {
		t.Errorf("expected first snapshot to be OpRestoreSafety, got %s", snap.snapshots[0].op)
	}
	if snap.snapshots[1].op != domain.OpRestored {
		t.Errorf("expected second snapshot to be OpRestored, got %s", snap.snapshots[1].op)
	}
}

func TestService_SnapshotRestore_GetSnapshotError(t *testing.T) {
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (string, error) {
			return "", errors.New("version not found")
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)

	err := svc.SnapshotRestore(context.Background(), "prod", domain.SnapshotJob, "my-job", 99)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — SnapshotDiff
// ---------------------------------------------------------------------------

func TestService_SnapshotDiff_ReturnsBothVersions(t *testing.T) {
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, version int) (string, error) {
			if version == 1 {
				return "<v1/>", nil
			}
			return "<v2/>", nil
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)

	a, b, err := svc.SnapshotDiff(context.Background(), "prod", domain.SnapshotJob, "my-job", 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != "<v1/>" {
		t.Errorf("expected <v1/>, got %s", a)
	}
	if b != "<v2/>" {
		t.Errorf("expected <v2/>, got %s", b)
	}
}

// ---------------------------------------------------------------------------
// Tests — SortedEnvNames
// ---------------------------------------------------------------------------

func TestSortedEnvNames(t *testing.T) {
	m := map[string]*jenkins.Service{
		"prod":    nil,
		"staging": nil,
		"dev":     nil,
	}
	names := jenkins.SortedEnvNames(m)
	expected := []string{"dev", "prod", "staging"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, n := range names {
		if n != expected[i] {
			t.Errorf("index %d: expected %s, got %s", i, expected[i], n)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests — NoopSnapshotRepository
// ---------------------------------------------------------------------------

func TestNoopSnapshotRepository_Snapshot(t *testing.T) {
	noop := &jenkins.NoopSnapshotRepository{}
	ver, err := noop.Snapshot(context.Background(), "prod", domain.SnapshotJob, "job", "<xml/>", domain.OpCreated)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 0 {
		t.Errorf("expected version 0, got %d", ver)
	}
}

func TestNoopSnapshotRepository_GetSnapshot_ReturnsError(t *testing.T) {
	noop := &jenkins.NoopSnapshotRepository{}
	_, err := noop.GetSnapshot(context.Background(), "prod", domain.SnapshotJob, "job", 1)
	if err == nil {
		t.Fatal("expected error from noop GetSnapshot, got nil")
	}
}

func TestNoopSnapshotRepository_ListSnapshots_ReturnsEmpty(t *testing.T) {
	noop := &jenkins.NoopSnapshotRepository{}
	list, err := noop.ListSnapshots(context.Background(), "prod", domain.SnapshotJob, "job", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}
}

// ---------------------------------------------------------------------------
// Tests — JobBuild pass-through
// ---------------------------------------------------------------------------

func TestService_JobBuild(t *testing.T) {
	repo := &fakeRepo{
		jobBuildFn: func(_ context.Context, name string, params map[string]string) (int64, error) {
			return 99, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	queueID, err := svc.JobBuild(context.Background(), "my-job", map[string]string{"BRANCH": "main"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if queueID != 99 {
		t.Errorf("expected queue ID 99, got %d", queueID)
	}
}

// ---------------------------------------------------------------------------
// Tests — ScriptConsole pass-through
// ---------------------------------------------------------------------------

func TestService_ScriptConsole(t *testing.T) {
	repo := &fakeRepo{
		scriptConsoleFn: func(_ context.Context, script string) (string, error) {
			return "Result: 42", nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	out, err := svc.ScriptConsole(context.Background(), "println 42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Result: 42" {
		t.Errorf("expected 'Result: 42', got %s", out)
	}
}

// ---------------------------------------------------------------------------
// Tests — JobSetConfig error paths
// ---------------------------------------------------------------------------

func TestService_JobSetConfig_ErrorApplyingConfig(t *testing.T) {
	const currentXML = "<project><old/></project>"
	const newXML = "<project><new/></project>"

	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return currentXML, nil
		},
		jobSetConfigFn: func(_ context.Context, name, configXML string) error {
			return errors.New("jenkins unreachable")
		},
	}
	svc := newService(repo, snap, 0)

	ver, err := svc.JobSetConfig(context.Background(), "prod", "my-job", newXML)
	if err == nil {
		t.Fatal("expected error when applying config fails, got nil")
	}
	// Snapshot was taken before the failed apply — version should be non-zero
	if ver == 0 {
		t.Error("expected non-zero version (snapshot was taken before failed apply)")
	}
	if len(snap.snapshots) != 1 {
		t.Errorf("expected 1 snapshot (taken before failed apply), got %d", len(snap.snapshots))
	}
}

// ---------------------------------------------------------------------------
// Tests — JobDelete error paths
// ---------------------------------------------------------------------------

func TestService_JobDelete_ErrorGettingConfig(t *testing.T) {
	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return "", errors.New("job not found")
		},
	}
	svc := newService(repo, snap, 0)

	_, err := svc.JobDelete(context.Background(), "prod", "missing-job")
	if err == nil {
		t.Fatal("expected error when config fetch fails, got nil")
	}
	if len(snap.snapshots) != 0 {
		t.Errorf("expected no snapshots when config fetch fails, got %d", len(snap.snapshots))
	}
}

func TestService_JobDelete_ErrorDeleting(t *testing.T) {
	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return "<project/>", nil
		},
		jobDeleteFn: func(_ context.Context, name string) error {
			return errors.New("delete failed")
		},
	}
	svc := newService(repo, snap, 0)

	ver, err := svc.JobDelete(context.Background(), "prod", "my-job")
	if err == nil {
		t.Fatal("expected error when delete fails, got nil")
	}
	// Snapshot was taken before the failed delete
	if ver == 0 {
		t.Error("expected non-zero version (snapshot was taken before failed delete)")
	}
	if len(snap.snapshots) != 1 {
		t.Errorf("expected 1 snapshot (taken before failed delete), got %d", len(snap.snapshots))
	}
}

// ---------------------------------------------------------------------------
// Tests — JobCopy error paths
// ---------------------------------------------------------------------------

func TestService_JobCopy_ErrorGettingSourceConfig(t *testing.T) {
	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return "", errors.New("source not found")
		},
	}
	svc := newService(repo, snap, 0)

	_, err := svc.JobCopy(context.Background(), "prod", "src-job", "dst-job")
	if err == nil {
		t.Fatal("expected error when source config fetch fails, got nil")
	}
	if len(snap.snapshots) != 0 {
		t.Errorf("expected no snapshots when source config fetch fails, got %d", len(snap.snapshots))
	}
}

func TestService_JobCopy_ErrorCopying(t *testing.T) {
	snap := &fakeSnapshotRepo{}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return "<project/>", nil
		},
		jobCopyFn: func(_ context.Context, from, to string) error {
			return errors.New("copy failed")
		},
	}
	svc := newService(repo, snap, 0)

	_, err := svc.JobCopy(context.Background(), "prod", "src-job", "dst-job")
	if err == nil {
		t.Fatal("expected error when copy fails, got nil")
	}
	// No snapshot should be taken when the copy itself fails
	if len(snap.snapshots) != 0 {
		t.Errorf("expected no snapshots when copy fails, got %d", len(snap.snapshots))
	}
}

// ---------------------------------------------------------------------------
// Tests — SnapshotRestore error paths
// ---------------------------------------------------------------------------

func TestService_SnapshotRestore_ErrorApplyingConfig(t *testing.T) {
	const targetXML = "<project><v1/></project>"
	const currentXML = "<project><current/></project>"

	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (string, error) {
			return targetXML, nil
		},
	}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return currentXML, nil
		},
		jobSetConfigFn: func(_ context.Context, name, configXML string) error {
			return errors.New("apply failed")
		},
	}
	svc := newService(repo, snap, 0)

	err := svc.SnapshotRestore(context.Background(), "prod", domain.SnapshotJob, "my-job", 1)
	if err == nil {
		t.Fatal("expected error when applying restored config fails, got nil")
	}
	// Safety snapshot should have been taken before the failed apply
	if len(snap.snapshots) != 1 {
		t.Errorf("expected 1 safety snapshot before failed apply, got %d", len(snap.snapshots))
	}
	if snap.snapshots[0].op != domain.OpRestoreSafety {
		t.Errorf("expected OpRestoreSafety, got %s", snap.snapshots[0].op)
	}
}

// ---------------------------------------------------------------------------
// Tests — SnapshotDiff error paths
// ---------------------------------------------------------------------------

func TestService_SnapshotDiff_ErrorOnVersionA(t *testing.T) {
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, version int) (string, error) {
			if version == 1 {
				return "", errors.New("version 1 not found")
			}
			return "<v2/>", nil
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)

	_, _, err := svc.SnapshotDiff(context.Background(), "prod", domain.SnapshotJob, "my-job", 1, 2)
	if err == nil {
		t.Fatal("expected error when version A not found, got nil")
	}
}

func TestService_SnapshotDiff_ErrorOnVersionB(t *testing.T) {
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, version int) (string, error) {
			if version == 2 {
				return "", errors.New("version 2 not found")
			}
			return "<v1/>", nil
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)

	_, _, err := svc.SnapshotDiff(context.Background(), "prod", domain.SnapshotJob, "my-job", 1, 2)
	if err == nil {
		t.Fatal("expected error when version B not found, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — SnapshotList / SnapshotPrune / SnapshotCount pass-through
// ---------------------------------------------------------------------------

func TestService_SnapshotList_PassThrough(t *testing.T) {
	expected := []domain.SnapshotInfo{
		{Version: 2, Operation: domain.OpUpdated},
		{Version: 1, Operation: domain.OpCreated},
	}
	snap := &fakeSnapshotRepo{
		listFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, limit, offset int) ([]domain.SnapshotInfo, error) {
			return expected, nil
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)

	list, err := svc.SnapshotList(context.Background(), "prod", domain.SnapshotJob, "my-job", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 items, got %d", len(list))
	}
	if list[0].Version != 2 {
		t.Errorf("expected first item version 2, got %d", list[0].Version)
	}
}

func TestService_SnapshotPrune_PassThrough(t *testing.T) {
	snap := &fakeSnapshotRepo{
		pruneFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, keep int) (int, error) {
			return 3, nil // pretend 3 were deleted
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)

	deleted, err := svc.SnapshotPrune(context.Background(), "prod", domain.SnapshotJob, "my-job", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 3 {
		t.Errorf("expected 3 deleted, got %d", deleted)
	}
}

func TestService_SnapshotCount_PassThrough(t *testing.T) {
	snap := &fakeSnapshotRepo{
		countFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string) (int, error) {
			return 7, nil
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)

	count, err := svc.SnapshotCount(context.Background(), "prod", domain.SnapshotJob, "my-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 7 {
		t.Errorf("expected 7, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Tests — NodeCreate / ViewCreate / CredentialCreate with snapshot
// ---------------------------------------------------------------------------

func TestService_NodeCreate_SnapshotsAfterCreate(t *testing.T) {
	const xml = "<slave/>"
	snap := &fakeSnapshotRepo{}
	svc := newService(&fakeRepo{}, snap, 0)

	ver, err := svc.NodeCreate(context.Background(), "prod", "agent-01", xml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}
	if len(snap.snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snap.snapshots))
	}
	s := snap.snapshots[0]
	if s.objType != domain.SnapshotNode {
		t.Errorf("expected SnapshotNode, got %s", s.objType)
	}
	if s.op != domain.OpCreated {
		t.Errorf("expected OpCreated, got %s", s.op)
	}
}

func TestService_ViewCreate_SnapshotsAfterCreate(t *testing.T) {
	const xml = "<listView/>"
	snap := &fakeSnapshotRepo{}
	svc := newService(&fakeRepo{}, snap, 0)

	ver, err := svc.ViewCreate(context.Background(), "prod", "my-view", xml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}
	if len(snap.snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snap.snapshots))
	}
	s := snap.snapshots[0]
	if s.objType != domain.SnapshotView {
		t.Errorf("expected SnapshotView, got %s", s.objType)
	}
	if s.op != domain.OpCreated {
		t.Errorf("expected OpCreated, got %s", s.op)
	}
}

func TestService_CredentialCreate_SnapshotsAfterCreate(t *testing.T) {
	const xml = "<com.cloudbees.plugins.credentials.impl.UsernamePasswordCredentialsImpl/>"
	snap := &fakeSnapshotRepo{}
	svc := newService(&fakeRepo{}, snap, 0)

	ver, err := svc.CredentialCreate(context.Background(), "prod", "system", "_", "my-cred", xml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}
	if len(snap.snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snap.snapshots))
	}
	s := snap.snapshots[0]
	if s.objType != domain.SnapshotCredential {
		t.Errorf("expected SnapshotCredential, got %s", s.objType)
	}
	if s.objName != "my-cred" {
		t.Errorf("expected objName 'my-cred', got %s", s.objName)
	}
}

// ---------------------------------------------------------------------------
// Tests — snapshotXML with maxVersions triggers auto-prune
// ---------------------------------------------------------------------------

func TestService_SnapshotXML_AutoPruneWhenMaxVersionsSet(t *testing.T) {
	pruneCalled := false
	snap := &fakeSnapshotRepo{
		pruneFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, keep int) (int, error) {
			pruneCalled = true
			if keep != 3 {
				return 0, errors.New("unexpected keep value")
			}
			return 1, nil
		},
	}
	svc := newService(&fakeRepo{}, snap, 3) // maxVersions=3

	_, err := svc.JobCreate(context.Background(), "prod", "my-job", "<project/>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give the goroutine time to run — prune is fire-and-forget in a goroutine.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if pruneCalled {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !pruneCalled {
		t.Error("expected auto-prune to be called when maxVersions > 0")
	}
}

// ---------------------------------------------------------------------------
// Tests — NoopSnapshotRepository — remaining methods
// ---------------------------------------------------------------------------

func TestNoopSnapshotRepository_Prune_ReturnsZero(t *testing.T) {
	noop := &jenkins.NoopSnapshotRepository{}
	deleted, err := noop.Prune(context.Background(), "prod", domain.SnapshotJob, "job", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

func TestNoopSnapshotRepository_Count_ReturnsZero(t *testing.T) {
	noop := &jenkins.NoopSnapshotRepository{}
	count, err := noop.Count(context.Background(), "prod", domain.SnapshotJob, "job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestNoopSnapshotRepository_LatestSnapshot_ReturnsError(t *testing.T) {
	noop := &jenkins.NoopSnapshotRepository{}
	info, xml, err := noop.LatestSnapshot(context.Background(), "prod", domain.SnapshotJob, "job")
	if err == nil {
		t.Fatal("expected error from noop LatestSnapshot, got nil")
	}
	if info != nil {
		t.Errorf("expected nil info, got %+v", info)
	}
	if xml != "" {
		t.Errorf("expected empty xml, got %s", xml)
	}
}

func TestNoopSnapshotRepository_Close_ReturnsNil(t *testing.T) {
	noop := &jenkins.NoopSnapshotRepository{}
	if err := noop.Close(); err != nil {
		t.Errorf("expected nil from Close, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests — QuietDown / CancelQuietDown pass-through
// ---------------------------------------------------------------------------

func TestService_QuietDown(t *testing.T) {
	called := false
	repo := &fakeRepo{
		quietDownFn: func(_ context.Context) error {
			called = true
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.QuietDown(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected QuietDown to be called on repo")
	}
}

func TestService_QuietDown_Error(t *testing.T) {
	repo := &fakeRepo{
		quietDownFn: func(_ context.Context) error {
			return errors.New("quiet down failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.QuietDown(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_CancelQuietDown(t *testing.T) {
	called := false
	repo := &fakeRepo{
		cancelQuietFn: func(_ context.Context) error {
			called = true
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.CancelQuietDown(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected CancelQuietDown to be called on repo")
	}
}

func TestService_CancelQuietDown_Error(t *testing.T) {
	repo := &fakeRepo{
		cancelQuietFn: func(_ context.Context) error {
			return errors.New("cancel quiet failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.CancelQuietDown(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — JobGet / JobConfig pass-through
// ---------------------------------------------------------------------------

func TestService_JobGet(t *testing.T) {
	expected := &domain.JobInfo{Name: "my-job", URL: "http://jenkins/job/my-job"}
	repo := &fakeRepo{
		jobGetFn: func(_ context.Context, name string) (*domain.JobInfo, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	info, err := svc.JobGet(context.Background(), "my-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "my-job" {
		t.Errorf("expected name 'my-job', got %s", info.Name)
	}
}

func TestService_JobGet_Error(t *testing.T) {
	repo := &fakeRepo{
		jobGetFn: func(_ context.Context, name string) (*domain.JobInfo, error) {
			return nil, errors.New("not found")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.JobGet(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_JobConfig(t *testing.T) {
	const xml = "<project/>"
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return xml, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	cfg, err := svc.JobConfig(context.Background(), "my-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != xml {
		t.Errorf("expected %q, got %q", xml, cfg)
	}
}

func TestService_JobConfig_Error(t *testing.T) {
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return "", errors.New("job not found")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.JobConfig(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — JobEnable / JobDisable pass-through
// ---------------------------------------------------------------------------

func TestService_JobEnable(t *testing.T) {
	called := false
	repo := &fakeRepo{
		jobEnableFn: func(_ context.Context, name string) error {
			called = true
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.JobEnable(context.Background(), "my-job"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected JobEnable to be called on repo")
	}
}

func TestService_JobEnable_Error(t *testing.T) {
	repo := &fakeRepo{
		jobEnableFn: func(_ context.Context, name string) error {
			return errors.New("enable failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.JobEnable(context.Background(), "my-job"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_JobDisable(t *testing.T) {
	called := false
	repo := &fakeRepo{
		jobDisableFn: func(_ context.Context, name string) error {
			called = true
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.JobDisable(context.Background(), "my-job"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected JobDisable to be called on repo")
	}
}

func TestService_JobDisable_Error(t *testing.T) {
	repo := &fakeRepo{
		jobDisableFn: func(_ context.Context, name string) error {
			return errors.New("disable failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.JobDisable(context.Background(), "my-job"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — Build pass-through methods
// ---------------------------------------------------------------------------

func TestService_BuildLog(t *testing.T) {
	repo := &fakeRepo{
		buildLogFn: func(_ context.Context, jobName string, buildNum int, startLine int) (string, int, error) {
			return "line1\nline2\n", 2, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	log, nextLine, err := svc.BuildLog(context.Background(), "my-job", 1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if log != "line1\nline2\n" {
		t.Errorf("unexpected log content: %q", log)
	}
	if nextLine != 2 {
		t.Errorf("expected nextLine 2, got %d", nextLine)
	}
}

func TestService_BuildLog_Error(t *testing.T) {
	repo := &fakeRepo{
		buildLogFn: func(_ context.Context, jobName string, buildNum int, startLine int) (string, int, error) {
			return "", 0, errors.New("build not found")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, _, err := svc.BuildLog(context.Background(), "my-job", 999, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_BuildLogProgressive(t *testing.T) {
	repo := &fakeRepo{
		buildLogProgressiveFn: func(_ context.Context, jobName string, buildNum int) (string, error) {
			return "progressive output", nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	out, err := svc.BuildLogProgressive(context.Background(), "my-job", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "progressive output" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestService_BuildLogProgressive_Error(t *testing.T) {
	repo := &fakeRepo{
		buildLogProgressiveFn: func(_ context.Context, jobName string, buildNum int) (string, error) {
			return "", errors.New("build not found")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.BuildLogProgressive(context.Background(), "my-job", 999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_BuildStop(t *testing.T) {
	called := false
	repo := &fakeRepo{
		buildStopFn: func(_ context.Context, jobName string, buildNum int) error {
			called = true
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.BuildStop(context.Background(), "my-job", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected BuildStop to be called on repo")
	}
}

func TestService_BuildStop_Error(t *testing.T) {
	repo := &fakeRepo{
		buildStopFn: func(_ context.Context, jobName string, buildNum int) error {
			return errors.New("stop failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.BuildStop(context.Background(), "my-job", 1); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_BuildDelete(t *testing.T) {
	called := false
	repo := &fakeRepo{
		buildDeleteFn: func(_ context.Context, jobName string, buildNum int) error {
			called = true
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.BuildDelete(context.Background(), "my-job", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected BuildDelete to be called on repo")
	}
}

func TestService_BuildDelete_Error(t *testing.T) {
	repo := &fakeRepo{
		buildDeleteFn: func(_ context.Context, jobName string, buildNum int) error {
			return errors.New("delete failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.BuildDelete(context.Background(), "my-job", 1); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_BuildArtifacts(t *testing.T) {
	expected := []domain.Artifact{{Name: "app.jar", Path: "target/app.jar"}}
	repo := &fakeRepo{
		buildArtifactsFn: func(_ context.Context, jobName string, buildNum int) ([]domain.Artifact, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	artifacts, err := svc.BuildArtifacts(context.Background(), "my-job", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Name != "app.jar" {
		t.Errorf("expected 'app.jar', got %s", artifacts[0].Name)
	}
}

func TestService_BuildArtifacts_Error(t *testing.T) {
	repo := &fakeRepo{
		buildArtifactsFn: func(_ context.Context, jobName string, buildNum int) ([]domain.Artifact, error) {
			return nil, errors.New("build not found")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.BuildArtifacts(context.Background(), "my-job", 999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — Node pass-through methods
// ---------------------------------------------------------------------------

func TestService_NodeList(t *testing.T) {
	expected := []domain.NodeInfo{{Name: "agent-01", Online: true}, {Name: "agent-02", Online: false}}
	repo := &fakeRepo{
		nodeListFn: func(_ context.Context) ([]domain.NodeInfo, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	nodes, err := svc.NodeList(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestService_NodeList_Error(t *testing.T) {
	repo := &fakeRepo{
		nodeListFn: func(_ context.Context) ([]domain.NodeInfo, error) {
			return nil, errors.New("connection refused")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.NodeList(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_NodeGet(t *testing.T) {
	expected := &domain.NodeInfo{Name: "agent-01", Online: true, Idle: true}
	repo := &fakeRepo{
		nodeGetFn: func(_ context.Context, name string) (*domain.NodeInfo, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	node, err := svc.NodeGet(context.Background(), "agent-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Name != "agent-01" {
		t.Errorf("expected 'agent-01', got %s", node.Name)
	}
}

func TestService_NodeGet_Error(t *testing.T) {
	repo := &fakeRepo{
		nodeGetFn: func(_ context.Context, name string) (*domain.NodeInfo, error) {
			return nil, errors.New("node not found")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.NodeGet(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_NodeDelete(t *testing.T) {
	called := false
	repo := &fakeRepo{
		nodeDeleteFn: func(_ context.Context, name string) error {
			called = true
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.NodeDelete(context.Background(), "agent-01"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected NodeDelete to be called on repo")
	}
}

func TestService_NodeDelete_Error(t *testing.T) {
	repo := &fakeRepo{
		nodeDeleteFn: func(_ context.Context, name string) error {
			return errors.New("delete failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.NodeDelete(context.Background(), "agent-01"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_NodeEnable(t *testing.T) {
	called := false
	repo := &fakeRepo{
		nodeEnableFn: func(_ context.Context, name string) error {
			called = true
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.NodeEnable(context.Background(), "agent-01"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected NodeEnable to be called on repo")
	}
}

func TestService_NodeEnable_Error(t *testing.T) {
	repo := &fakeRepo{
		nodeEnableFn: func(_ context.Context, name string) error {
			return errors.New("enable failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.NodeEnable(context.Background(), "agent-01"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_NodeDisable(t *testing.T) {
	var gotMsg string
	repo := &fakeRepo{
		nodeDisableFn: func(_ context.Context, name string, message string) error {
			gotMsg = message
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.NodeDisable(context.Background(), "agent-01", "maintenance"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMsg != "maintenance" {
		t.Errorf("expected message 'maintenance', got %q", gotMsg)
	}
}

func TestService_NodeDisable_Error(t *testing.T) {
	repo := &fakeRepo{
		nodeDisableFn: func(_ context.Context, name string, message string) error {
			return errors.New("disable failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.NodeDisable(context.Background(), "agent-01", ""); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_NodeDisconnect(t *testing.T) {
	var gotMsg string
	repo := &fakeRepo{
		nodeDisconnectFn: func(_ context.Context, name string, message string) error {
			gotMsg = message
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.NodeDisconnect(context.Background(), "agent-01", "going offline"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMsg != "going offline" {
		t.Errorf("expected message 'going offline', got %q", gotMsg)
	}
}

func TestService_NodeDisconnect_Error(t *testing.T) {
	repo := &fakeRepo{
		nodeDisconnectFn: func(_ context.Context, name string, message string) error {
			return errors.New("disconnect failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.NodeDisconnect(context.Background(), "agent-01", ""); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — View pass-through methods
// ---------------------------------------------------------------------------

func TestService_ViewList(t *testing.T) {
	expected := []domain.ViewInfo{{Name: "All"}, {Name: "Deploy"}}
	repo := &fakeRepo{
		viewListFn: func(_ context.Context) ([]domain.ViewInfo, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	views, err := svc.ViewList(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(views) != 2 {
		t.Errorf("expected 2 views, got %d", len(views))
	}
}

func TestService_ViewList_Error(t *testing.T) {
	repo := &fakeRepo{
		viewListFn: func(_ context.Context) ([]domain.ViewInfo, error) {
			return nil, errors.New("connection refused")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.ViewList(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_ViewGet(t *testing.T) {
	expected := &domain.ViewInfo{Name: "Deploy", URL: "http://jenkins/view/Deploy"}
	repo := &fakeRepo{
		viewGetFn: func(_ context.Context, name string) (*domain.ViewInfo, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	view, err := svc.ViewGet(context.Background(), "Deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Name != "Deploy" {
		t.Errorf("expected 'Deploy', got %s", view.Name)
	}
}

func TestService_ViewGet_Error(t *testing.T) {
	repo := &fakeRepo{
		viewGetFn: func(_ context.Context, name string) (*domain.ViewInfo, error) {
			return nil, errors.New("view not found")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.ViewGet(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_ViewDelete(t *testing.T) {
	called := false
	repo := &fakeRepo{
		viewDeleteFn: func(_ context.Context, name string) error {
			called = true
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.ViewDelete(context.Background(), "Deploy"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected ViewDelete to be called on repo")
	}
}

func TestService_ViewDelete_Error(t *testing.T) {
	repo := &fakeRepo{
		viewDeleteFn: func(_ context.Context, name string) error {
			return errors.New("delete failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.ViewDelete(context.Background(), "Deploy"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_ViewAddJob(t *testing.T) {
	var gotView, gotJob string
	repo := &fakeRepo{
		viewAddJobFn: func(_ context.Context, viewName, jobName string) error {
			gotView, gotJob = viewName, jobName
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.ViewAddJob(context.Background(), "Deploy", "my-job"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotView != "Deploy" || gotJob != "my-job" {
		t.Errorf("expected (Deploy, my-job), got (%s, %s)", gotView, gotJob)
	}
}

func TestService_ViewAddJob_Error(t *testing.T) {
	repo := &fakeRepo{
		viewAddJobFn: func(_ context.Context, viewName, jobName string) error {
			return errors.New("add failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.ViewAddJob(context.Background(), "Deploy", "my-job"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_ViewRemoveJob(t *testing.T) {
	var gotView, gotJob string
	repo := &fakeRepo{
		viewRemoveJobFn: func(_ context.Context, viewName, jobName string) error {
			gotView, gotJob = viewName, jobName
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.ViewRemoveJob(context.Background(), "Deploy", "my-job"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotView != "Deploy" || gotJob != "my-job" {
		t.Errorf("expected (Deploy, my-job), got (%s, %s)", gotView, gotJob)
	}
}

func TestService_ViewRemoveJob_Error(t *testing.T) {
	repo := &fakeRepo{
		viewRemoveJobFn: func(_ context.Context, viewName, jobName string) error {
			return errors.New("remove failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.ViewRemoveJob(context.Background(), "Deploy", "my-job"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — Queue pass-through methods
// ---------------------------------------------------------------------------

func TestService_QueueList(t *testing.T) {
	expected := []domain.QueueItem{{ID: 1, Task: "my-job"}, {ID: 2, Task: "other-job"}}
	repo := &fakeRepo{
		queueListFn: func(_ context.Context) ([]domain.QueueItem, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	items, err := svc.QueueList(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", items[0].ID)
	}
}

func TestService_QueueList_Error(t *testing.T) {
	repo := &fakeRepo{
		queueListFn: func(_ context.Context) ([]domain.QueueItem, error) {
			return nil, errors.New("connection refused")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.QueueList(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_QueueCancel(t *testing.T) {
	var gotID int64
	repo := &fakeRepo{
		queueCancelFn: func(_ context.Context, id int64) error {
			gotID = id
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.QueueCancel(context.Background(), 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotID != 42 {
		t.Errorf("expected ID 42, got %d", gotID)
	}
}

func TestService_QueueCancel_Error(t *testing.T) {
	repo := &fakeRepo{
		queueCancelFn: func(_ context.Context, id int64) error {
			return errors.New("cancel failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.QueueCancel(context.Background(), 42); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — PluginList pass-through
// ---------------------------------------------------------------------------

func TestService_PluginList(t *testing.T) {
	expected := []domain.PluginInfo{
		{ShortName: "git", Version: "4.11.0", Enabled: true},
		{ShortName: "workflow-aggregator", Version: "2.6", Enabled: true},
	}
	repo := &fakeRepo{
		pluginListFn: func(_ context.Context) ([]domain.PluginInfo, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	plugins, err := svc.PluginList(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}
	if plugins[0].ShortName != "git" {
		t.Errorf("expected 'git', got %s", plugins[0].ShortName)
	}
}

func TestService_PluginList_Error(t *testing.T) {
	repo := &fakeRepo{
		pluginListFn: func(_ context.Context) ([]domain.PluginInfo, error) {
			return nil, errors.New("connection refused")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.PluginList(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — Credential pass-through methods
// ---------------------------------------------------------------------------

func TestService_CredentialList(t *testing.T) {
	expected := []domain.CredentialInfo{{ID: "my-cred", Description: "GitHub token"}}
	repo := &fakeRepo{
		credentialListFn: func(_ context.Context, store, d string) ([]domain.CredentialInfo, error) {
			return expected, nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	creds, err := svc.CredentialList(context.Background(), "system", "_")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(creds))
	}
	if creds[0].ID != "my-cred" {
		t.Errorf("expected 'my-cred', got %s", creds[0].ID)
	}
}

func TestService_CredentialList_Error(t *testing.T) {
	repo := &fakeRepo{
		credentialListFn: func(_ context.Context, store, d string) ([]domain.CredentialInfo, error) {
			return nil, errors.New("store not found")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	_, err := svc.CredentialList(context.Background(), "system", "_")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestService_CredentialDelete(t *testing.T) {
	var gotStore, gotDomain, gotID string
	repo := &fakeRepo{
		credentialDeleteFn: func(_ context.Context, store, d, id string) error {
			gotStore, gotDomain, gotID = store, d, id
			return nil
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.CredentialDelete(context.Background(), "system", "_", "my-cred"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotStore != "system" || gotDomain != "_" || gotID != "my-cred" {
		t.Errorf("unexpected args: store=%s domain=%s id=%s", gotStore, gotDomain, gotID)
	}
}

func TestService_CredentialDelete_Error(t *testing.T) {
	repo := &fakeRepo{
		credentialDeleteFn: func(_ context.Context, store, d, id string) error {
			return errors.New("delete failed")
		},
	}
	svc := newService(repo, &jenkins.NoopSnapshotRepository{}, 0)
	if err := svc.CredentialDelete(context.Background(), "system", "_", "my-cred"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — SnapshotGet pass-through
// ---------------------------------------------------------------------------

func TestService_SnapshotGet(t *testing.T) {
	const xml = "<project><v3/></project>"
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, version int) (string, error) {
			if version == 3 {
				return xml, nil
			}
			return "", errors.New("not found")
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)
	got, err := svc.SnapshotGet(context.Background(), "prod", domain.SnapshotJob, "my-job", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != xml {
		t.Errorf("expected %q, got %q", xml, got)
	}
}

// ---------------------------------------------------------------------------
// Tests — JobSetConfig snapshot error path
// ---------------------------------------------------------------------------

func TestService_JobSetConfig_SnapshotError(t *testing.T) {
	const currentXML = "<project><old/></project>"
	const newXML = "<project><new/></project>"

	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return currentXML, nil
		},
	}
	svc := newService(repo, &errorSnapshotRepo{}, 0)

	_, err := svc.JobSetConfig(context.Background(), "prod", "my-job", newXML)
	if err == nil {
		t.Fatal("expected error when snapshot fails before update, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — JobCopy snapshot error path
// ---------------------------------------------------------------------------

func TestService_JobCopy_SnapshotError(t *testing.T) {
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return "<project/>", nil
		},
	}
	svc := newService(repo, &errorSnapshotRepo{}, 0)

	_, err := svc.JobCopy(context.Background(), "prod", "src-job", "dst-job")
	if err == nil {
		t.Fatal("expected error when snapshot fails after copy, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — JobDelete snapshot error path
// ---------------------------------------------------------------------------

func TestService_JobDelete_SnapshotError(t *testing.T) {
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			return "<project/>", nil
		},
	}
	svc := newService(repo, &errorSnapshotRepo{}, 0)

	_, err := svc.JobDelete(context.Background(), "prod", "my-job")
	if err == nil {
		t.Fatal("expected error when snapshot fails before delete, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — NodeCreate / ViewCreate / CredentialCreate repo error paths
// ---------------------------------------------------------------------------

func TestService_NodeCreate_RepoError(t *testing.T) {
	repo := &fakeRepo{
		nodeCreateFn: func(_ context.Context, name, configXML string) error {
			return errors.New("node create failed")
		},
	}
	svc := newService(repo, &fakeSnapshotRepo{}, 0)
	_, err := svc.NodeCreate(context.Background(), "prod", "agent-01", "<slave/>")
	if err == nil {
		t.Fatal("expected error when NodeCreate repo fails, got nil")
	}
}

func TestService_ViewCreate_RepoError(t *testing.T) {
	repo := &fakeRepo{
		viewCreateFn: func(_ context.Context, name, configXML string) error {
			return errors.New("view create failed")
		},
	}
	svc := newService(repo, &fakeSnapshotRepo{}, 0)
	_, err := svc.ViewCreate(context.Background(), "prod", "my-view", "<listView/>")
	if err == nil {
		t.Fatal("expected error when ViewCreate repo fails, got nil")
	}
}

func TestService_CredentialCreate_RepoError(t *testing.T) {
	repo := &fakeRepo{
		credentialCreateFn: func(_ context.Context, store, d, id, configXML string) error {
			return errors.New("credential create failed")
		},
	}
	svc := newService(repo, &fakeSnapshotRepo{}, 0)
	_, err := svc.CredentialCreate(context.Background(), "prod", "system", "_", "my-cred", "<cred/>")
	if err == nil {
		t.Fatal("expected error when CredentialCreate repo fails, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — SnapshotRestore post-restore snapshot error path
// ---------------------------------------------------------------------------

func TestService_SnapshotRestore_PostRestoreSnapshotError(t *testing.T) {
	// The restore applies successfully but the final "record restore" snapshot fails.
	const targetXML = "<project><v1/></project>"

	callCount := 0
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (string, error) {
			return targetXML, nil
		},
	}
	// Fail on the 2nd Snapshot call (1st = safety, 2nd = post-restore record)
	errOnSecond := &errorOnNthSnapshotRepo{failOn: 2, delegate: snap}
	repo := &fakeRepo{
		// getConfigXML returns empty for SnapshotJob when jobConfigFn is nil — but we need it to return something
		// so the safety snapshot is attempted. Use a non-nil jobConfigFn:
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			callCount++
			return "<project><current/></project>", nil
		},
	}
	svc := newService(repo, errOnSecond, 0)

	err := svc.SnapshotRestore(context.Background(), "prod", domain.SnapshotJob, "my-job", 1)
	if err == nil {
		t.Fatal("expected error when post-restore snapshot fails, got nil")
	}
}

func TestService_SnapshotGet_Error(t *testing.T) {
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, version int) (string, error) {
			return "", errors.New("version not found")
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)
	_, err := svc.SnapshotGet(context.Background(), "prod", domain.SnapshotJob, "my-job", 99)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — snapshotXML error path (Snapshot repo returns error)
// ---------------------------------------------------------------------------

func TestService_SnapshotXML_SnapshotRepoError(t *testing.T) {
	snap := &fakeSnapshotRepo{
		// Override Snapshot to return an error
	}
	// We need a custom fakeSnapshotRepo that errors on Snapshot
	errSnap := &errorSnapshotRepo{}
	svc := newService(&fakeRepo{}, errSnap, 0)

	// JobCreate calls snapshotXML after creating the job
	_, err := svc.JobCreate(context.Background(), "prod", "my-job", "<project/>")
	if err == nil {
		t.Fatal("expected error when snapshot repo fails, got nil")
	}
	_ = snap // suppress unused warning
}

// errorSnapshotRepo always returns an error from Snapshot.
type errorSnapshotRepo struct{}

func (r *errorSnapshotRepo) Snapshot(_ context.Context, _ string, _ domain.SnapshotType, _, _ string, _ domain.SnapshotOperation) (int, error) {
	return 0, errors.New("snapshot storage failed")
}
func (r *errorSnapshotRepo) ListSnapshots(_ context.Context, _ string, _ domain.SnapshotType, _ string, _, _ int) ([]domain.SnapshotInfo, error) {
	return nil, nil
}
func (r *errorSnapshotRepo) GetSnapshot(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (string, error) {
	return "", errors.New("snapshots disabled")
}
func (r *errorSnapshotRepo) LatestSnapshot(_ context.Context, _ string, _ domain.SnapshotType, _ string) (*domain.SnapshotInfo, string, error) {
	return nil, "", nil
}
func (r *errorSnapshotRepo) Prune(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (int, error) {
	return 0, nil
}
func (r *errorSnapshotRepo) Count(_ context.Context, _ string, _ domain.SnapshotType, _ string) (int, error) {
	return 0, nil
}
func (r *errorSnapshotRepo) Close() error { return nil }

// ---------------------------------------------------------------------------
// Tests — getConfigXML default branch (non-Job/Folder types return empty)
// ---------------------------------------------------------------------------

func TestService_SnapshotRestore_NodeType_GetConfigXMLReturnsEmpty(t *testing.T) {
	// For SnapshotNode, getConfigXML returns ("", nil) — no safety snapshot is taken.
	// The restore should still apply the config via setConfigXML, which will fail
	// because setConfigXML doesn't support Node type — so we get an error from setConfigXML.
	const targetXML = "<slave/>"
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (string, error) {
			return targetXML, nil
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)

	// SnapshotNode hits the default branch in setConfigXML → error
	err := svc.SnapshotRestore(context.Background(), "prod", domain.SnapshotNode, "agent-01", 1)
	if err == nil {
		t.Fatal("expected error for unsupported object type in setConfigXML, got nil")
	}
	// No safety snapshot should have been taken (getConfigXML returned "")
	if len(snap.snapshots) != 0 {
		t.Errorf("expected no safety snapshot for node type, got %d", len(snap.snapshots))
	}
}

// ---------------------------------------------------------------------------
// Tests — setConfigXML default branch (unsupported type returns error)
// ---------------------------------------------------------------------------

func TestService_SnapshotRestore_UnsupportedType_SetConfigXMLError(t *testing.T) {
	// SnapshotCredential hits the default branch in setConfigXML
	const targetXML = "<credential/>"
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (string, error) {
			return targetXML, nil
		},
	}
	svc := newService(&fakeRepo{}, snap, 0)

	err := svc.SnapshotRestore(context.Background(), "prod", domain.SnapshotCredential, "my-cred", 1)
	if err == nil {
		t.Fatal("expected error for unsupported type in setConfigXML, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — NodeCreate / ViewCreate / CredentialCreate snapshot error paths
// ---------------------------------------------------------------------------

func TestService_NodeCreate_SnapshotError(t *testing.T) {
	svc := newService(&fakeRepo{}, &errorSnapshotRepo{}, 0)
	_, err := svc.NodeCreate(context.Background(), "prod", "agent-01", "<slave/>")
	if err == nil {
		t.Fatal("expected error when snapshot fails after NodeCreate, got nil")
	}
}

func TestService_ViewCreate_SnapshotError(t *testing.T) {
	svc := newService(&fakeRepo{}, &errorSnapshotRepo{}, 0)
	_, err := svc.ViewCreate(context.Background(), "prod", "my-view", "<listView/>")
	if err == nil {
		t.Fatal("expected error when snapshot fails after ViewCreate, got nil")
	}
}

func TestService_CredentialCreate_SnapshotError(t *testing.T) {
	svc := newService(&fakeRepo{}, &errorSnapshotRepo{}, 0)
	_, err := svc.CredentialCreate(context.Background(), "prod", "system", "_", "my-cred", "<cred/>")
	if err == nil {
		t.Fatal("expected error when snapshot fails after CredentialCreate, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests — SnapshotRestore safety snapshot error path
// ---------------------------------------------------------------------------

func TestService_SnapshotRestore_SafetySnapshotError(t *testing.T) {
	// Safety snapshot fails → restore is aborted
	const targetXML = "<project><v1/></project>"
	const currentXML = "<project><current/></project>"

	callCount := 0
	snap := &fakeSnapshotRepo{
		getFn: func(_ context.Context, _ string, _ domain.SnapshotType, _ string, _ int) (string, error) {
			return targetXML, nil
		},
	}
	// Override Snapshot to fail on the safety snapshot call
	errOnFirstSnap := &errorOnNthSnapshotRepo{failOn: 1, delegate: snap}
	repo := &fakeRepo{
		jobConfigFn: func(_ context.Context, name string) (string, error) {
			callCount++
			return currentXML, nil
		},
	}
	svc := newService(repo, errOnFirstSnap, 0)

	err := svc.SnapshotRestore(context.Background(), "prod", domain.SnapshotJob, "my-job", 1)
	if err == nil {
		t.Fatal("expected error when safety snapshot fails, got nil")
	}
}

// errorOnNthSnapshotRepo fails on the Nth call to Snapshot, delegates otherwise.
type errorOnNthSnapshotRepo struct {
	failOn   int
	calls    int
	delegate *fakeSnapshotRepo
}

func (r *errorOnNthSnapshotRepo) Snapshot(ctx context.Context, env string, objType domain.SnapshotType, objName, configXML string, op domain.SnapshotOperation) (int, error) {
	r.calls++
	if r.calls == r.failOn {
		return 0, errors.New("safety snapshot failed")
	}
	return r.delegate.Snapshot(ctx, env, objType, objName, configXML, op)
}
func (r *errorOnNthSnapshotRepo) ListSnapshots(ctx context.Context, env string, objType domain.SnapshotType, objName string, limit, offset int) ([]domain.SnapshotInfo, error) {
	return r.delegate.ListSnapshots(ctx, env, objType, objName, limit, offset)
}
func (r *errorOnNthSnapshotRepo) GetSnapshot(ctx context.Context, env string, objType domain.SnapshotType, objName string, version int) (string, error) {
	return r.delegate.GetSnapshot(ctx, env, objType, objName, version)
}
func (r *errorOnNthSnapshotRepo) LatestSnapshot(ctx context.Context, env string, objType domain.SnapshotType, objName string) (*domain.SnapshotInfo, string, error) {
	return r.delegate.LatestSnapshot(ctx, env, objType, objName)
}
func (r *errorOnNthSnapshotRepo) Prune(ctx context.Context, env string, objType domain.SnapshotType, objName string, keep int) (int, error) {
	return r.delegate.Prune(ctx, env, objType, objName, keep)
}
func (r *errorOnNthSnapshotRepo) Count(ctx context.Context, env string, objType domain.SnapshotType, objName string) (int, error) {
	return r.delegate.Count(ctx, env, objType, objName)
}
func (r *errorOnNthSnapshotRepo) Close() error { return nil }
