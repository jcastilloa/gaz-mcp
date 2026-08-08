package diagnostics

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

func metricSnapshotFromExpvars(vars map[string]any) MetricSnapshot {
	return metricSnapshotFromExpvarsAt(vars, time.Now().UTC())
}

func metricSnapshotFromExpvarsAt(vars map[string]any, observedAt time.Time) MetricSnapshot {
	snapshot := emptyMetricSnapshot()
	snapshot.ObservedAt = observedAt
	diagnostics, ok := object(vars["gaz_diagnostics"])
	if ok {
		snapshot.CollectedAt = timestamp(diagnostics["collected_at"])
		snapshot.Runtime = objectOrEmpty(diagnostics["runtime"])
		snapshot.Process = objectOrEmpty(diagnostics["process"])
		snapshot.Host = objectOrEmpty(diagnostics["host"])
		snapshot.Cgroup = objectOrEmpty(diagnostics["cgroup"])
		snapshot.Custom = objectOrEmpty(diagnostics["custom"])
		snapshot.DBPools = dbPoolStatsFromExpvars(vars)
	}
	addStandardRuntimeMetrics(snapshot.Runtime, vars)
	return snapshot
}

func emptyMetricSnapshot() MetricSnapshot {
	return MetricSnapshot{
		Runtime: map[string]any{}, Process: map[string]any{}, Host: map[string]any{}, Cgroup: map[string]any{},
		DBPools: []DBPoolStats{}, Custom: map[string]any{},
	}
}

func objectOrEmpty(value any) map[string]any {
	result, ok := object(value)
	if !ok {
		return map[string]any{}
	}
	return result
}

func timestamp(value any) time.Time {
	text, ok := value.(string)
	if !ok {
		return time.Time{}
	}
	parsed, _ := time.Parse(time.RFC3339Nano, text)
	return parsed
}

func addStandardRuntimeMetrics(runtimeMetrics, vars map[string]any) {
	if _, ok := runtimeMetrics["goroutines"]; !ok {
		if value, published := vars["num_goroutine"]; published {
			runtimeMetrics["goroutines"] = map[string]any{"total": value}
		}
	}
	if _, ok := runtimeMetrics["threads"]; !ok {
		if value, published := vars["num_threads"]; published {
			runtimeMetrics["threads"] = value
		}
	}
	if memstats, ok := object(vars["memstats"]); ok {
		runtimeMetrics["memstats"] = memstats
	}
}

func metricWindow(before, after MetricSnapshot) MetricWindow {
	duration := sampleDuration(before, after)
	if duration < 0 {
		duration = 0
	}
	return MetricWindow{Before: before, After: after, DurationNs: duration.Nanoseconds()}
}

func sampleDuration(before, after MetricSnapshot) time.Duration {
	if !before.ObservedAt.IsZero() && !after.ObservedAt.IsZero() {
		return after.ObservedAt.Sub(before.ObservedAt)
	}
	return after.CollectedAt.Sub(before.CollectedAt)
}

func valueAt(values map[string]any, path ...string) (any, bool) {
	var current any = values
	for _, name := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[name]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func numberAt(values map[string]any, path ...string) float64 {
	number, _ := numberAtOK(values, path...)
	return number
}

func numberAtOK(values map[string]any, path ...string) (float64, bool) {
	value, ok := valueAt(values, path...)
	if !ok {
		return 0, false
	}
	return numericValue(value)
}

func numericValue(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		number, err := strconv.ParseFloat(value.String(), 64)
		return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
	case float64:
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	case float32:
		number := float64(value)
		return number, !math.IsNaN(number) && !math.IsInf(number, 0)
	case uint64:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint:
		return float64(value), true
	case int64:
		return float64(value), true
	case int:
		return float64(value), true
	}
	return 0, false
}

func deltaAt(before, after map[string]any, path ...string) float64 {
	beforeValue, beforeOK := valueAt(before, path...)
	afterValue, afterOK := valueAt(after, path...)
	if !beforeOK || !afterOK {
		return 0
	}
	if beforeNumber, beforeExact := unsignedInteger(beforeValue); beforeExact {
		if afterNumber, afterExact := unsignedInteger(afterValue); afterExact {
			if afterNumber < beforeNumber {
				return 0
			}
			return float64(afterNumber - beforeNumber)
		}
	}
	beforeNumber, beforeValid := numericValue(beforeValue)
	afterNumber, afterValid := numericValue(afterValue)
	if !beforeValid || !afterValid {
		return 0
	}
	delta := afterNumber - beforeNumber
	if delta < 0 {
		return 0
	}
	return delta
}

func unsignedInteger(value any) (uint64, bool) {
	switch value := value.(type) {
	case json.Number:
		if strings.ContainsAny(value.String(), ".eE-") {
			return 0, false
		}
		number, err := strconv.ParseUint(value.String(), 10, 64)
		return number, err == nil
	case uint64:
		return value, true
	case uint32:
		return uint64(value), true
	case uint:
		return uint64(value), true
	case int64:
		return uint64(value), value >= 0
	case int:
		return uint64(value), value >= 0
	default:
		return 0, false
	}
}
