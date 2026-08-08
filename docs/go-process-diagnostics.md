# Go process diagnostics

`gaz-mcp` can inspect a running Go process through the standard `pprof` and `expvar` HTTP endpoints. It captures profiles, groups goroutine stacks, and keeps raw runtime, process, cgroup, and optional `database/sql` pool metrics separate from heuristic suspicions.

The diagnostic destination is supplied with every MCP call as an absolute `target` URL, for example `http://orders.internal:6060`. There is deliberately no target list in `config.yaml`: the caller names the authorised process to inspect.

For installation and shared configuration, see the [root README](../README.md).
Agents investigating Go processes can install the self-contained [`gaz-mcp-diagnostics` skill](../skills/gaz-mcp-diagnostics/SKILL.md).

## Target instrumentation

The target must expose `/debug/pprof/` and, for runtime and custom metrics, `/debug/vars`. A target without SQL is fully supported; pool metrics are an optional extension, not a requirement.

### Reusable instrumentation module

This repository includes an independent Go module at
`github.com/jcastilloa/gaz-mcp/diagnostics`. It contains no MCP or Jenkins
dependencies, so a service can import only the instrumentation it needs:

```bash
go get github.com/jcastilloa/gaz-mcp/diagnostics@v0.1.0
```

Use a dedicated mux and register everything with one call. The default paths
are `/debug/pprof/` and `/debug/vars`:

```go
import (
	"database/sql"
	"net/http"
	"time"

	"github.com/jcastilloa/gaz-mcp/diagnostics"
)

func serve(db *sql.DB) error {
	mux := http.NewServeMux()
	if err := diagnostics.Register(mux,
		diagnostics.WithDBPool("primary", db),
		diagnostics.WithBlockProfileRate(time.Millisecond),
		diagnostics.WithMutexProfileFraction(5),
		diagnostics.WithIPAllowlist("10.0.0.0/8", "192.168.0.0/16"),
		diagnostics.WithMiddleware(requireInternalOperator),
	); err != nil {
		return err
	}
	return http.ListenAndServe(":6060", mux)
}
```

`diagnostics.MustRegister(mux, ...)` is available when startup configuration
errors should stop the process immediately.

SQL is optional; a service without a database can simply call
`diagnostics.Register(mux)`. To mount below a custom prefix, pass an absolute
path to `WithBasePath`:

```go
diagnostics.Register(mux, diagnostics.WithBasePath("/internal/diagnostics"))
```

That single option changes the endpoint mapping as follows:

| Default route | WithBasePath(`/internal/diagnostics`) |
|---|---|
| `/debug/pprof/` | `/internal/diagnostics/debug/pprof/` |
| `/debug/pprof/goroutine?debug=2` | `/internal/diagnostics/debug/pprof/goroutine?debug=2` |
| `/debug/pprof/profile?seconds=10` | `/internal/diagnostics/debug/pprof/profile?seconds=10` |
| `/debug/vars` | `/internal/diagnostics/debug/vars` |

The MCP `target` must include that prefix, for example
`http://service:6060/internal/diagnostics`; the tools append only the
`/debug/...` suffix. A trailing slash is accepted and normalized. The base
path must start with `/`; values such as `internal/diagnostics` are rejected.
URL query, fragment, and router wildcard delimiters are rejected too.
For routers that accept a handler rather than a `*http.ServeMux`, use
`diagnostics.Handler(diagnostics.WithBasePath("/internal/diagnostics"), ...)`
and mount the returned handler under the desired route. Do not mount the
handler under the same prefix twice, or the resulting path will be duplicated.

`WithCustomVars` receives an `*expvar.Map`. Variables registered through the
hook are returned under `gaz_diagnostics.custom`, while database statistics
remain under `gaz_diagnostics.db_pools`:

```go
diagnostics.Register(mux, diagnostics.WithCustomVars(func(vars *expvar.Map) {
	vars.Set("queue_depth", expvar.Func(func() any { return queue.Len() }))
}))
```

The reusable module publishes the following raw metric groups under
`gaz_diagnostics` on every `/debug/vars` request:

| Group | Signals |
|---|---|
| `runtime` | Go version, GOMAXPROCS, goroutine states, thread count, heap and memory limits, allocation/CPU/mutex counters, scheduler latency, and GC pause quantiles |
| `process` | Uptime, RSS and virtual memory, process CPU, I/O bytes, open-file count, and file-descriptor limits |
| `host` | Logical CPUs, physical/swap memory, load, process counts, uptime, global file-handle use, swap activity, and CPU/memory/I/O pressure |
| `cgroup` | cgroup v2 memory, CPU quota/throttling, PID limits, memory events, and CPU/memory/I/O pressure |
| `db_pools` | Optional `database/sql` pool statistics |
| `custom` | Optional application variables |

Runtime values and the logical CPU count are portable. Other process, host,
and cgroup values are included on Linux only when the corresponding kernel data
is available. The open-file metric is a count; file paths and descriptor
targets are not exposed. These values are facts, not diagnoses: the
instrumentation library does not emit heuristic labels.

