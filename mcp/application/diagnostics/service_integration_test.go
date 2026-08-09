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
	repository := &sequenceTargetRepository{
		goroutines: []byte(blockedRequests),
		expvars: [][]byte{
			[]byte(`{
  "num_goroutine": 1238,
  "gaz_diagnostics": {"db_pools": {"primary": {
    "open_connections": 100, "in_use": 100, "idle": 0,
    "max_open_connections": 100, "wait_count": 15338, "wait_duration_ns": 510000000000
  }}}
}`),
			[]byte(`{
  "num_goroutine": 1240,
  "memstats": {"HeapAlloc": 1234, "NumGC": 8},
  "gaz_diagnostics": {"db_pools": {"primary": {
    "open_connections": 100, "in_use": 100, "idle": 0,
    "max_open_connections": 100, "wait_count": 15340, "wait_duration_ns": 512000000000
  }}}
}`),
		},
	}
	service := NewService(repository, memoryProfileStore{})
	service.wait = func(context.Context, time.Duration) error { return nil }

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
	observedTimes := []time.Time{time.Unix(199, 0), time.Unix(200, 0), time.Unix(201, 0)}
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
	if !hasSuspicion(report.Suspicions, FindingFDLimitPressure) || !hasSuspicion(report.Suspicions, FindingFDGrowthBurst) {
		t.Fatalf("suspicions = %#v", report.Suspicions)
	}
}

func TestServiceDiagnoseWithOptionsDetectsSustainedGrowth(t *testing.T) {
	repository := &sequenceTargetRepository{
		goroutineSequence: [][]byte{
			[]byte(goroutineGroupDump(10)),
			[]byte(goroutineGroupDump(15)),
			[]byte(goroutineGroupDump(20)),
		},
		expvars: [][]byte{
			[]byte(metricDump(100, 100<<20, 50<<20, 4, 4, 1000)),
			[]byte(metricDump(120, 140<<20, 55<<20, 4, 4, 1000)),
			[]byte(metricDump(145, 180<<20, 60<<20, 6, 3, 900)),
		},
	}
	service := NewService(repository, memoryProfileStore{})
	service.wait = func(context.Context, time.Duration) error { return nil }
	times := []time.Time{
		time.Unix(99, 0), time.Unix(100, 0),
		time.Unix(103, 0), time.Unix(104, 0),
		time.Unix(109, 0), time.Unix(110, 0),
	}
	service.now = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}

	report, err := service.DiagnoseWithOptions(context.Background(), "http://service:6060", DiagnosisOptions{Samples: 3, Interval: time.Second})
	if err != nil {
		t.Fatalf("diagnose: %v", err)
	}
	for _, code := range []string{FindingPossibleFDLeak, FindingPossibleNativeMemoryLeak, FindingPossibleGoroutineLeak} {
		if !hasSuspicion(report.Suspicions, code) {
			t.Errorf("suspicions = %#v, want %s", report.Suspicions, code)
		}
	}
	if len(report.Metrics.Samples) != 3 || report.Metrics.IntervalJitterNs == nil || *report.Metrics.IntervalJitterNs != int64(2*time.Second) {
		t.Fatalf("metric window = %#v, want samples and real interval jitter", report.Metrics)
	}
	if len(report.GoroutineSamples) != 3 {
		t.Fatalf("goroutine samples = %#v, want three", report.GoroutineSamples)
	}
	if report.GoroutineIntervalJitterNs == nil || *report.GoroutineIntervalJitterNs != int64(2*time.Second) {
		t.Fatalf("goroutine interval jitter = %#v, want source-specific 2s jitter", report.GoroutineIntervalJitterNs)
	}
	if !report.GoroutineSamples[0].ObservedAt.Equal(time.Unix(99, 0)) ||
		!report.Metrics.Samples[0].ObservedAt.Equal(time.Unix(100, 0)) {
		t.Fatalf("observation times = goroutines:%s metrics:%s, want source-specific timestamps", report.GoroutineSamples[0].ObservedAt, report.Metrics.Samples[0].ObservedAt)
	}
	if len(report.GoroutineSamples[0].Groups) == 0 || report.GoroutineSamples[0].Groups[0].Fingerprint == "" {
		t.Fatalf("goroutine samples = %#v, want compact fingerprint series", report.GoroutineSamples)
	}
	if len(report.StructuralChanges) != 3 {
		t.Fatalf("structural changes = %#v, want gomaxprocs, pids and DB max changes", report.StructuralChanges)
	}
}

