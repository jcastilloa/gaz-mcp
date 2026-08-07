# Go process diagnostics

`gaz-mcp` can inspect a running Go process through the standard `pprof` and `expvar` HTTP endpoints. It captures profiles, groups goroutine stacks, reads runtime metrics, and correlates optional `database/sql` pool statistics.

The diagnostic destination is supplied with every MCP call as an absolute `target` URL, for example `http://orders.internal:6060`. There is deliberately no target list in `config.yaml`: the caller names the authorised process to inspect.

For installation and shared configuration, see the [root README](../README.md).

## Target instrumentation

The target must expose `/debug/pprof/` and, for runtime and custom metrics, `/debug/vars`. A target without SQL is fully supported; pool metrics are an optional extension, not a requirement.

### Baseline: pprof and expvar

For a service with its own `http.ServeMux`, register the handlers before serving requests:

```go
package diagnostics

import (
	"expvar"
	"net/http"
	"net/http/pprof"
	"runtime"
	"sync"
)

var publishRuntimeMetrics sync.Once

func RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/vars", expvar.Handler())

	publishRuntimeMetrics.Do(func() {
		if expvar.Get("num_goroutine") == nil {
			expvar.Publish("num_goroutine", expvar.Func(func() any {
				return runtime.NumGoroutine()
			}))
		}
	})
}
```

Importing `expvar` publishes the standard `cmdline` and `memstats` variables. The additional `num_goroutine` variable above lets `get_runtime_stats` report the current goroutine count without replacing an existing service metric. `num_threads` is also read when a service publishes it, but Go has no general `runtime` accessor for it, so do not publish a made-up value.

If the application uses `http.DefaultServeMux`, importing `_ "net/http/pprof"` and `expvar` is the usual alternative. Do not register the same paths twice.

### Optional: database/sql pool metrics

`pprof` does not know the state of `database/sql` pools. A service that uses one or more `*sql.DB` instances can publish their `DBStats` under the optional `gaz_diagnostics.db_pools` contract:

```go
package diagnostics

import (
	"database/sql"
	"expvar"
	"sync"
)

var (
	dbPools          = new(expvar.Map)
	publishDBPools   sync.Once
)

func RegisterDBPool(name string, db *sql.DB) {
	publishDBPools.Do(func() {
		diagnostics := expvar.NewMap("gaz_diagnostics")
		diagnostics.Set("db_pools", dbPools)
	})

	dbPools.Set(name, expvar.Func(func() any {
		stats := db.Stats()
		return map[string]any{
			"open_connections":     stats.OpenConnections,
			"in_use":               stats.InUse,
			"idle":                 stats.Idle,
			"max_open_connections": stats.MaxOpenConnections,
			"wait_count":           stats.WaitCount,
			"wait_duration_ns":     stats.WaitDuration.Nanoseconds(),
			"max_idle_closed":      stats.MaxIdleClosed,
			"max_idle_time_closed": stats.MaxIdleTimeClosed,
			"max_lifetime_closed":  stats.MaxLifetimeClosed,
		}
	}))
}
```

Register every pool whose behaviour matters:

```go
diagnostics.RegisterDBPool("primary", primaryDB)
diagnostics.RegisterDBPool("analytics", analyticsDB)
```

The `sync.Once` prevents duplicate publication when multiple pools are registered. This optional snippet assumes that the service does not already publish an unrelated `gaz_diagnostics` expvar. A service with no SQL should omit this instrumentation entirely; it remains diagnosable through profiles, goroutines, standard runtime values, and any service-specific expvars it chooses to publish.

The MCP recognises the field names shown above. Additional fields and arbitrary service variables are allowed and remain available through `get_expvars`, even though they are not interpreted as pool statistics.

### Restrict endpoint access

`pprof` stacks, profiles, and expvars can reveal internal code paths, command-line arguments, host details, and operational state. Keep these endpoints on a private network, behind an authenticated proxy, or protected by firewall rules. The diagnostic tools fetch the target over HTTP(S), so use them only against processes you are authorised to inspect.

## Tool reference

All diagnostic tools are read-only from the target's perspective. Tools that accept `target` require an absolute `http://` or `https://` base URL; a base path is supported and the appropriate `/debug/...` suffix is appended.

| Tool | Parameters | Description |
|---|---|---|
| `capture_profile` | `target`, `type`, `seconds?` | Capture and persist `cpu`, `heap`, `allocs`, `goroutine`, `block`, `mutex`, or `threadcreate`, returning a `profile_id`. CPU uses `/debug/pprof/profile?seconds=N`; it accepts 1–60 seconds and defaults to 10. `seconds` is ignored for other profile types. |
| `analyze_profile` | `profile_id`, `top_n?` | Return functions ordered by cumulative cost, with flat and cumulative values. `top_n` defaults to 20 and is limited to 1–100. |
| `diff_profiles` | `id_a`, `id_b`, `top_n?` | Compare two stored profiles with the same primary sample type and unit. Positive values grew from A to B. |
| `generate_flamegraph` | `profile_id` | Generate an SVG flame graph from a stored profile. |
| `analyze_goroutines` | `target` | Fetch `goroutine?debug=2`, group equivalent stacks, retain state and blocked duration, and label known signatures. |
| `get_runtime_stats` | `target` | Read `num_goroutine`, optional `num_threads`, and expvar `memstats`/GC values. |
| `get_db_pool_stats` | `target` | Read the optional `gaz_diagnostics.db_pools` contract. A target without it yields no pool data. |
| `get_expvars` | `target` | Return the complete JSON object from `/debug/vars`, including service-specific variables. |
| `diagnose` | `target` | Combine goroutine groups, runtime data, and optional pool metrics into one diagnosis report. |

