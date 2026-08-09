---
name: gaz-mcp-diagnostics
description: Instrument and diagnose Go processes with the gaz-mcp diagnostics module and MCP tools, including pprof profiles, flame graphs, grouped goroutine stacks, expvar metrics, database/sql pool statistics, and evidence-backed heuristic suspicions. Use when adding diagnostics endpoints to a Go service or investigating CPU, memory, allocation, contention, goroutine, file-descriptor, scheduler, cgroup, host-resource, or SQL-pool problems.
---

# gaz-mcp Go diagnostics

Instrument a Go service with the standalone diagnostics module, then inspect it through its absolute `target` URL. Profiles require `/debug/pprof`; metrics require `/debug/vars`.

## Instrument the target service

Install the independent Go module in the service repository:

```bash
go get github.com/jcastilloa/gaz-mcp/diagnostics@latest
```

The module requires Go 1.26 or newer. Confirm the service toolchain before adding it.

Register it on the service's dedicated `http.ServeMux`. Runtime, process, host, and cgroup facts are published automatically; do not recreate them as custom expvars.

```go
import (
	"database/sql"
	"expvar"
	"net/http"

	"github.com/jcastilloa/gaz-mcp/diagnostics"
)

func registerDiagnostics(mux *http.ServeMux, db *sql.DB, queueDepth func() int) error {
	return diagnostics.Register(mux,
		diagnostics.WithDBPool("primary", db),
		diagnostics.WithCustomVars(func(vars *expvar.Map) {
			vars.Set("queue_depth", expvar.Func(func() any { return queueDepth() }))
		}),
	)
}
```

Adapt the options to the service:

- Omit `WithDBPool` when there is no `*sql.DB`; register each relevant pool under a unique stable name.
- Use `WithCustomVars` only for cheap, read-only, application-specific values. They appear below `gaz_diagnostics.custom`.
- Enable block and mutex sampling only when needed with `WithBlockProfileRate(time.Millisecond)` and `WithMutexProfileFraction(5)`. Both options change process-wide runtime settings and add overhead.
- Add `WithMiddleware` and/or `WithIPAllowlist` when the endpoints are not already protected by a trusted internal proxy or router.
- Use `Handler(options...)` instead of `Register` when the framework mounts an `http.Handler`; expose both `/debug/pprof` (including its subpaths) and `/debug/vars`.
- Propagate registration errors during startup, or use `MustRegister`/`MustHandler` when invalid diagnostics configuration must stop the process.
- Never serve `http.DefaultServeMux`; the module intentionally requires an explicit protected mux or handler.

By default, tell gaz-mcp to use the service origin as `target`, for example `https://orders.internal`. With `WithBasePath("/internal/diagnostics")`, use `target="https://orders.internal/internal/diagnostics"`; gaz-mcp appends `/debug/...` itself.

Verify the integration before diagnosing:

```bash
curl --fail --show-error https://orders.internal/debug/vars
curl --fail --show-error 'https://orders.internal/debug/pprof/goroutine?debug=2'
```

Expect JSON containing `gaz_diagnostics` from the first request and a goroutine dump from the second. An HTTP 200 containing application HTML usually means a reverse proxy or SPA fallback is swallowing `/debug/*`; fix routing or use the backend service origin.

Keep the boundary clear: the service-side module publishes raw observations. It does not apply thresholds or emit health conclusions; gaz-mcp creates separate, reviewable suspicions from the observations.

## Choose the first tool

- Start broad with `diagnose(target)` when the cause is unknown. It returns raw observations and separate heuristic suspicions.
- Use `diagnose(target, samples=3..10, interval_seconds=1..30)` only when sustained FD, RSS, or goroutine growth matters. Both parameters are whole numbers. It repeats goroutine dumps, is capped at 60 seconds total configured duration, and costs more on the target than the default diagnosis.
- Use `analyze_goroutines(target)` for blocking, leaks, connection waiters, or lock contention.
- Use `get_runtime_stats(target)` for a compact standard-runtime view.
- Use `get_db_pool_stats(target)` for instrumented `database/sql` pools.
- Use `get_expvars(target)` only when complete raw expvar data is necessary; it may be large or sensitive.
- Use profiling tools when a symptom needs code-level attribution.

## Interpret diagnose correctly

`diagnose` deliberately separates:

