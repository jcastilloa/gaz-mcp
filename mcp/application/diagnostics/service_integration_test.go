package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

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
	if numberAt(report.Metrics.After.Runtime, "goroutines", "total") != 1240 {
		t.Fatalf("runtime metrics = %#v, want 1240 goroutines", report.Metrics.After.Runtime)
	}
	if suspicion := suspicionByCode(t, report.Suspicions, FindingDBPoolExhausted); suspicion.Confidence != ConfidenceConfirmed {
		t.Fatalf("suspicion = %#v", suspicion)
	}
	for _, group := range report.Goroutines.Groups {
		if len(group.Findings) != 0 {
			t.Fatalf("goroutine facts contain heuristic findings: %#v", group.Findings)
		}
	}
}

func TestServiceDiagnoseReturnsRawMetricSamplesAndSeparateSuspicions(t *testing.T) {
	repository := &sequenceTargetRepository{
		goroutines: []byte("goroutine 1 [running]:\nmain.main()\n\t/srv/main.go:1\n"),
		expvars: [][]byte{
			[]byte(`{"gaz_diagnostics":{"collected_at":"2026-08-08T12:00:00Z","process":{"open_fds":700,"fd_soft_limit":1000},"runtime":{},"cgroup":{},"db_pools":{},"custom":{}}}`),
			[]byte(`{"gaz_diagnostics":{"collected_at":"2026-08-08T12:00:02Z","process":{"open_fds":900,"fd_soft_limit":1000},"runtime":{},"cgroup":{},"db_pools":{},"custom":{}}}`),
		},
	}
	service := NewService(repository, memoryProfileStore{})
	service.wait = func(context.Context, time.Duration) error { return nil }
	observedTimes := []time.Time{time.Unix(200, 0), time.Unix(201, 0)}
	service.now = func() time.Time {
		value := observedTimes[0]
		observedTimes = observedTimes[1:]
		return value
	}

	report, err := service.Diagnose(context.Background(), "http://service:6060")
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if repository.expvarCalls != 2 {
		t.Fatalf("expvar calls = %d, want two metric samples", repository.expvarCalls)
	}
	if report.Metrics.DurationNs != int64(time.Second) {
		t.Fatalf("duration = %s, want locally observed 1s", time.Duration(report.Metrics.DurationNs))
	}
	if !report.Metrics.Before.CollectedAt.Equal(time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)) ||
		!report.Metrics.Before.ObservedAt.Equal(time.Unix(200, 0)) {
		t.Fatalf("timestamps = %#v, want separate target and observer clocks", report.Metrics.Before)
	}
	if numberAt(report.Metrics.Before.Process, "open_fds") != 700 || numberAt(report.Metrics.After.Process, "open_fds") != 900 {
		t.Fatalf("metrics = %#v", report.Metrics)
	}
	if !hasSuspicion(report.Suspicions, FindingFDLimitPressure) || !hasSuspicion(report.Suspicions, FindingPossibleFDLeak) {
		t.Fatalf("suspicions = %#v", report.Suspicions)
	}
}

func TestServiceDiagnoseDisablesDeltasWhenSecondSampleFails(t *testing.T) {
	repository := &failingSecondSampleRepository{
		sequenceTargetRepository: sequenceTargetRepository{
			goroutines: []byte("goroutine 1 [running]:\nmain.main()\n\t/srv/main.go:1\n"),
			expvars:    [][]byte{[]byte(`{"gaz_diagnostics":{"process":{"open_fds":700,"fd_soft_limit":1000}}}`)},
		},
	}
	service := NewService(repository, memoryProfileStore{})
	service.wait = func(context.Context, time.Duration) error { return nil }
	service.now = func() time.Time { return time.Unix(200, 0) }

	report, err := service.Diagnose(context.Background(), "http://service:6060")
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if report.Metrics.DurationNs != 0 || !reflect.DeepEqual(report.Metrics.Before, report.Metrics.After) {
		t.Fatalf("metrics = %#v, failed second sample must not create a rate window", report.Metrics)
	}
	if !hasAlert(report.Alerts, "METRIC_SECOND_SAMPLE_UNAVAILABLE") {
		t.Fatalf("alerts = %#v, want second sample alert", report.Alerts)
	}
}

