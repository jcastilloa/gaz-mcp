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

func TestServiceDiagnoseDeduplicatesInformationalInspectionAlerts(t *testing.T) {
	alert := domain.InspectionAlert{Code: "TLS_SELF_SIGNED_CERTIFICATE", Severity: "info"}
	service := NewService(fakeTargetRepository{
		goroutines:      []byte("goroutine 1 [running]:\nmain.main()\n\t/srv/main.go:1\n"),
		expvars:         []byte(`{"num_goroutine":1}`),
		goroutineAlerts: []domain.InspectionAlert{alert},
		expvarAlerts:    []domain.InspectionAlert{alert},
	}, memoryProfileStore{})

	report, err := service.Diagnose(context.Background(), "https://service:6060")
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if len(report.Alerts) != 1 || report.Alerts[0].Code != alert.Code || report.Alerts[0].Severity != "info" {
		t.Fatalf("alerts = %#v, want one informational TLS alert", report.Alerts)
	}
	if len(report.Goroutines.Alerts) != 0 || len(report.Runtime.Alerts) != 0 {
		t.Fatalf("nested alerts = goroutines %#v, runtime %#v, want only the top-level alert", report.Goroutines.Alerts, report.Runtime.Alerts)
	}
	if len(report.Conclusions) != 0 {
		t.Fatalf("conclusions = %#v, want no TLS health conclusion", report.Conclusions)
	}
}

type fakeTargetRepository struct {
	profiles        []byte
	goroutines      []byte
	expvars         []byte
	goroutineAlerts []domain.InspectionAlert
	expvarAlerts    []domain.InspectionAlert
}

func (r fakeTargetRepository) FetchProfile(_ context.Context, _, _ string, _ int) (domain.FetchResult, error) {
	return domain.FetchResult{Data: r.profiles}, nil
}

func (r fakeTargetRepository) FetchGoroutines(_ context.Context, _ string) (domain.FetchResult, error) {
	return domain.FetchResult{Data: r.goroutines, Alerts: r.goroutineAlerts}, nil
}

func (r fakeTargetRepository) FetchExpvars(_ context.Context, _ string) (domain.FetchResult, error) {
	return domain.FetchResult{Data: r.expvars, Alerts: r.expvarAlerts}, nil
}

type memoryProfileStore struct{}

func (memoryProfileStore) Save(_ context.Context, profile domain.CapturedProfile) (domain.ProfileMetadata, error) {
	return profile.Metadata, nil
}

func (memoryProfileStore) Load(_ context.Context, id string) (domain.CapturedProfile, error) {
	return domain.CapturedProfile{}, fmt.Errorf("profile %q not found", id)
}
