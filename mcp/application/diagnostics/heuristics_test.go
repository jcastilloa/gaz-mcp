package diagnostics

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBuildMetricSnapshotPreservesRawDiagnosticMetrics(t *testing.T) {
	vars := metricVars(t, `{
  "collected_at":"2026-08-08T12:00:00Z",
  "runtime":{"gomaxprocs":4,"goroutines":{"runnable":8}},
  "process":{"open_fds":900,"fd_soft_limit":1000},
  "host":{"logical_cpus":4,"memory_available_bytes":1000},
  "cgroup":{"memory_current_bytes":900,"memory_max_bytes":1000},
  "db_pools":{"primary":{"in_use":10,"max_open_connections":10,"wait_count":3}}
}`)

	snapshot := metricSnapshotFromExpvars(vars)
	if snapshot.CollectedAt.IsZero() {
		t.Fatal("collected_at was not parsed")
	}
	if numberAt(snapshot.Process, "open_fds") != 900 {
		t.Fatalf("process metrics = %#v", snapshot.Process)
	}
	if numberAt(snapshot.Runtime, "goroutines", "runnable") != 8 {
		t.Fatalf("runtime metrics = %#v", snapshot.Runtime)
	}
	if numberAt(snapshot.Host, "logical_cpus") != 4 {
		t.Fatalf("host metrics = %#v", snapshot.Host)
	}
	if len(snapshot.DBPools) != 1 || snapshot.DBPools[0].Name != "primary" {
		t.Fatalf("db pools = %#v", snapshot.DBPools)
	}
}

func TestHeuristicsReturnSeparateSuspicionsWithStructuredEvidence(t *testing.T) {
	before := MetricSnapshot{
		CollectedAt: time.Unix(100, 0),
		Runtime:     metricMap(t, `{"gomaxprocs":4,"goroutines":{"runnable":10},"mutex_wait_seconds":1}`),
		Process:     metricMap(t, `{"open_fds":800,"fd_soft_limit":1000}`),
		Host:        metricMap(t, `{"memory_total_bytes":1000,"memory_available_bytes":200,"file_handles_allocated":700,"file_handles_max":1000,"cpu_pressure":{"some_avg10":5}}`),
		Cgroup:      metricMap(t, `{"memory_current_bytes":850,"memory_max_bytes":1000,"cpu_periods":100,"cpu_throttled_periods":10,"cpu_throttled_usec":100}`),
	}
	after := MetricSnapshot{
		CollectedAt: time.Unix(102, 0),
		Runtime:     metricMap(t, `{"gomaxprocs":4,"goroutines":{"runnable":12},"mutex_wait_seconds":1.8}`),
		Process:     metricMap(t, `{"open_fds":920,"fd_soft_limit":1000}`),
		Host:        metricMap(t, `{"memory_total_bytes":1000,"memory_available_bytes":80,"file_handles_allocated":900,"file_handles_max":1000,"cpu_pressure":{"some_avg10":25}}`),
		Cgroup:      metricMap(t, `{"memory_current_bytes":960,"memory_max_bytes":1000,"cpu_periods":120,"cpu_throttled_periods":16,"cpu_throttled_usec":40600}`),
	}

	suspicions := analyzeMetricSnapshots(before, after, GoroutineReport{})
	for _, code := range []string{FindingFDLimitPressure, FindingHostMemoryPressure, FindingHostCPUPressure, FindingSystemFileTablePressure, FindingCgroupMemoryPressure, FindingCPUThrottled, FindingSchedulerPressure, FindingMutexContention} {
		suspicion := suspicionByCode(t, suspicions, code)
		if suspicion.Confidence != ConfidenceSuspected || len(suspicion.Evidence) == 0 || len(suspicion.Criteria) == 0 || suspicion.Scope == "" || suspicion.Verify == "" {
			t.Fatalf("suspicion %s = %#v", code, suspicion)
		}
		for _, evidence := range suspicion.Evidence {
			if evidence.Metric == "" || evidence.Value == nil {
				t.Fatalf("evidence for %s = %#v", code, evidence)
			}
		}
		for _, criterion := range suspicion.Criteria {
			if criterion.Required && !criterion.Matched {
				t.Fatalf("required criterion for %s did not match: %#v", code, criterion)
			}
		}
	}
}

