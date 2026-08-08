package diagnostics

import (
	"encoding/json"
	"net/http"
	"runtime"
	"testing"
)

func TestRuntimeMetricsExposeDiagnosticSignals(t *testing.T) {
	stats := collectRuntimeMetrics()

	for _, name := range []string{
		"go_version", "gomaxprocs", "goroutines", "threads",
		"heap_live_bytes", "heap_goal_bytes", "memory_limit_bytes",
		"runtime_memory_bytes", "heap_released_bytes", "stack_bytes",
		"heap_allocated_bytes_total", "gc_cpu_seconds", "user_cpu_seconds",
		"mutex_wait_seconds", "scheduler_latency_seconds",
	} {
		if _, ok := stats[name]; !ok {
			t.Errorf("runtime metric %q is missing", name)
		}
	}

	goroutines := stats["goroutines"].(map[string]uint64)
	for _, state := range []string{"total", "runnable", "running", "waiting", "not_in_go"} {
		if _, ok := goroutines[state]; !ok {
			t.Errorf("goroutine state %q is missing", state)
		}
	}
	if stats["go_version"] != runtime.Version() {
		t.Fatalf("go version = %v, want %s", stats["go_version"], runtime.Version())
	}

	latency := stats["scheduler_latency_seconds"].(map[string]float64)
	for _, quantile := range []string{"p50", "p95", "p99"} {
		if _, ok := latency[quantile]; !ok {
			t.Errorf("scheduler latency %q is missing", quantile)
		}
	}
}

func TestExpvarsExposeRawDiagnosticGroups(t *testing.T) {
	mux := newTestMux(t)
	vars := varsResponse(t, request(mux, "/debug/vars", "127.0.0.1:1000", "", ""))
	diagnostics := object(t, vars["gaz_diagnostics"])

	if _, ok := diagnostics["collected_at"]; !ok {
		t.Fatal("gaz_diagnostics.collected_at is missing")
	}
	for _, name := range []string{"runtime", "process", "host", "cgroup", "db_pools", "custom"} {
		if _, ok := diagnostics[name]; !ok {
			t.Errorf("gaz_diagnostics.%s is missing", name)
		}
	}

	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatalf("encode diagnostics: %v", err)
	}
	if !json.Valid(encoded) {
		t.Fatal("gaz_diagnostics is not valid JSON")
	}
}

func newTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	if err := Register(mux); err != nil {
		t.Fatalf("register: %v", err)
	}
	return mux
}
