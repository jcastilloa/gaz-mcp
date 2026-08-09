package diagnostics

import (
	"fmt"
	"math"
	"sort"
	"time"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

const (
	minimumTrendSamples             = 3
	minimumTrendDuration            = 10 * time.Second
	minimumPositiveIntervalFraction = 0.6
	minimumGoroutineGrowth          = 10
)

type trendSuspicionSpec struct {
	code                  string
	scope                 string
	reason                string
	metric                string
	minimumGrowth         float64
	minimumRelativeGrowth float64
	unit                  string
	verify                string
}

func buildSeriesReport(metrics []MetricSnapshot, goroutines []GoroutineReport, alerts []domain.InspectionAlert) DiagnosisReport {
	report := DiagnosisReport{
		GoroutineSamples:          compactGoroutineSamples(goroutines),
		GoroutineIntervalJitterNs: goroutineIntervalJitter(goroutines),
		Alerts:                    alerts,
	}
	if len(goroutines) > 0 {
		report.Goroutines = goroutines[len(goroutines)-1]
	} else {
		report.Goroutines = unavailableGoroutineReport()
	}
	if len(metrics) == 0 {
		report.Metrics = metricWindow(emptyMetricSnapshot(), emptyMetricSnapshot())
	} else {
		report.Metrics = metricSeries(metrics)
	}
	report.StructuralChanges = structuralChanges(metrics)
	report.Suspicions = analyzeMetricSnapshots(report.Metrics.Before, report.Metrics.After, report.Goroutines)
	trends := trendSuspicions(metrics, goroutines)
	report.Suspicions = suppressBurstSuspicions(report.Suspicions, trends)
	report.Suspicions = append(report.Suspicions, trends...)
	sort.SliceStable(report.Suspicions, func(i, j int) bool {
		return severityRank(report.Suspicions[i].Severity) > severityRank(report.Suspicions[j].Severity)
	})
	clearAllGoroutineFindings(&report)
	return report
}

func metricSeries(samples []MetricSnapshot) MetricWindow {
	window := metricWindow(samples[0], samples[len(samples)-1])
	window.Samples = samples
	if len(samples) < 3 {
		return window
	}
	window.IntervalJitterNs = intervalJitter(len(samples), func(index int) time.Time { return samples[index].ObservedAt })
	return window
}

func goroutineIntervalJitter(samples []GoroutineReport) *int64 {
	return intervalJitter(len(samples), func(index int) time.Time { return samples[index].ObservedAt })
}

func intervalJitter(sampleCount int, observedAt func(int) time.Time) *int64 {
	if sampleCount < 3 {
		return nil
	}
	minimum, maximum := time.Duration(math.MaxInt64), time.Duration(0)
	for index := 1; index < sampleCount; index++ {
		interval := observedAt(index).Sub(observedAt(index - 1))
		if interval < minimum {
			minimum = interval
		}
		if interval > maximum {
			maximum = interval
		}
	}
	jitter := (maximum - minimum).Nanoseconds()
	return &jitter
}

func trendSuspicions(metrics []MetricSnapshot, goroutines []GoroutineReport) []Suspicion {
	var result []Suspicion
	if metricSamplesComparable(metrics) {
		result = append(result, metricTrendSuspicions(metrics)...)
	}
	result = append(result, goroutineGrowthSuspicions(goroutines)...)
	return result
}

func metricTrendSuspicions(metrics []MetricSnapshot) []Suspicion {
	var result []Suspicion
	if trend := metricTrend(metrics, func(sample MetricSnapshot) (float64, bool) { return numberAtOK(sample.Process, "open_fds") }); trend.sustained(float64(minimumFDGrowth), 0.05) {
		result = append(result, trendSuspicion(trend, len(metrics), trendSuspicionSpec{
			code: FindingPossibleFDLeak, scope: ScopeProcess,
			reason: "Open file descriptors show sustained growth across multiple samples.", metric: "process.open_fds",
			minimumGrowth: float64(minimumFDGrowth), minimumRelativeGrowth: 0.05, unit: "count",
			verify: "Inspect descriptor types and repeat under stable workload before concluding that descriptors leak.",
		}))
	}
	rss := metricTrend(metrics, func(sample MetricSnapshot) (float64, bool) { return numberAtOK(sample.Process, "rss_bytes") })
	runtimeMemory := metricTrend(metrics, func(sample MetricSnapshot) (float64, bool) { return numberAtOK(sample.Runtime, "runtime_memory_bytes") })
	if !rss.sustained(float64(minimumRSSGrowth), 0) || !runtimeMemory.available || runtimeMemory.netGrowth >= rss.netGrowth/2 {
		return result
	}
	suspicion := trendSuspicion(rss, len(metrics), trendSuspicionSpec{
		code: FindingPossibleNativeMemoryLeak, scope: ScopeProcess,
		reason: "RSS shows sustained growth without comparable Go runtime-memory growth.", metric: "process.rss_bytes",
		minimumGrowth: float64(minimumRSSGrowth), unit: "bytes",
		verify: "Inspect cgo, mmap and native libraries, and repeat over a longer representative workload.",
	})
	runtimeRatio := runtimeMemory.netGrowth / rss.netGrowth
	suspicion.Evidence = append(suspicion.Evidence, MetricEvidence{Metric: "runtime.runtime_memory_bytes.net_growth", Value: runtimeMemory.netGrowth})
	suspicion.Criteria = append(suspicion.Criteria, criterion("runtime_to_rss_net_growth_ratio", runtimeRatio, "<", 0.5, "ratio", true))
	return append(result, suspicion)
}

func metricSamplesComparable(samples []MetricSnapshot) bool {
	if len(samples) < minimumTrendSamples || sampleDuration(samples[0], samples[len(samples)-1]) < minimumTrendDuration {
		return false
	}
	for index, sample := range samples {
		if sample.ObservedAt.IsZero() || index > 0 && !sample.ObservedAt.After(samples[index-1].ObservedAt) {
			return false
		}
	}
	return true
}

type trend struct {
	available        bool
	first            float64
	last             float64
	netGrowth        float64
	slopePerSecond   float64
	positiveFraction float64
	observedDuration time.Duration
}

func metricTrend(samples []MetricSnapshot, read func(MetricSnapshot) (float64, bool)) trend {
	if len(samples) < 2 {
		return trend{}
	}
	values := make([]float64, len(samples))
	for index, sample := range samples {
		value, ok := read(sample)
		if !ok || sample.ObservedAt.IsZero() {
			return trend{}
		}
		values[index] = value
	}
	times := make([]float64, len(samples))
	for index := range samples {
		times[index] = samples[index].ObservedAt.Sub(samples[0].ObservedAt).Seconds()
	}
	positive := 0
	for index := 1; index < len(values); index++ {
		if values[index] > values[index-1] {
			positive++
		}
	}
	return trend{
		available: true, first: values[0], last: values[len(values)-1], netGrowth: values[len(values)-1] - values[0],
		slopePerSecond: linearSlope(times, values), positiveFraction: float64(positive) / float64(len(values)-1),
		observedDuration: samples[len(samples)-1].ObservedAt.Sub(samples[0].ObservedAt),
	}
}

func (value trend) sustained(minimumGrowth, minimumRelativeGrowth float64) bool {
	return value.available && value.slopePerSecond > 0 && value.netGrowth >= minimumGrowth &&
		value.last > 0 && value.netGrowth/value.last >= minimumRelativeGrowth && value.positiveFraction >= minimumPositiveIntervalFraction
}

func linearSlope(x, y []float64) float64 {
	var sumX, sumY float64
	for index := range x {
		sumX += x[index]
		sumY += y[index]
	}
	meanX, meanY := sumX/float64(len(x)), sumY/float64(len(y))
	var numerator, denominator float64
	for index := range x {
		numerator += (x[index] - meanX) * (y[index] - meanY)
		denominator += (x[index] - meanX) * (x[index] - meanX)
	}
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func trendSuspicion(value trend, sampleCount int, spec trendSuspicionSpec) Suspicion {
	criteria := []Criterion{
		criterion("sample_count", sampleCount, ">=", minimumTrendSamples, "samples", true),
		criterion("observed_duration", value.observedDuration.Nanoseconds(), ">=", minimumTrendDuration.Nanoseconds(), "nanoseconds", true),
		criterion("positive_slope", value.slopePerSecond, ">", 0.0, spec.unit+"_per_second", true),
		criterion("net_growth", value.netGrowth, ">=", spec.minimumGrowth, spec.unit, true),
	}
	if spec.minimumRelativeGrowth > 0 {
		criteria = append(criteria, criterion("relative_growth", value.netGrowth/value.last, ">=", spec.minimumRelativeGrowth, "ratio", true))
	}
	criteria = append(criteria, criterion("positive_interval_fraction", value.positiveFraction, ">=", minimumPositiveIntervalFraction, "ratio", true))
	return Suspicion{
		Code: spec.code, Severity: "warning", Confidence: ConfidenceSuspected, Scope: spec.scope, Reason: spec.reason,
		Evidence: []MetricEvidence{
			{Metric: spec.metric, Value: value.last, Previous: value.first},
			{Metric: spec.metric + ".slope_per_second", Value: value.slopePerSecond},
			{Metric: spec.metric + ".positive_interval_fraction", Value: value.positiveFraction},
		},
		Criteria: criteria,
		Verify:   spec.verify,
	}
}

func suppressBurstSuspicions(suspicions, trends []Suspicion) []Suspicion {
	suppressed := make(map[string]bool)
	for _, suspicion := range trends {
		switch suspicion.Code {
		case FindingPossibleFDLeak:
			suppressed[FindingFDGrowthBurst] = true
		case FindingPossibleNativeMemoryLeak:
			suppressed[FindingNativeMemoryGrowthBurst] = true
		}
	}
	result := suspicions[:0]
	for _, suspicion := range suspicions {
		if suspicion.Code == FindingLongLivedGoroutineGroup && growingFingerprint(suspicion, trends) {
			continue
		}
		if !suppressed[suspicion.Code] {
			result = append(result, suspicion)
		}
	}
	return result
}

func growingFingerprint(suspicion Suspicion, trends []Suspicion) bool {
	fingerprint, ok := evidenceValue(suspicion, "goroutines.group.fingerprint")
	if !ok {
		return false
	}
	for _, trend := range trends {
		value, present := evidenceValue(trend, "goroutines.fingerprint")
		if trend.Code == FindingPossibleGoroutineLeak && present && fmt.Sprint(value) == fmt.Sprint(fingerprint) {
			return true
		}
	}
	return false
}

func evidenceValue(suspicion Suspicion, metric string) (any, bool) {
	for _, item := range suspicion.Evidence {
		if item.Metric == metric {
			return item.Value, true
		}
	}
	return nil, false
}

func goroutineGrowthSuspicion(samples []GoroutineReport) (Suspicion, bool) {
	suspicions := goroutineGrowthSuspicions(samples)
	if len(suspicions) == 0 {
		return Suspicion{}, false
	}
	return suspicions[0], true
}

func goroutineGrowthSuspicions(samples []GoroutineReport) []Suspicion {
	if len(samples) < minimumTrendSamples || !goroutineSamplesComparable(samples) ||
		samples[len(samples)-1].ObservedAt.Sub(samples[0].ObservedAt) < minimumTrendDuration {
		return nil
	}
	series := make(map[string][]int)
	groups := make(map[string]GoroutineGroup)
	for sampleIndex, sample := range samples {
		for _, group := range sample.Groups {
			values := series[group.Fingerprint]
			if values == nil {
				values = make([]int, len(samples))
			}
			values[sampleIndex] = group.Count
			series[group.Fingerprint], groups[group.Fingerprint] = values, group
		}
	}
	fingerprints := make([]string, 0, len(series))
	for fingerprint := range series {
		fingerprints = append(fingerprints, fingerprint)
	}
	sort.Strings(fingerprints)
	result := make([]Suspicion, 0, len(fingerprints))
	for _, fingerprint := range fingerprints {
		counts := series[fingerprint]
		positive := 0
		for index := 1; index < len(counts); index++ {
			if counts[index] > counts[index-1] {
				positive++
			}
		}
		netGrowth := counts[len(counts)-1] - counts[0]
		group := groups[fingerprint]
		positiveFraction := float64(positive) / float64(len(counts)-1)
		observedDuration := samples[len(samples)-1].ObservedAt.Sub(samples[0].ObservedAt)
		if netGrowth < minimumGoroutineGrowth || positiveFraction < minimumPositiveIntervalFraction || group.BlockedFor < longLivedGoroutineDuration {
			continue
		}
		result = append(result, Suspicion{
			Code: FindingPossibleGoroutineLeak, Severity: "warning", Confidence: ConfidenceSuspected, Scope: ScopeGoroutine,
			Reason: "The same long-lived blocked goroutine fingerprint grew across multiple samples.",
			Evidence: []MetricEvidence{
				{Metric: "goroutines.fingerprint", Value: fingerprint},
				{Metric: "goroutines.fingerprint.count", Value: counts[len(counts)-1], Previous: counts[0]},
				{Metric: "goroutines.fingerprint.counts", Value: counts},
			},
			Criteria: []Criterion{
				criterion("sample_count", len(samples), ">=", minimumTrendSamples, "samples", true),
				criterion("all_samples_complete", true, "=", true, "boolean", true),
				criterion("observed_duration", observedDuration.Nanoseconds(), ">=", minimumTrendDuration.Nanoseconds(), "nanoseconds", true),
				criterion("fingerprint_net_growth", netGrowth, ">=", minimumGoroutineGrowth, "goroutines", true),
				criterion("positive_interval_fraction", positiveFraction, ">=", minimumPositiveIntervalFraction, "ratio", true),
				criterion("blocked_duration", group.BlockedFor.Nanoseconds(), ">=", longLivedGoroutineDuration.Nanoseconds(), "nanoseconds", true),
			},
			Verify: "Continue sampling under stable load and verify ownership, cancellation and producer paths for this stack.",
		})
	}
	return result
}

func goroutineSamplesComparable(samples []GoroutineReport) bool {
	for index, sample := range samples {
		if sample.ObservedAt.IsZero() || sample.Completeness != GoroutineCompletenessComplete {
			return false
		}
		if index > 0 && !sample.ObservedAt.After(samples[index-1].ObservedAt) {
			return false
		}
	}
	return true
}

func structuralChanges(samples []MetricSnapshot) []StructuralChange {
	if len(samples) < 2 {
		return nil
	}
	var changes []StructuralChange
	for index := 1; index < len(samples); index++ {
		before, after := samples[index-1], samples[index]
		changes = appendChange(changes, after.ObservedAt, "runtime.gomaxprocs", ScopeRuntime, before.Runtime, after.Runtime, "gomaxprocs")
		changes = appendChange(changes, after.ObservedAt, "cgroup.memory_max_bytes", ScopeCgroup, before.Cgroup, after.Cgroup, "memory_max_bytes")
		changes = appendChange(changes, after.ObservedAt, "cgroup.cpu_quota_cores", ScopeCgroup, before.Cgroup, after.Cgroup, "cpu_quota_cores")
		changes = appendChange(changes, after.ObservedAt, "cgroup.pids_max", ScopeCgroup, before.Cgroup, after.Cgroup, "pids_max")
		beforePools := poolsByName(before.DBPools)
		for _, pool := range after.DBPools {
			previous, ok := beforePools[pool.Name]
			if ok && previous.MaxOpenConnections != pool.MaxOpenConnections {
				changes = append(changes, StructuralChange{
					ObservedAt: after.ObservedAt, Metric: fmt.Sprintf("db_pools.%s.max_open_connections", pool.Name), Scope: ScopeDatabasePool,
					Previous: previous.MaxOpenConnections, Value: pool.MaxOpenConnections,
				})
			}
		}
	}
	return changes
}

func appendChange(changes []StructuralChange, observedAt time.Time, metric, scope string, before, after map[string]any, path ...string) []StructuralChange {
	previous, previousOK := valueAt(before, path...)
	value, valueOK := valueAt(after, path...)
	if !previousOK || !valueOK || fmt.Sprint(previous) == fmt.Sprint(value) {
		return changes
	}
	return append(changes, StructuralChange{ObservedAt: observedAt, Metric: metric, Scope: scope, Previous: previous, Value: value})
}

func clearAllGoroutineFindings(report *DiagnosisReport) {
	clearGoroutineFindings(report)
	report.Goroutines.Alerts = nil
}

func compactGoroutineSamples(samples []GoroutineReport) []GoroutineSample {
	result := make([]GoroutineSample, 0, len(samples))
	for _, sample := range samples {
		groups := make([]GoroutineSampleGroup, 0, len(sample.Groups))
		for _, group := range sample.Groups {
			groups = append(groups, GoroutineSampleGroup{
				Fingerprint: group.Fingerprint, Count: group.Count, State: group.State, BlockedFor: group.BlockedFor,
			})
		}
		result = append(result, GoroutineSample{
			ObservedAt: sample.ObservedAt, Total: sample.Total, BlocksParsed: sample.BlocksParsed,
			BlocksDiscarded: sample.BlocksDiscarded, Completeness: sample.Completeness, Groups: groups,
		})
	}
	return result
}