func TestConfirmedMutexContentionExposesCorroboratingStackCriterion(t *testing.T) {
	before := MetricSnapshot{
		ObservedAt: time.Unix(100, 0),
		Runtime:    metricMap(t, `{"mutex_wait_seconds":1}`),
	}
	after := MetricSnapshot{
		ObservedAt: time.Unix(102, 0),
		Runtime:    metricMap(t, `{"mutex_wait_seconds":2}`),
	}
	goroutines := GoroutineReport{Groups: []GoroutineGroup{{
		Count:    3,
		Findings: []Finding{{Code: FindingLockContention}},
	}}}

	suspicion := suspicionByCode(t, analyzeMetricSnapshots(before, after, goroutines), FindingMutexContention)
	if suspicion.Confidence != ConfidenceConfirmed || !hasRequiredCriterion(suspicion, "matching_mutex_waiters") {
		t.Fatalf("suspicion = %#v, want the evidence that raised confidence", suspicion)
	}
}

func TestCriticalPressureCriteriaExposeCriticalThreshold(t *testing.T) {
	after := MetricSnapshot{
		Host:    metricMap(t, `{"memory_total_bytes":1000,"memory_available_bytes":40,"file_handles_allocated":960,"file_handles_max":1000}`),
		Process: metricMap(t, `{"open_fds":960,"fd_soft_limit":1000}`),
		Cgroup:  metricMap(t, `{"memory_current_bytes":960,"memory_max_bytes":1000}`),
	}
	suspicions := analyzeMetricSnapshots(MetricSnapshot{}, after, GoroutineReport{})
	for _, code := range []string{FindingHostMemoryPressure, FindingSystemFileTablePressure, FindingFDLimitPressure, FindingCgroupMemoryPressure} {
		suspicion := suspicionByCode(t, suspicions, code)
		if suspicion.Severity != "critical" || !hasCriterionThreshold(suspicion, 0.95, 0.05) {
			t.Fatalf("suspicion %s = %#v, want its critical ratio threshold", code, suspicion)
		}
	}
}

func TestBurstHeuristicsDoNotClaimLeaks(t *testing.T) {
	before := MetricSnapshot{
		ObservedAt: time.Unix(100, 0),
		Process:    metricMap(t, `{"open_fds":100,"fd_soft_limit":1000,"rss_bytes":100000000}`),
		Runtime:    metricMap(t, `{"runtime_memory_bytes":50000000}`),
	}
	after := MetricSnapshot{
		ObservedAt: time.Unix(101, 0),
		Process:    metricMap(t, `{"open_fds":140,"fd_soft_limit":1000,"rss_bytes":150000000}`),
		Runtime:    metricMap(t, `{"runtime_memory_bytes":55000000}`),
	}

	suspicions := analyzeMetricSnapshots(before, after, GoroutineReport{})
	if !hasSuspicion(suspicions, FindingFDGrowthBurst) || hasSuspicion(suspicions, FindingPossibleFDLeak) {
		t.Fatalf("FD suspicions = %#v, want burst without leak", suspicions)
	}
	if !hasSuspicion(suspicions, FindingNativeMemoryGrowthBurst) || hasSuspicion(suspicions, FindingPossibleNativeMemoryLeak) {
		t.Fatalf("memory suspicions = %#v, want burst without leak", suspicions)
	}
}