Captured profiles are stored locally under the operating system temporary directory (`<temp>/gaz-mcp/pprof`) with restrictive permissions. A `profile_id` is therefore usable by the same `gaz-mcp` installation while those files remain present; it is not a portable or permanent profile reference.

The profile binary stays in local temporary storage. `capture_profile` returns metadata and all analysis tools return structured summaries rather than raw pprof bytes.

### Choosing a profile

| Type | Use it for |
|---|---|
| `cpu` | Slow requests or sustained CPU use. |
| `heap` | Retained memory and GC pressure. |
| `allocs` | Allocation churn. |
| `goroutine` | Leaks and blocking. |
| `block` | Time spent in blocking operations. |
| `mutex` | Lock contention. |
| `threadcreate` | Unexpected operating-system thread creation. |

## Workflows

### Diagnose a process directly

```text
diagnose(target="http://orders.internal:6060")
```

This first reads goroutines. If `/debug/vars` is available, it adds runtime and optional pool metrics. If `/debug/vars` cannot be read, `diagnose` still returns the goroutine analysis and marks runtime instrumentation as unavailable. The individual runtime and expvar tools require that endpoint and return an error if it is unavailable.

### Capture and inspect CPU work

```text
capture_profile(target="http://orders.internal:6060", type="cpu", seconds=30)
analyze_profile(profile_id="<id returned above>", top_n=20)
generate_flamegraph(profile_id="<id returned above>")
```

Use a CPU duration that covers the behaviour under investigation. The HTTP client has a 70-second timeout, so the 60-second capture limit leaves room for request overhead.

### Compare two memory or allocation snapshots

```text
capture_profile(target="http://orders.internal:6060", type="heap")
# Reproduce the workload, then capture a second heap profile.
capture_profile(target="http://orders.internal:6060", type="heap")
diff_profiles(id_a="<first id>", id_b="<second id>", top_n=20)
```

Compare like with like: `diff_profiles` rejects profiles whose primary sample type or unit differs. Heap profiles help with retained memory; `allocs` focuses on allocation churn.

### Confirm a connection-pool problem

```text
analyze_goroutines(target="http://orders.internal:6060")
get_db_pool_stats(target="http://orders.internal:6060")
diagnose(target="http://orders.internal:6060")
```

Without the optional pool instrumentation, a `database/sql` waiter remains useful evidence but the final conclusion is only `suspected`. With matching saturated pool metrics, it is `confirmed`.

## Findings and diagnosis limits

`analyze_goroutines` labels each matching goroutine group with the following signatures:

| Finding | Conditions | Meaning |
|---|---|---|
| `DB_POOL_EXHAUSTED` | Stack contains `database/sql.(*DB).conn` and state is `semacquire`, `chan receive`, or `select` | Goroutines wait for a `database/sql` connection. |
| `HTTP_INFLIGHT_FASTHTTP` | Stack contains `fasthttp.(*Server).serveConn` | In-flight `fasthttp` requests share the stack. |
| `HTTP_INFLIGHT_NET_HTTP` | Stack contains `net/http.(*conn).serve` | In-flight standard-library HTTP requests share the stack. |
| `LOCK_CONTENTION` | More than one goroutine and a mutex-lock stack frame | Several goroutines contend on a mutex. |
| `POSSIBLE_GOROUTINE_LEAK` | At least 10 goroutines blocked on channel send/receive for at least five minutes | A large, long-lived blocked group may be leaking. |

`diagnose` turns only the database waiter finding into a top-level conclusion. It reports `DB_POOL_EXHAUSTED` as **confirmed** only when at least one exposed pool has all of these conditions:

- `max_open_connections > 0`
- `in_use >= max_open_connections`
- `idle == 0`
- `wait_count > 0`

Otherwise it reports a **suspected** pool exhaustion and recommends publishing `DBStats`. The other signature labels are evidence to investigate, not automatic root-cause conclusions.

`get_runtime_stats` reads `HeapAlloc`, `HeapSys`, `NumGC`, `PauseTotalNs`, and `LastGC` from standard `memstats`. It includes `goroutine_source` when `num_goroutine` is published. During `diagnose`, an absent `num_goroutine` is filled from the goroutine dump and marked as `goroutine_dump`; `get_runtime_stats` alone leaves it at zero. Missing optional `num_threads` appears as zero; use `get_expvars` when a distinction between zero and an unpublished variable matters.

## Operational notes

- Profile and endpoint responses are capped at 128 MiB. Very large profiles or expvar responses are rejected rather than kept in memory unboundedly.
- `generate_flamegraph` renders an SVG flame graph locally from the captured profile; it does not require Graphviz or the target binary.
- CPU profiles are an active capture on the target for their chosen duration; prefer a short, representative interval and avoid indiscriminate repeated captures on a busy production process.
- `get_expvars` returns the full endpoint payload. Treat it as potentially sensitive and avoid forwarding large raw outputs where a focused analysis is enough.
- For SQL and Jenkins guidance, return to the [documentation map](../README.md#documentation-map).