Block and mutex profiling require runtime sampling. The example enables them
explicitly with `WithBlockProfileRate` and `WithMutexProfileFraction`; omit
those options when the profiles are not needed. Both settings apply to the
whole process and introduce sampling overhead.

The package does not impose authentication. `WithMiddleware` is the hook for
the service's existing authentication/authorization, and `WithIPAllowlist`
adds an optional network boundary; when both are configured, both checks must
pass. Client IP selection follows the service convention: `True-Client-IP`,
then `X-Forwarded-For`, then `RemoteAddr`. Forwarded headers must only be
accepted from a trusted proxy that sanitizes them; the package does not infer
trust from those headers. Invalid supplied client-IP headers are denied when
an allowlist is enabled. `Register` intentionally rejects
`http.DefaultServeMux` and the module never adds `pprof` handlers globally.
However, Go's `expvar` package registers `GET /debug/vars` on the global mux
when imported. A process that uses this module must not serve the global mux;
serve the dedicated mux passed to `Register` (or mount `Handler` inside a
protected router group) so every reachable diagnostics route is protected.

### Baseline: pprof and expvar

If adopting the reusable module is not possible, a service may register the
standard handlers manually on its own `http.ServeMux`:

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

Diagnostic tools do not modify application data or configuration. Profile
capture is nevertheless an active operation: it samples the runtime and
`gc=true` explicitly triggers garbage collection. Tools that accept `target`
require an absolute `http://` or `https://` base URL; credentials, query, and
fragment are rejected, while a base path is supported. By default, diagnostics
accepts a valid HTTPS certificate when its issuer is an unknown private CA, as
well as a valid self-signed certificate, so development endpoints do not block
an authorised inspection. Hostname, validity period, and server-auth usage are
still verified. The result carries a `TLS_UNTRUSTED_PRIVATE_CA` or
`TLS_SELF_SIGNED_CERTIFICATE` alert with severity `info`; it is not a process
health finding. Expired, not-yet-valid, hostname-mismatched, and client-only
certificates remain errors.

| Tool | Parameters | Description |
|---|---|---|
| `capture_profile` | `target`, `type`, `seconds?`, `gc?` | Capture and persist `cpu`, `heap`, `allocs`, `goroutine`, `block`, `mutex`, or `threadcreate`, returning a `profile_id`. CPU defaults to 10 seconds. On non-CPU profiles, `seconds` requests a delta profile. `gc=true` forces GC before a heap snapshot and cannot be combined with `seconds`. |
| `analyze_profile` | `profile_id`, `top_n?` | Return functions ordered by absolute cumulative cost, including raw values, percentages, and per-second rates when the profile carries a duration. `top_n` defaults to 20 and is limited to 1–100. |
| `diff_profiles` | `id_a`, `id_b`, `top_n?` | Compare two stored profiles with the same primary sample type and unit. Returns raw deltas and duration-normalized rate deltas when both durations are available. |
| `generate_flamegraph` | `profile_id` | Generate an SVG flame graph from a stored profile. |
| `analyze_goroutines` | `target` | Fetch `goroutine?debug=2`, group equivalent stacks, retain state and blocked duration, and label known signatures. |
| `get_runtime_stats` | `target` | Read `num_goroutine`, optional `num_threads`, and expvar `memstats`/GC values. |
| `get_db_pool_stats` | `target` | Read the optional `gaz_diagnostics.db_pools` contract. Returns `{ "pools": [...] }`; a target without it returns an empty `pools` array. |
| `get_expvars` | `target` | Return the complete JSON object from `/debug/vars`, including service-specific variables. |
| `diagnose` | `target` | Return two raw metric samples and grouped goroutines, plus a separate list of heuristic suspicions with structured evidence and verification hints. |

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

This first reads goroutines, then takes two `/debug/vars` samples one second
apart. Its top-level contract deliberately separates observation from
interpretation:

- `metrics.before` and `metrics.after` contain the raw runtime,
  process, host, cgroup, pool, and custom values. Each sample records the MCP
  `observed_at` timestamp and preserves target `collected_at` when published;
  `metrics.duration_ns` is calculated from the MCP clock.
- `goroutines` contains the grouped stack evidence without heuristic labels
  mixed into the groups.
- `suspicions` contains heuristic candidates. Each has a code, severity,
  confidence, reason, structured evidence, and a `verify` hint.

This lets an AI or operator reject a suspicion after inspecting the underlying
facts. If `/debug/vars` cannot be read, `diagnose` still returns goroutine data;
if the goroutine profile cannot be read, it still returns metric evidence. The
missing source is identified by an informational alert. The individual focused
tools remain strict and return an error when their endpoint is unavailable.

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

Use `gc=true` when an up-to-date retained-heap snapshot is needed. Use
`seconds=N` on heap, allocs, block, mutex, goroutine, or threadcreate to capture
activity during a defined interval rather than cumulative history. Analysis and
diff output expose `rates_available`; when true, use the per-second fields to
compare profiles with different durations.

### Confirm a connection-pool problem