func TestHeuristicsDoNotTreatHistoricalCountersAsCurrentActivity(t *testing.T) {
	before := MetricSnapshot{
		CollectedAt: time.Unix(100, 0),
		Cgroup:      metricMap(t, `{"cpu_periods":100,"cpu_throttled_periods":50,"cpu_throttled_usec":1000}`),
		DBPools:     []DBPoolStats{{Name: "primary", WaitCount: 99}},
	}
	after := MetricSnapshot{
		CollectedAt: time.Unix(102, 0),
		Cgroup:      metricMap(t, `{"cpu_periods":120,"cpu_throttled_periods":50,"cpu_throttled_usec":1000}`),
		DBPools:     []DBPoolStats{{Name: "primary", WaitCount: 99}},
	}

	suspicions := analyzeMetricSnapshots(before, after, GoroutineReport{})
	if hasSuspicion(suspicions, FindingCPUThrottled) || hasSuspicion(suspicions, FindingDBPoolExhausted) {
		t.Fatalf("suspicions = %#v, want no current throttling or DB wait", suspicions)
	}
}

func TestCounterDeltaPreservesLargeJSONIntegers(t *testing.T) {
	before := map[string]any{"counter": json.Number("9007199254740992")}
	after := map[string]any{"counter": json.Number("9007199254740993")}
	if delta := deltaAt(before, after, "counter"); delta != 1 {
		t.Fatalf("counter delta = %v, want 1", delta)
	}
}

func TestHeuristicsIgnoreMalformedNumericMetrics(t *testing.T) {
	after := MetricSnapshot{
		CollectedAt: time.Unix(102, 0),
		Host: metricMap(t, `{
			"memory_total_bytes":1000,
			"memory_available_bytes":"invalid",
			"file_handles_allocated":"invalid",
			"file_handles_max":1000
		}`),
	}
	suspicions := analyzeMetricSnapshots(MetricSnapshot{CollectedAt: time.Unix(100, 0)}, after, GoroutineReport{})
	if hasSuspicion(suspicions, FindingHostMemoryPressure) || hasSuspicion(suspicions, FindingSystemFileTablePressure) {
		t.Fatalf("malformed metrics produced suspicions: %#v", suspicions)
	}
}

func TestSchedulerPressureRequiresBothSamples(t *testing.T) {
	before := MetricSnapshot{
		CollectedAt: time.Unix(100, 0),
		Runtime:     metricMap(t, `{"gomaxprocs":4,"goroutines":{"runnable":1}}`),
	}
	after := MetricSnapshot{
		CollectedAt: time.Unix(102, 0),
		Runtime:     metricMap(t, `{"gomaxprocs":4,"goroutines":{"runnable":8}}`),
	}
	if suspicions := analyzeMetricSnapshots(before, after, GoroutineReport{}); hasSuspicion(suspicions, FindingSchedulerPressure) {
		t.Fatalf("transient runnable spike produced scheduler pressure: %#v", suspicions)
	}
}

func TestCPUThrottlingIgnoresNegligibleThrottleWithoutSchedulerPressure(t *testing.T) {
	before := MetricSnapshot{
		CollectedAt: time.Unix(100, 0),
		Cgroup:      metricMap(t, `{"cpu_periods":100,"cpu_throttled_periods":10,"cpu_throttled_usec":100}`),
	}
	after := MetricSnapshot{
		CollectedAt: time.Unix(102, 0),
		Cgroup:      metricMap(t, `{"cpu_periods":120,"cpu_throttled_periods":12,"cpu_throttled_usec":101}`),
	}
	if suspicions := analyzeMetricSnapshots(before, after, GoroutineReport{}); hasSuspicion(suspicions, FindingCPUThrottled) {
		t.Fatalf("negligible throttling produced suspicion: %#v", suspicions)
	}
}

