# gaz-mcp diagnostics module

`github.com/jcastilloa/gaz-mcp/diagnostics` is a standalone Go module that
adds `pprof` and `expvar` endpoints to a service, with optional access
controls. It has no MCP,
Jenkins, or database-driver dependencies.

```bash
go get github.com/jcastilloa/gaz-mcp/diagnostics@v0.1.0
```

Register it on a dedicated mux:

```go
mux := http.NewServeMux()
err := diagnostics.Register(mux,
	diagnostics.WithBasePath("/internal/diagnostics"),
	diagnostics.WithDBPool("primary", db),
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
for the expvar contract and MCP workflows.