func TestDiagnosisOptionsRejectExcessiveSampling(t *testing.T) {
	service := NewService(fakeTargetRepository{}, memoryProfileStore{})
	_, err := service.DiagnoseWithOptions(context.Background(), "http://service:6060", DiagnosisOptions{Samples: 10, Interval: 10 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "60 seconds") {
		t.Fatalf("error = %v, want total duration limit", err)
	}
}

func TestTrendAnalysisRejectsInitialJumpAndPlateau(t *testing.T) {
	samples := []MetricSnapshot{
		trendMetricSample(time.Unix(100, 0), 100, 100<<20, 50<<20),
		trendMetricSample(time.Unix(101, 0), 140, 150<<20, 50<<20),
		trendMetricSample(time.Unix(102, 0), 140, 150<<20, 50<<20),
		trendMetricSample(time.Unix(103, 0), 140, 150<<20, 50<<20),
	}
	for _, suspicion := range trendSuspicions(samples, nil) {
		if suspicion.Code == FindingPossibleFDLeak || suspicion.Code == FindingPossibleNativeMemoryLeak {
			t.Fatalf("suspicions = %#v, jump followed by plateau is not sustained growth", trendSuspicions(samples, nil))
		}
	}
}

func TestTrendAnalysisRequiresMinimumObservedDuration(t *testing.T) {
	samples := []MetricSnapshot{
		trendMetricSample(time.Unix(100, 0), 100, 100<<20, 50<<20),
		trendMetricSample(time.Unix(101, 0), 120, 140<<20, 55<<20),
		trendMetricSample(time.Unix(102, 0), 145, 180<<20, 60<<20),
	}
	if suspicions := trendSuspicions(samples, nil); len(suspicions) != 0 {
		t.Fatalf("suspicions = %#v, two observed seconds are too short for a possible leak", suspicions)
	}
}

func TestDefaultDiagnosisReportsStructuralChanges(t *testing.T) {
	repository := &sequenceTargetRepository{
		goroutines: []byte("goroutine 1 [running]:\nmain.main()\n\t/srv/main.go:1\n"),
		expvars: [][]byte{
			[]byte(`{"gaz_diagnostics":{"runtime":{"gomaxprocs":4}}}`),
			[]byte(`{"gaz_diagnostics":{"runtime":{"gomaxprocs":6}}}`),
		},
	}
	service := NewService(repository, memoryProfileStore{})
	service.wait = func(context.Context, time.Duration) error { return nil }

	report, err := service.Diagnose(context.Background(), "http://service:6060")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.StructuralChanges) != 1 || report.StructuralChanges[0].Metric != "runtime.gomaxprocs" {
		t.Fatalf("structural changes = %#v, want default-window GOMAXPROCS change", report.StructuralChanges)
	}
}

func TestGoroutineTrendRequiresCompleteGrowingFingerprint(t *testing.T) {
	times := []time.Time{time.Unix(100, 0), time.Unix(101, 0), time.Unix(102, 0)}
	stable := make([]GoroutineReport, 0, len(times))
	for _, observedAt := range times {
		report, err := AnalyzeGoroutines(goroutineGroupDump(10))
		if err != nil {
			t.Fatal(err)
		}
		report.ObservedAt = observedAt
		stable = append(stable, report)
	}
	if _, ok := goroutineGrowthSuspicion(stable); ok {
		t.Fatal("stable fingerprint was classified as a possible leak")
	}

	stable[1].BlocksDiscarded = 1
	stable[1].Completeness = "partial"
	stable[2].Groups[0].Count = 30
	if _, ok := goroutineGrowthSuspicion(stable); ok {
		t.Fatal("partial goroutine series was classified as a possible leak")
	}
}

func TestGoroutineTrendRequiresMinimumObservedDuration(t *testing.T) {
	samples := goroutineTrendSamples(t, []time.Time{time.Unix(100, 0), time.Unix(101, 0), time.Unix(102, 0)}, 10, 15, 20)
	if _, ok := goroutineGrowthSuspicion(samples); ok {
		t.Fatal("two observed seconds are too short for a possible goroutine leak")
	}
}

func TestSeriesReportKeepsGoroutineTrendWhenMetricsAreUnavailable(t *testing.T) {
	samples := goroutineTrendSamples(t, []time.Time{time.Unix(100, 0), time.Unix(105, 0), time.Unix(110, 0)}, 10, 15, 20)
	report := buildSeriesReport(nil, samples, nil)
	if !hasSuspicion(report.Suspicions, FindingPossibleGoroutineLeak) {
		t.Fatalf("suspicions = %#v, want goroutine trend independent of metric availability", report.Suspicions)
	}
}