```text
analyze_goroutines(target="http://orders.internal:6060")
get_db_pool_stats(target="http://orders.internal:6060")
diagnose(target="http://orders.internal:6060")
```

Without the optional pool instrumentation, a `database/sql` waiter remains
useful evidence but the suspicion is only `suspected`. With one instrumented
pool, matching blocked stacks, current saturation, and wait activity can raise
its confidence to `confirmed`. With multiple pools it remains suspected
because a stack cannot identify which `*sql.DB` instance caused the wait.

## Heuristics and diagnosis limits

`analyze_goroutines` labels each matching goroutine group with the following signatures:

| Finding | Conditions | Meaning |
|---|---|---|
| `DB_POOL_EXHAUSTED` | Stack contains `database/sql.(*DB).conn(` and state is `semacquire`, `chan receive`, or `select` | Goroutines wait for a `database/sql` connection. The normal `connectionOpener` worker does not match. |
| `HTTP_CONNECTION_FASTHTTP` | Stack contains `fasthttp.(*Server).serveConn` | `fasthttp` connection handlers share the stack; connections may be active or idle. |
| `HTTP_CONNECTION_NET_HTTP` | Stack contains `net/http.(*conn).serve` | Standard-library HTTP connection handlers share the stack; connections may be active or idle. |
| `LOCK_CONTENTION` | More than one goroutine and a mutex-lock stack frame | Several goroutines contend on a mutex. |
| `POSSIBLE_GOROUTINE_LEAK` | At least 10 goroutines blocked on channel send/receive for at least five minutes | A large, long-lived blocked group may be leaking. |

`diagnose` uses the two metric samples and goroutine stacks to generate these
separate suspicions:

| Suspicion | Trigger evidence |
|---|---|
| `FD_LIMIT_PRESSURE` | Open files reach at least 80% of the soft descriptor limit |
| `POSSIBLE_FD_LEAK` | Open files grow materially during the sample window |
| `HOST_MEMORY_PRESSURE` | The host has at most 10% of physical memory available |
| `HOST_CPU_PRESSURE` | Host CPU PSI reports tasks delayed during at least 20% of the latest 10-second window |
| `SYSTEM_FILE_TABLE_PRESSURE` | The host uses at least 80% of its global file-handle table |
| `CGROUP_MEMORY_PRESSURE` | Current cgroup memory reaches at least 85% of its limit, or new OOM/OOM-kill events occur |
| `GO_MEMORY_LIMIT_MISMATCH` | Go's memory limit leaves almost no cgroup headroom |
| `NATIVE_MEMORY_PRESSURE` | RSS grows by at least 32 MiB without comparable Go runtime-memory growth |
| `CPU_THROTTLED` | At least 10% of observed cgroup CPU periods and at least 1% of the sample window are throttled |
| `GC_PRESSURE` | GC consumes at least 25% of measured Go CPU while allocation continues |
| `SCHEDULER_PRESSURE` | Runnable goroutines exceed GOMAXPROCS in both samples |
| `MUTEX_CONTENTION` | Mutex wait time grows materially in the window; matching blocked stacks increase confidence |
| `PID_LIMIT_PRESSURE` | The cgroup reaches at least 80% of its PID limit |
| `DB_POOL_EXHAUSTED` | Current pool saturation, new waits, and/or matching `database/sql` waiter stacks |
| `LOCK_CONTENTION`, `POSSIBLE_GOROUTINE_LEAK` | Matching grouped-stack signatures |

Counter heuristics use deltas, not historical totals: an old throttling or pool
wait counter does not by itself describe current pressure. Growth heuristics
are skipped if either sample lacks the required metric. A `confirmed`
confidence still appears under `suspicions`; it means multiple independent
signals agree, not that the MCP has declared the root cause.

For `DB_POOL_EXHAUSTED`, confirmation requires exactly one exposed pool,
blocked `database/sql` waiter stacks, `max_open_connections > 0`,
`in_use >= max_open_connections`, `idle == 0`, and a nonzero wait counter.
With multiple pools, stack-to-pool attribution is unavailable and confidence
remains `suspected`.

`get_runtime_stats` remains a compact view over standard `memstats`,
`num_goroutine`, and optional `num_threads`. Use `get_expvars` for the complete
raw endpoint or `diagnose` for the two-sample metric window. If the shared
instrumentation is absent, `diagnose` preserves standard expvar `memstats` and
the optional goroutine/thread values as a fallback.

## Operational notes

- Profile and endpoint responses are capped at 256 MiB. Very large profiles or expvar responses are rejected rather than kept in memory unboundedly.
- `generate_flamegraph` renders an SVG flame graph locally from the captured profile; it does not require Graphviz or the target binary.
- Profile captures are active on the target; interval captures sample for their chosen duration and `gc=true` forces garbage collection. Prefer short, representative intervals and avoid indiscriminate captures on a busy production process.
- `get_expvars` returns the full endpoint payload. Treat it as potentially sensitive and avoid forwarding large raw outputs where a focused analysis is enough.
- For SQL and Jenkins guidance, return to the [documentation map](../README.md#documentation-map).