func TestServiceDiagnoseStopsWhenSamplingContextIsCancelled(t *testing.T) {
	repository := &sequenceTargetRepository{
		goroutines: []byte("goroutine 1 [running]:\nmain.main()\n\t/srv/main.go:1\n"),
		expvars:    [][]byte{[]byte(`{"gaz_diagnostics":{"runtime":{},"process":{},"cgroup":{}}}`)},
	}
	service := NewService(repository, memoryProfileStore{})
	service.wait = func(context.Context, time.Duration) error { return context.Canceled }

	_, err := service.Diagnose(context.Background(), "http://service:6060")
	if err == nil || !reflect.DeepEqual(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestServiceDiagnoseDoesNotHideCancelledEndpointFetch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := NewService(fakeTargetRepository{goroutineErr: context.Canceled}, memoryProfileStore{})
	if _, err := service.Diagnose(ctx, "http://service:6060"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestServiceDiagnoseKeepsMetricEvidenceWhenGoroutinesAreUnavailable(t *testing.T) {
	service := NewService(fakeTargetRepository{
		goroutineErr: fmt.Errorf("goroutine endpoint unavailable"),
		expvars:      []byte(`{"gaz_diagnostics":{"host":{"memory_total_bytes":1000,"memory_available_bytes":50}}}`),
	}, memoryProfileStore{})
	service.wait = func(context.Context, time.Duration) error { return nil }

	report, err := service.Diagnose(context.Background(), "http://service:6060")
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	if !hasSuspicion(report.Suspicions, FindingHostMemoryPressure) {
		t.Fatalf("suspicions = %#v, want metric-based diagnosis", report.Suspicions)
	}
	if !hasAlert(report.Alerts, "GOROUTINES_UNAVAILABLE") {
		t.Fatalf("alerts = %#v, want goroutine availability alert", report.Alerts)
	}
	if description := alertDescription(report.Alerts, "GOROUTINES_UNAVAILABLE"); !strings.Contains(description, "goroutine endpoint unavailable") {
		t.Fatalf("alert description = %q, want original endpoint error", description)
	}
}

func TestServiceDiagnoseReportsMetricParsingFailure(t *testing.T) {
	service := NewService(fakeTargetRepository{
		goroutines: []byte("goroutine 1 [running]:\nmain.main()\n\t/srv/main.go:1\n"),
		expvars:    []byte("<html>application fallback</html>"),
	}, memoryProfileStore{})

	report, err := service.Diagnose(context.Background(), "https://service.test")
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	description := alertDescription(report.Alerts, "METRICS_UNAVAILABLE")
	if !strings.Contains(description, "invalid character '<'") {
		t.Fatalf("alert description = %q, want JSON parsing cause", description)
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
	if len(report.Goroutines.Alerts) != 0 {
		t.Fatalf("nested goroutine alerts = %#v, want only the top-level alert", report.Goroutines.Alerts)
	}
	if len(report.Suspicions) != 0 {
		t.Fatalf("suspicions = %#v, want no TLS health suspicion", report.Suspicions)
	}
}

type fakeTargetRepository struct {
	profiles        []byte
	goroutines      []byte
	expvars         []byte
	goroutineAlerts []domain.InspectionAlert
	expvarAlerts    []domain.InspectionAlert
	goroutineErr    error
}

type sequenceTargetRepository struct {
	goroutines  []byte
	expvars     [][]byte
	expvarCalls int
}

type failingSecondSampleRepository struct {
	sequenceTargetRepository
}

func (r *failingSecondSampleRepository) FetchExpvars(ctx context.Context, target string) (domain.FetchResult, error) {
	if r.expvarCalls > 0 {
		r.expvarCalls++
		return domain.FetchResult{}, fmt.Errorf("second sample unavailable")
	}
	return r.sequenceTargetRepository.FetchExpvars(ctx, target)
}

func hasAlert(alerts []domain.InspectionAlert, code string) bool {
	for _, alert := range alerts {
		if alert.Code == code {
			return true
		}
	}
	return false
}

func alertDescription(alerts []domain.InspectionAlert, code string) string {
	for _, alert := range alerts {
		if alert.Code == code {
			return alert.Description
		}
	}
	return ""
}

func (r *sequenceTargetRepository) FetchProfile(context.Context, domain.ProfileRequest) (domain.FetchResult, error) {
	return domain.FetchResult{}, nil
}

func (r *sequenceTargetRepository) FetchGoroutines(context.Context, string) (domain.FetchResult, error) {
	return domain.FetchResult{Data: r.goroutines}, nil
}

func (r *sequenceTargetRepository) FetchExpvars(context.Context, string) (domain.FetchResult, error) {
	index := r.expvarCalls
	r.expvarCalls++
	if index >= len(r.expvars) {
		index = len(r.expvars) - 1
	}
	return domain.FetchResult{Data: r.expvars[index]}, nil
}

func (r fakeTargetRepository) FetchProfile(_ context.Context, _ domain.ProfileRequest) (domain.FetchResult, error) {
	return domain.FetchResult{Data: r.profiles}, nil
}

func (r fakeTargetRepository) FetchGoroutines(_ context.Context, _ string) (domain.FetchResult, error) {
	if r.goroutineErr != nil {
		return domain.FetchResult{}, r.goroutineErr
	}
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
