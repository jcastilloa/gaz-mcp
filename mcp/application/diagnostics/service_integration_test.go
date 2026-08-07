package diagnostics

import (
	"context"
	"fmt"
	"testing"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

func TestServiceDiagnoseReturnsConfirmedPoolExhaustion(t *testing.T) {
	service := NewService(fakeTargetRepository{
		goroutines: []byte(blockedRequests),
		expvars: []byte(`{
  "num_goroutine": 1240,
  "memstats": {"HeapAlloc": 1234, "NumGC": 8},
  "gaz_diagnostics": {"db_pools": {"primary": {
    "open_connections": 100, "in_use": 100, "idle": 0,
    "max_open_connections": 100, "wait_count": 15340, "wait_duration_ns": 512000000000
  }}}
}`),
	}, memoryProfileStore{})

	report, err := service.Diagnose(context.Background(), "http://service:6060")
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if report.Runtime.Goroutines != 1240 {
		t.Fatalf("goroutines = %d, want 1240", report.Runtime.Goroutines)
	}
	if len(report.Conclusions) != 1 || report.Conclusions[0].Confidence != ConfidenceConfirmed {
		t.Fatalf("conclusions = %#v", report.Conclusions)
	}
}

type fakeTargetRepository struct {
	profiles   []byte
	goroutines []byte
	expvars    []byte
}

func (r fakeTargetRepository) FetchProfile(_ context.Context, _, _ string, _ int) ([]byte, error) {
	return r.profiles, nil
}

func (r fakeTargetRepository) FetchGoroutines(_ context.Context, _ string) ([]byte, error) {
	return r.goroutines, nil
}

func (r fakeTargetRepository) FetchExpvars(_ context.Context, _ string) ([]byte, error) {
	return r.expvars, nil
}

type memoryProfileStore struct{}

func (memoryProfileStore) Save(_ context.Context, profile domain.CapturedProfile) (domain.ProfileMetadata, error) {
	return profile.Metadata, nil
}

func (memoryProfileStore) Load(_ context.Context, id string) (domain.CapturedProfile, error) {
	return domain.CapturedProfile{}, fmt.Errorf("profile %q not found", id)
}
