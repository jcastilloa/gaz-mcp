package jenkins_test

import (
	"context"
	"errors"
	"testing"

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