func TestCgroupOOMEventIsReportedAfterMemoryDrops(t *testing.T) {
	before := MetricSnapshot{
		CollectedAt: time.Unix(100, 0),
		Cgroup:      metricMap(t, `{"memory_events":{"oom":0,"oom_kill":0}}`),
	}
	after := MetricSnapshot{
		CollectedAt: time.Unix(102, 0),
		Cgroup:      metricMap(t, `{"memory_events":{"oom":1,"oom_kill":1}}`),
	}
	suspicion := suspicionByCode(t, analyzeMetricSnapshots(before, after, GoroutineReport{}), FindingCgroupMemoryPressure)
	if suspicion.Severity != "critical" {
		t.Fatalf("suspicion = %#v, want critical OOM evidence", suspicion)
	}
	for _, evidence := range suspicion.Evidence {
		if evidence.Value == nil {
			t.Fatalf("suspicion includes unavailable evidence: %#v", suspicion)
		}
	}
}

func TestHeuristicsDoNotTreatMetricsMissingFromBaselineAsGrowth(t *testing.T) {
	before := MetricSnapshot{CollectedAt: time.Unix(100, 0), Runtime: map[string]any{}, Process: map[string]any{}, Cgroup: map[string]any{}}
	after := MetricSnapshot{
		CollectedAt: time.Unix(102, 0),
		Runtime:     metricMap(t, `{"gc_cpu_seconds":5,"user_cpu_seconds":5,"heap_allocated_bytes_total":100000000,"mutex_wait_seconds":2}`),
		Process:     metricMap(t, `{"rss_bytes":100000000,"open_fds":40,"fd_soft_limit":1000}`),
		Cgroup:      metricMap(t, `{"cpu_periods":100,"cpu_throttled_periods":90,"cpu_throttled_usec":1000}`),
		DBPools:     []DBPoolStats{{Name: "primary", InUse: 10, MaxOpenConnections: 10, WaitCount: 99}},
	}

	suspicions := analyzeMetricSnapshots(before, after, GoroutineReport{})
	for _, code := range []string{FindingFDGrowthBurst, FindingNativeMemoryGrowthBurst, FindingCPUThrottled, FindingGCPressure, FindingMutexContention, FindingDBPoolExhausted} {
		if hasSuspicion(suspicions, code) {
			t.Errorf("%s should require the metric in both samples: %#v", code, suspicions)
		}
	}
}

func TestDBPoolSuspicionIsConfirmedOnlyWithCurrentWaitersAndSaturation(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatal(err)
	}
	before := MetricSnapshot{CollectedAt: time.Unix(100, 0), DBPools: []DBPoolStats{{Name: "primary", WaitCount: 5}}}
	after := MetricSnapshot{CollectedAt: time.Unix(102, 0), DBPools: []DBPoolStats{{Name: "primary", InUse: 10, MaxOpenConnections: 10, WaitCount: 8}}}

	suspicion := suspicionByCode(t, analyzeMetricSnapshots(before, after, goroutines), FindingDBPoolExhausted)
	if suspicion.Confidence != ConfidenceConfirmed {
		t.Fatalf("confidence = %q, want confirmed", suspicion.Confidence)
	}
	if len(suspicion.Evidence) < 3 {
		t.Fatalf("evidence = %#v, want stacks, saturation and wait delta", suspicion.Evidence)
	}
	if !hasRequiredCriterion(suspicion, "unique_active_saturated_pool") {
		t.Fatalf("criteria = %#v, want the attribution condition that raised confidence", suspicion.Criteria)
	}
}

func TestDBPoolSaturationWithWaitersIsPreservedWithoutCounterGrowth(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatal(err)
	}
	before := MetricSnapshot{ObservedAt: time.Unix(100, 0), DBPools: []DBPoolStats{{Name: "primary", WaitCount: 5}}}
	after := MetricSnapshot{ObservedAt: time.Unix(102, 0), DBPools: []DBPoolStats{{
		Name: "primary", InUse: 10, MaxOpenConnections: 10, WaitCount: 5,
	}}}

	suspicion := suspicionByCode(t, analyzeMetricSnapshots(before, after, goroutines), FindingDBPoolExhausted)
	if suspicion.Confidence != ConfidenceSuspected || suspicion.Severity != "critical" || !strings.Contains(suspicion.Reason, "saturated") {
		t.Fatalf("suspicion = %#v, want current per-pool saturation preserved without claiming confirmation", suspicion)
	}
}