func TestServiceSeriesKeepsGoroutineTrendWhenEveryMetricSampleFails(t *testing.T) {
	repository := &failingMetricSeriesRepository{sequenceTargetRepository: sequenceTargetRepository{
		goroutineSequence: [][]byte{
			[]byte(goroutineGroupDump(10)),
			[]byte(goroutineGroupDump(15)),
			[]byte(goroutineGroupDump(20)),
		},
	}}
	service := NewService(repository, memoryProfileStore{})
	service.wait = func(context.Context, time.Duration) error { return nil }
	times := []time.Time{time.Unix(100, 0), time.Unix(105, 0), time.Unix(110, 0)}
	service.now = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}

	report, err := service.DiagnoseWithOptions(context.Background(), "http://service:6060", DiagnosisOptions{Samples: 3, Interval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !hasSuspicion(report.Suspicions, FindingPossibleGoroutineLeak) || !hasAlert(report.Alerts, "METRIC_SAMPLE_UNAVAILABLE") {
		t.Fatalf("report = %#v, want independent goroutine trend and metric availability alert", report)
	}
	if report.GoroutineIntervalJitterNs == nil || *report.GoroutineIntervalJitterNs != 0 {
		t.Fatalf("goroutine interval jitter = %#v, want explicit zero even without metric samples", report.GoroutineIntervalJitterNs)
	}
}

func TestSeriesReportReturnsEveryGrowingFingerprintDeterministically(t *testing.T) {
	samples := []GoroutineReport{
		goroutineFingerprintSample(time.Unix(100, 0), map[string]int{"zeta": 10, "alpha": 10}),
		goroutineFingerprintSample(time.Unix(105, 0), map[string]int{"zeta": 15, "alpha": 15}),
		goroutineFingerprintSample(time.Unix(110, 0), map[string]int{"zeta": 20, "alpha": 20}),
	}
	report := buildSeriesReport(nil, samples, nil)
	var fingerprints []string
	for _, suspicion := range report.Suspicions {
		if suspicion.Code == FindingPossibleGoroutineLeak {
			fingerprints = append(fingerprints, fmt.Sprint(suspicion.Evidence[0].Value))
		}
	}
	if !reflect.DeepEqual(fingerprints, []string{"alpha", "zeta"}) {
		t.Fatalf("fingerprints = %#v, want every growing group in deterministic order", fingerprints)
	}
}

func TestSeriesReportSuppressesOnlyMatchingLongLivedGroup(t *testing.T) {
	samples := []GoroutineReport{
		goroutineFingerprintSample(time.Unix(100, 0), map[string]int{"growing": 10, "stable": 10}),
		goroutineFingerprintSample(time.Unix(105, 0), map[string]int{"growing": 15, "stable": 10}),
		goroutineFingerprintSample(time.Unix(110, 0), map[string]int{"growing": 20, "stable": 10}),
	}
	report := buildSeriesReport(nil, samples, nil)
	if countSuspicions(report.Suspicions, FindingPossibleGoroutineLeak) != 1 ||
		countSuspicions(report.Suspicions, FindingLongLivedGoroutineGroup) != 1 {
		t.Fatalf("suspicions = %#v, want growing leak plus independent stable long-lived group", report.Suspicions)
	}
}

func TestSeriesReportUsesEmptyGoroutineGroupsWhenSamplesAreUnavailable(t *testing.T) {
	report := buildSeriesReport([]MetricSnapshot{trendMetricSample(time.Unix(100, 0), 1, 1, 1)}, nil, nil)
	if report.Goroutines.Groups == nil || report.Goroutines.Completeness != GoroutineCompletenessUnavailable {
		t.Fatalf("goroutines = %#v, want explicit unavailable report with an empty groups array", report.Goroutines)
	}
}

func TestAppendingGoroutineSamplesRetainsFullDetailsOnlyForLatestCapture(t *testing.T) {
	first := goroutineFingerprintSample(time.Unix(100, 0), map[string]int{"first": 10})
	second := goroutineFingerprintSample(time.Unix(105, 0), map[string]int{"second": 10})
	samples := appendGoroutineSample(nil, first)
	samples = appendGoroutineSample(samples, second)
	if samples[0].Groups[0].Stack != nil || samples[0].Groups[0].Findings != nil {
		t.Fatalf("older sample = %#v, want compact trend-only details", samples[0])
	}
	if len(samples[1].Groups[0].Stack) == 0 || len(samples[1].Groups[0].Findings) == 0 {
		t.Fatalf("latest sample = %#v, want full diagnostic details", samples[1])
	}
}

func TestMetricTrendRejectsNonIncreasingObservationTimes(t *testing.T) {
	samples := []MetricSnapshot{
		trendMetricSample(time.Unix(100, 0), 100, 100<<20, 50<<20),
		trendMetricSample(time.Unix(100, 0), 120, 140<<20, 55<<20),
		trendMetricSample(time.Unix(110, 0), 145, 180<<20, 60<<20),
	}
	if suspicions := trendSuspicions(samples, nil); len(suspicions) != 0 {
		t.Fatalf("suspicions = %#v, non-increasing observation times are not comparable", suspicions)
	}
}

func TestStructuralChangesIncludeTransientChanges(t *testing.T) {
	samples := []MetricSnapshot{
		{ObservedAt: time.Unix(100, 0), Runtime: map[string]any{"gomaxprocs": 4}},
		{ObservedAt: time.Unix(101, 0), Runtime: map[string]any{"gomaxprocs": 6}},
		{ObservedAt: time.Unix(102, 0), Runtime: map[string]any{"gomaxprocs": 4}},
	}
	changes := structuralChanges(samples)
	if len(changes) != 2 || changes[0].ObservedAt.IsZero() || changes[1].ObservedAt.IsZero() {
		t.Fatalf("changes = %#v, want both transient GOMAXPROCS changes with timestamps", changes)
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
	goroutines        []byte
	goroutineSequence [][]byte
	expvars           [][]byte
	expvarCalls       int
	goroutineCalls    int
}

type failingSecondSampleRepository struct {
	sequenceTargetRepository
}

type failingMetricSeriesRepository struct {
	sequenceTargetRepository
}

func (r *failingMetricSeriesRepository) FetchExpvars(context.Context, string) (domain.FetchResult, error) {
	r.expvarCalls++
	return domain.FetchResult{}, fmt.Errorf("metrics unavailable")
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
	if len(r.goroutineSequence) > 0 {
		index := r.goroutineCalls
		r.goroutineCalls++
		if index >= len(r.goroutineSequence) {
			index = len(r.goroutineSequence) - 1
		}
		return domain.FetchResult{Data: r.goroutineSequence[index]}, nil
	}
	return domain.FetchResult{Data: r.goroutines}, nil
}

func goroutineGroupDump(count int) string {
	var dump strings.Builder
	for index := range count {
		fmt.Fprintf(&dump, "goroutine %d [chan receive, 6 minutes]:\nmain.worker()\n\t/srv/main.go:10\n\n", index+1)
	}
	return dump.String()
}

func metricDump(openFDs, rss, runtimeMemory, gomaxprocs, pids, maxConnections int) string {
	return fmt.Sprintf(`{"gaz_diagnostics":{"runtime":{"runtime_memory_bytes":%d,"gomaxprocs":%d},"process":{"open_fds":%d,"fd_soft_limit":1000,"rss_bytes":%d},"cgroup":{"pids_current":1,"pids_max":%d},"db_pools":{"primary":{"max_open_connections":%d}},"custom":{}}}`, runtimeMemory, gomaxprocs, openFDs, rss, pids, maxConnections)
}

func trendMetricSample(observedAt time.Time, openFDs, rss, runtimeMemory int) MetricSnapshot {
	return MetricSnapshot{
		ObservedAt: observedAt,
		Process:    map[string]any{"open_fds": openFDs, "rss_bytes": rss},
		Runtime:    map[string]any{"runtime_memory_bytes": runtimeMemory},
	}
}

func goroutineTrendSamples(t *testing.T, times []time.Time, counts ...int) []GoroutineReport {
	t.Helper()
	samples := make([]GoroutineReport, 0, len(times))
	for index, observedAt := range times {
		report, err := AnalyzeGoroutines(goroutineGroupDump(counts[index]))
		if err != nil {
			t.Fatal(err)
		}
		report.ObservedAt = observedAt
		samples = append(samples, report)
	}
	return samples
}

func goroutineFingerprintSample(observedAt time.Time, counts map[string]int) GoroutineReport {
	groups := make([]GoroutineGroup, 0, len(counts))
	for fingerprint, count := range counts {
		groups = append(groups, GoroutineGroup{
			Fingerprint: fingerprint,
			Count:       count,
			State:       "chan receive",
			BlockedFor:  6 * time.Minute,
			Stack:       []string{"main." + fingerprint + "()"},
			Findings: []Finding{{
				Code: FindingLongLivedGoroutineGroup, Severity: "warning",
				Description: "long-lived group", Hint: "verify ownership",
			}},
		})
	}
	return GoroutineReport{
		ObservedAt: observedAt, Total: len(groups), BlocksParsed: len(groups),
		Completeness: GoroutineCompletenessComplete, Groups: groups,
	}
}

func countSuspicions(suspicions []Suspicion, code string) int {
	count := 0
	for _, suspicion := range suspicions {
		if suspicion.Code == code {
			count++
		}
	}
	return count
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
