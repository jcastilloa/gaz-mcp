package diagnostics

import (
	"math"
	"runtime"
	runtimemetrics "runtime/metrics"
)

var runtimeMetricNames = []string{
	"/sched/gomaxprocs:threads",
	"/sched/goroutines:goroutines",
	"/sched/goroutines/runnable:goroutines",
	"/sched/goroutines/running:goroutines",
	"/sched/goroutines/waiting:goroutines",
	"/sched/goroutines/not-in-go:goroutines",
	"/sched/threads/total:threads",
	"/gc/heap/live:bytes",
	"/gc/heap/goal:bytes",
	"/gc/gomemlimit:bytes",
	"/memory/classes/total:bytes",
	"/memory/classes/heap/released:bytes",
	"/memory/classes/heap/stacks:bytes",
	"/memory/classes/os-stacks:bytes",
	"/gc/heap/allocs:bytes",
	"/cpu/classes/gc/total:cpu-seconds",
	"/cpu/classes/user:cpu-seconds",
	"/sync/mutex/wait/total:seconds",
	"/sched/latencies:seconds",
	"/sched/pauses/total/gc:seconds",
}

func collectRuntimeMetrics() map[string]any {
	values := readRuntimeMetrics()
	return map[string]any{
		"go_version":                 runtime.Version(),
		"gomaxprocs":                 metricUint64(values, "/sched/gomaxprocs:threads"),
		"goroutines":                 goroutineMetrics(values),
		"threads":                    metricUint64(values, "/sched/threads/total:threads"),
		"heap_live_bytes":            metricUint64(values, "/gc/heap/live:bytes"),
		"heap_goal_bytes":            metricUint64(values, "/gc/heap/goal:bytes"),
		"memory_limit_bytes":         metricUint64(values, "/gc/gomemlimit:bytes"),
		"runtime_memory_bytes":       metricUint64(values, "/memory/classes/total:bytes"),
		"heap_released_bytes":        metricUint64(values, "/memory/classes/heap/released:bytes"),
		"stack_bytes":                stackBytes(values),
		"heap_allocated_bytes_total": metricUint64(values, "/gc/heap/allocs:bytes"),
		"gc_cpu_seconds":             metricFloat64(values, "/cpu/classes/gc/total:cpu-seconds"),
		"user_cpu_seconds":           metricFloat64(values, "/cpu/classes/user:cpu-seconds"),
		"mutex_wait_seconds":         metricFloat64(values, "/sync/mutex/wait/total:seconds"),
		"scheduler_latency_seconds":  metricQuantiles(values, "/sched/latencies:seconds"),
		"gc_pause_seconds":           metricQuantiles(values, "/sched/pauses/total/gc:seconds"),
	}
}

func readRuntimeMetrics() map[string]runtimemetrics.Value {
	samples := make([]runtimemetrics.Sample, len(runtimeMetricNames))
	for index, name := range runtimeMetricNames {
		samples[index].Name = name
	}
	runtimemetrics.Read(samples)
	values := make(map[string]runtimemetrics.Value, len(samples))
	for _, sample := range samples {
		values[sample.Name] = sample.Value
	}
	return values
}

func goroutineMetrics(values map[string]runtimemetrics.Value) map[string]uint64 {
	return map[string]uint64{
		"total":     metricUint64(values, "/sched/goroutines:goroutines"),
		"runnable":  metricUint64(values, "/sched/goroutines/runnable:goroutines"),
		"running":   metricUint64(values, "/sched/goroutines/running:goroutines"),
		"waiting":   metricUint64(values, "/sched/goroutines/waiting:goroutines"),
		"not_in_go": metricUint64(values, "/sched/goroutines/not-in-go:goroutines"),
	}
}

func stackBytes(values map[string]runtimemetrics.Value) uint64 {
	return metricUint64(values, "/memory/classes/heap/stacks:bytes") +
		metricUint64(values, "/memory/classes/os-stacks:bytes")
}

func metricUint64(values map[string]runtimemetrics.Value, name string) uint64 {
	value := values[name]
	if value.Kind() != runtimemetrics.KindUint64 {
		return 0
	}
	return value.Uint64()
}

func metricFloat64(values map[string]runtimemetrics.Value, name string) float64 {
	value := values[name]
	if value.Kind() != runtimemetrics.KindFloat64 {
		return 0
	}
	return value.Float64()
}

func metricQuantiles(values map[string]runtimemetrics.Value, name string) map[string]float64 {
	result := map[string]float64{"p50": 0, "p95": 0, "p99": 0}
	value := values[name]
	if value.Kind() != runtimemetrics.KindFloat64Histogram {
		return result
	}
	histogram := value.Float64Histogram()
	result["p50"] = histogramQuantile(histogram, 0.50)
	result["p95"] = histogramQuantile(histogram, 0.95)
	result["p99"] = histogramQuantile(histogram, 0.99)
	return result
}

func histogramQuantile(histogram *runtimemetrics.Float64Histogram, quantile float64) float64 {
	var total uint64
	for _, count := range histogram.Counts {
		total += count
	}
	if total == 0 {
		return 0
	}
	target := uint64(math.Ceil(float64(total) * quantile))
	var seen uint64
	for index, count := range histogram.Counts {
		seen += count
		if seen < target {
			continue
		}
		upper := histogram.Buckets[index+1]
		if math.IsInf(upper, 1) {
			return histogram.Buckets[index]
		}
		return upper
	}
	return 0
}