- `metrics.before` and `metrics.after`: raw runtime, process, host, cgroup, SQL-pool, and custom values.
- `goroutines`: the latest grouped stack observations and parse completeness. Multi-sample mode adds compact earlier fingerprint/count samples, source-specific observation timestamps, and separate interval jitter for metric and goroutine series.
- `suspicions`: heuristic candidates with severity, confidence, scope, evidence, evaluated criteria, reason, and a verification hint.

Treat metrics and stacks as observations. Treat every suspicion, including one marked `confirmed`, as a hypothesis supported by multiple signals rather than an established root cause. Check its evidence, consider workload and container context, and use the `verify` hint or a focused profile before concluding.

Counter-based heuristics depend on deltas between the two samples. Do not infer current pressure from an accumulated total alone. Distinguish host metrics from cgroup limits: inside a container, host capacity may not describe resources available to the process.

## Profile workflow

1. Call `capture_profile(target, type, seconds?, gc?)` and retain its `profile_id`.
2. Call `analyze_profile(profile_id, top_n?)` for raw values, percentages, and available rates.
3. Call `generate_flamegraph(profile_id)` when stack shape matters.
4. Call `diff_profiles(id_a, id_b, top_n?)` only for comparable profiles with the same sample type and unit.

Choose the profile type by symptom:

| Type | Use for |
|---|---|
| `cpu` | Sustained CPU or slow CPU-bound requests |
| `heap` | Retained Go heap memory |
| `allocs` | Allocation churn |
| `goroutine` | Goroutine growth or blocking |
| `block` | Time blocked on synchronization or channels |
| `mutex` | Lock contention |
| `threadcreate` | Unexpected operating-system thread creation |

CPU captures accept a duration from 1 to 60 seconds and default to 10. For heap, allocs, goroutine, block, mutex, and threadcreate, `seconds` requests an interval delta profile. `gc=true` is valid only for a heap snapshot without `seconds`. Use a short representative interval in production.

`analyze_profile` and `diff_profiles` expose `rates_available`. When true, use per-second values to compare different capture durations; retain raw deltas to understand total change. When false, compare only like-for-like snapshots and workloads.

Block and mutex profiles can be empty unless runtime sampling was enabled in the target. Absence of samples is not evidence that contention is absent.

## Verification patterns

- High RSS with stable Go memory: inspect cgroup headroom and validate possible native, cgo, or mmap growth rather than blaming the Go heap.
- SQL pool suspicion: correlate waiter stacks with named-pool saturation and new wait-count activity. Wait duration only records completed waits; a duration-only delta without current saturation is warning evidence. Saturation plus waiter stacks stays critical evidence if counters are unchanged, but remains suspected. With several saturated active pools, stacks cannot identify the responsible `*sql.DB` instance.
- `FD_GROWTH_BURST`, `NATIVE_MEMORY_GROWTH_BURST`, and `LONG_LIVED_GOROUTINE_GROUP` describe one short window or capture; do not call them leaks.
- A possible leak requires at least three samples over ten observed seconds, with sustained growth of FDs, RSS, or the same complete goroutine fingerprint. Metric and goroutine trends remain independent when one endpoint fails. Every qualifying growing fingerprint is reported; stable long-lived groups remain separate suspicions. Earlier goroutine samples are compact fingerprint/count series; the latest report retains full stacks. One large stable worker group may be intentional.
- File-descriptor pressure: verify both open-FD ratio and trend; the instrumentation exposes counts, not paths.
- Scheduler or throttling pressure: correlate runnable goroutines, GOMAXPROCS, cgroup throttling deltas, and a CPU profile.

## Operational rules

- Diagnostic endpoints can expose internals. Use only authorised targets and do not reproduce sensitive expvars unnecessarily.
- HTTPS targets with a valid self-signed certificate or a certificate issued by an unknown private CA are accepted by default. Treat the informational TLS alert as transport context, not a process-health suspicion; hostname, validity, and server-auth usage are still checked.
- Profile IDs reference files local to the gaz-mcp process and are temporary, not portable artifacts.
- Profile capture is active on the target; interval profiles sample the runtime and `gc=true` forces garbage collection. Avoid repeated or unnecessarily long captures.
- Report the raw evidence, the heuristic interpretation, and remaining uncertainty separately.