func TestDBPoolSuspicionIsNotConfirmedWhenWaitersCannotBeAttributedAcrossPools(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatal(err)
	}
	before := MetricSnapshot{CollectedAt: time.Unix(100, 0), DBPools: []DBPoolStats{{Name: "primary"}, {Name: "analytics"}}}
	after := MetricSnapshot{CollectedAt: time.Unix(102, 0), DBPools: []DBPoolStats{
		{Name: "primary", InUse: 10, MaxOpenConnections: 10, WaitCount: 2},
		{Name: "analytics", InUse: 5, MaxOpenConnections: 5, WaitCount: 3},
	}}

	suspicions := analyzeMetricSnapshots(before, after, goroutines)
	for _, suspicion := range suspicions {
		if suspicion.Code == FindingDBPoolExhausted && suspicion.Confidence == ConfidenceConfirmed {
			t.Fatalf("suspicion = %#v, waiters cannot be attributed to one of multiple pools", suspicion)
		}
	}
}

func TestDBPoolSuspicionIsConfirmedForSingleActiveCandidateAcrossMultiplePools(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatal(err)
	}
	before := MetricSnapshot{ObservedAt: time.Unix(100, 0), DBPools: []DBPoolStats{
		{Name: "primary", WaitCount: 5, WaitDuration: time.Second},
		{Name: "analytics", WaitCount: 3, WaitDuration: time.Second},
	}}
	after := MetricSnapshot{ObservedAt: time.Unix(102, 0), DBPools: []DBPoolStats{
		{Name: "primary", InUse: 10, MaxOpenConnections: 10, WaitCount: 8, WaitDuration: 4 * time.Second},
		{Name: "analytics", InUse: 1, Idle: 4, MaxOpenConnections: 5, WaitCount: 3, WaitDuration: time.Second},
	}}

	suspicion := suspicionByCode(t, analyzeMetricSnapshots(before, after, goroutines), FindingDBPoolExhausted)
	if suspicion.Confidence != ConfidenceConfirmed || suspicion.Scope != ScopeDatabasePool {
		t.Fatalf("suspicion = %#v, want confirmed primary pool", suspicion)
	}
	if !hasEvidence(suspicion, "db_pools.primary.wait_duration_ns.delta", int64(3*time.Second)) {
		t.Fatalf("evidence = %#v, want wait duration delta", suspicion.Evidence)
	}
}

func TestDBPoolSuspicionRemainsSuspectedWithSecondSaturatedActivePool(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatal(err)
	}
	before := MetricSnapshot{ObservedAt: time.Unix(100, 0), DBPools: []DBPoolStats{
		{Name: "primary", WaitCount: 5, WaitDuration: time.Second},
		{Name: "analytics", WaitCount: 3, WaitDuration: time.Second},
	}}
	after := MetricSnapshot{ObservedAt: time.Unix(102, 0), DBPools: []DBPoolStats{
		{Name: "primary", InUse: 10, MaxOpenConnections: 10, WaitCount: 8, WaitDuration: time.Second},
		{Name: "analytics", InUse: 5, MaxOpenConnections: 5, WaitCount: 3, WaitDuration: 3 * time.Second},
	}}

	for _, suspicion := range analyzeMetricSnapshots(before, after, goroutines) {
		if suspicion.Code == FindingDBPoolExhausted && suspicion.Confidence == ConfidenceConfirmed {
			t.Fatalf("suspicion = %#v, stacks are ambiguous across two saturated pools with current-window activity", suspicion)
		}
	}
}

