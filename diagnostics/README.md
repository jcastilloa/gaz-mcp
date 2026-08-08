# gaz-mcp diagnostics module

`github.com/jcastilloa/gaz-mcp/diagnostics` is a standalone Go module that
adds `pprof` and `expvar` endpoints to a service, with optional access
controls. It has no MCP,
Jenkins, or database-driver dependencies.

```bash
go get github.com/jcastilloa/gaz-mcp/diagnostics@latest
```

Register it on a dedicated mux:

```go
mux := http.NewServeMux()
err := diagnostics.Register(mux,
	diagnostics.WithBasePath("/internal/diagnostics"),
	diagnostics.WithDBPool("primary", db),
	diagnostics.WithBlockProfileRate(time.Millisecond),
	diagnostics.WithMutexProfileFraction(5),
	diagnostics.WithCustomVars(func(vars *expvar.Map) {
		vars.Set("queue_depth", expvar.Func(func() any { return queue.Len() }))
	}),
	diagnostics.WithIPAllowlist("10.0.0.0/8"),
	diagnostics.WithMiddleware(requireInternalOperator),
)
```

For startup code that treats invalid options as a programming error, the same
setup can be a single call:

```go
diagnostics.MustRegister(mux, diagnostics.WithDBPool("primary", db))
```

Without `WithBasePath`, the endpoints are `/debug/pprof/` and
`/debug/vars`. To use a custom route, provide an absolute prefix:

```go
diagnostics.Register(mux, diagnostics.WithBasePath("/internal/diagnostics"))
```

The resulting routes are `/internal/diagnostics/debug/pprof/`,
`/internal/diagnostics/debug/pprof/goroutine?debug=2`, and
`/internal/diagnostics/debug/vars`. Therefore the MCP call must use
`target="http://service:6060/internal/diagnostics"`. A trailing slash is
normalized; a relative prefix and URL query, fragment, or wildcard delimiters
are rejected. `WithDBPool` is optional and publishes `db.Stats()` under
`gaz_diagnostics.db_pools`. Custom variables are placed under
`gaz_diagnostics.custom`. A service without SQL can call `Register(mux)`.

Every `/debug/vars` response also publishes raw facts below
`gaz_diagnostics`:

| Object | Included facts |
|---|---|
| `runtime` | Go version, GOMAXPROCS, goroutine states, threads, heap and memory limits, allocation/CPU/mutex counters, scheduler latency, and GC pause quantiles |
| `process` | Uptime, RSS and virtual memory, process CPU, I/O bytes, open-file count and file-descriptor limits |
| `host` | Logical CPUs, physical/swap memory, load, process counts, uptime, global file-handle use, swap activity, and CPU/memory/I/O pressure |
| `cgroup` | cgroup v2 memory, CPU quota/throttling, PID limits, memory events, and CPU/memory/I/O pressure |
| `db_pools` | Optional `database/sql` pool statistics |
| `custom` | Optional application-specific expvars |

Runtime facts and the logical CPU count are portable. The other process, host,
and cgroup facts are populated on Linux when the corresponding kernel files
are readable; unavailable facts are omitted. The module publishes only the
number of open file descriptors, never their paths or targets. It does not
classify these values: heuristic suspicions are produced separately by the MCP
`diagnose` tool.

Block and mutex sampling are disabled by the Go runtime by default. Enable
them explicitly with `WithBlockProfileRate` and
`WithMutexProfileFraction` when those profiles are needed. These settings
affect the whole process and add sampling overhead, so choose rates appropriate
for the service workload.

The pprof handlers support the standard profile query semantics without
registering anything on `http.DefaultServeMux`: `gc=1` forces a collection
before a heap snapshot, and `seconds=N` returns a delta profile for allocs,
block, goroutine, heap, mutex, and threadcreate. Delta profiling requires a
positive integer duration and cannot be combined with text (`debug`) output.

`WithIPAllowlist` accepts individual IPs or CIDR ranges. If configured, the
client address is selected in this order: `True-Client-IP`,
`X-Forwarded-For`, then `RemoteAddr`; invalid supplied headers are denied.
Only use forwarded headers when a trusted proxy sanitizes them. The allowlist
and any `WithMiddleware` wrapper apply to every diagnostics endpoint.

`Register` rejects `http.DefaultServeMux` and this package never registers
`pprof` handlers there. However, Go's standard `expvar` package registers
`GET /debug/vars` on the global mux when it is imported. Do not serve
`http.DefaultServeMux` in a process that uses this module; serve the dedicated
mux passed to `Register`, or use `Handler(options...)` with a protected router
group. The package does not impose an authentication scheme; integrate the
service's existing authentication with `WithMiddleware` and keep diagnostics
on a private network or behind a trusted proxy.

See the [full Go process diagnostics guide](../docs/go-process-diagnostics.md)
for the expvar contract and MCP workflows. Agents using gaz-mcp to inspect an
instrumented service can install the self-contained
[`gaz-mcp-diagnostics` skill](../skills/gaz-mcp-diagnostics/SKILL.md).