func TestDBPoolConfirmationDoesNotRequireCompletedWaitDuration(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatal(err)
	}
	before := MetricSnapshot{ObservedAt: time.Unix(100, 0), DBPools: []DBPoolStats{
		{Name: "primary", WaitCount: 5, WaitDuration: time.Second},
		{Name: "analytics", WaitCount: 3},
	}}
	after := MetricSnapshot{ObservedAt: time.Unix(102, 0), DBPools: []DBPoolStats{
		{Name: "primary", InUse: 10, MaxOpenConnections: 10, WaitCount: 8, WaitDuration: time.Second},
		{Name: "analytics", InUse: 1, Idle: 4, MaxOpenConnections: 5, WaitCount: 3},
	}}

	suspicion := suspicionByCode(t, analyzeMetricSnapshots(before, after, goroutines), FindingDBPoolExhausted)
	if suspicion.Confidence != ConfidenceConfirmed {
		t.Fatalf("suspicion = %#v, in-flight waits need not increase WaitDuration yet", suspicion)
	}
	for _, criterion := range suspicion.Criteria {
		if criterion.Required && !criterion.Matched {
			t.Fatalf("required criterion did not match: %#v", criterion)
		}
	}
}

func TestDBPoolCompletedWaitActivityDoesNotRequireNewWaitCount(t *testing.T) {
	before := MetricSnapshot{ObservedAt: time.Unix(100, 0), DBPools: []DBPoolStats{
		{Name: "primary", WaitCount: 5, WaitDuration: time.Second},
	}}
	after := MetricSnapshot{ObservedAt: time.Unix(102, 0), DBPools: []DBPoolStats{
		{Name: "primary", InUse: 1, Idle: 4, MaxOpenConnections: 10, WaitCount: 5, WaitDuration: 3 * time.Second},
	}}

	suspicion := suspicionByCode(t, analyzeMetricSnapshots(before, after, GoroutineReport{}), FindingDBPoolExhausted)
	if suspicion.Confidence != ConfidenceSuspected {
		t.Fatalf("suspicion = %#v, completed prior wait is evidence but not confirmation", suspicion)
	}
	if suspicion.Severity != "warning" || !strings.Contains(suspicion.Reason, "completed wait time") {
		t.Fatalf("suspicion = %#v, completed waits without current saturation must not claim a critical current incident", suspicion)
	}
	for _, criterion := range suspicion.Criteria {
		if criterion.Required && !criterion.Matched {
			t.Fatalf("required criterion did not match: %#v", criterion)
		}
	}
}

func TestTrendSuspicionsExposeEveryRequiredDecisionCriterion(t *testing.T) {
	metrics := []MetricSnapshot{
		trendMetricSample(time.Unix(100, 0), 100, 100<<20, 50<<20),
		trendMetricSample(time.Unix(105, 0), 120, 140<<20, 55<<20),
		trendMetricSample(time.Unix(110, 0), 145, 180<<20, 60<<20),
	}
	goroutines := goroutineTrendSamples(t, []time.Time{time.Unix(100, 0), time.Unix(105, 0), time.Unix(110, 0)}, 10, 15, 20)
	suspicions := trendSuspicions(metrics, goroutines)

	fd := suspicionByCode(t, suspicions, FindingPossibleFDLeak)
	for _, name := range []string{"sample_count", "observed_duration", "positive_slope", "net_growth", "relative_growth", "positive_interval_fraction"} {
		if !hasRequiredCriterion(fd, name) {
			t.Errorf("FD criteria = %#v, want required %q", fd.Criteria, name)
		}
	}
	native := suspicionByCode(t, suspicions, FindingPossibleNativeMemoryLeak)
	if !hasRequiredCriterion(native, "runtime_to_rss_net_growth_ratio") {
		t.Errorf("native memory criteria = %#v, want runtime corroboration", native.Criteria)
	}
	goroutine := suspicionByCode(t, suspicions, FindingPossibleGoroutineLeak)
	for _, name := range []string{"sample_count", "all_samples_complete", "observed_duration", "fingerprint_net_growth", "positive_interval_fraction", "blocked_duration"} {
		if !hasRequiredCriterion(goroutine, name) {
			t.Errorf("goroutine criteria = %#v, want required %q", goroutine.Criteria, name)
		}
	}
}

func TestDBPoolWaiterSuspicionIsPreservedWhenPublishedPoolsDoNotConfirmIt(t *testing.T) {
	goroutines, err := AnalyzeGoroutines(blockedRequests)
	if err != nil {
		t.Fatal(err)
	}
	before := MetricSnapshot{CollectedAt: time.Unix(100, 0), DBPools: []DBPoolStats{{Name: "primary", WaitCount: 5}}}
	after := MetricSnapshot{CollectedAt: time.Unix(102, 0), DBPools: []DBPoolStats{{Name: "primary", InUse: 1, Idle: 4, MaxOpenConnections: 10, WaitCount: 5}}}
	suspicion := suspicionByCode(t, analyzeMetricSnapshots(before, after, goroutines), FindingDBPoolExhausted)
	if suspicion.Confidence != ConfidenceSuspected || !strings.Contains(suspicion.Reason, "do not identify") {
		t.Fatalf("suspicion = %#v, want preserved unconfirmed stack evidence", suspicion)
	}
}

func TestMetricSnapshotGracefullyHandlesStandardExpvarOnly(t *testing.T) {
	vars := map[string]any{"num_goroutine": json.Number("3")}
	snapshot := metricSnapshotFromExpvars(vars)
	if snapshot.Runtime == nil || numberAt(snapshot.Runtime, "goroutines", "total") != 3 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.Process == nil || snapshot.Host == nil || snapshot.Cgroup == nil || snapshot.DBPools == nil {
		t.Fatalf("optional metrics must be empty rather than nil: %#v", snapshot)
	}
}

func TestMetricSnapshotUsesEmptyPoolArrayWhenDiagnosticsHasNoPools(t *testing.T) {
	snapshot := metricSnapshotFromExpvars(metricVars(t, `{}`))
	if snapshot.DBPools == nil {
		t.Fatalf("db pools = %#v, want an empty array rather than null", snapshot.DBPools)
	}
}

func metricVars(t *testing.T, diagnostics string) map[string]any {
	t.Helper()
	return metricMap(t, `{"gaz_diagnostics":`+diagnostics+`}`)
}

func metricMap(t *testing.T, data string) map[string]any {
	t.Helper()
	vars, err := parseExpvars([]byte(data))
	if err != nil {
		t.Fatalf("parse metrics: %v", err)
	}
	return vars
}

func suspicionByCode(t *testing.T, suspicions []Suspicion, code string) Suspicion {
	t.Helper()
	for _, suspicion := range suspicions {
		if suspicion.Code == code {
			return suspicion
		}
	}
	t.Fatalf("suspicion %s not found in %#v", code, suspicions)
	return Suspicion{}
}

func hasEvidence(suspicion Suspicion, metric string, value any) bool {
	for _, item := range suspicion.Evidence {
		if item.Metric == metric && fmt.Sprint(item.Value) == fmt.Sprint(value) {
			return true
		}
	}
	return false
}

func hasCriterionThreshold(suspicion Suspicion, thresholds ...float64) bool {
	for _, item := range suspicion.Criteria {
		for _, threshold := range thresholds {
			if fmt.Sprint(item.Threshold) == fmt.Sprint(threshold) {
				return true
			}
		}
	}
	return false
}

func hasRequiredCriterion(suspicion Suspicion, name string) bool {
	for _, item := range suspicion.Criteria {
		if item.Name == name {
			return item.Required && item.Matched
		}
	}
	return false
}
