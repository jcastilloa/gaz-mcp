package diagnostics

import (
	"bytes"
	"encoding/json"
	"expvar"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"time"

	profiledata "github.com/google/pprof/profile"
)

// Register exposes diagnostics endpoints on mux.
func Register(mux *http.ServeMux, options ...Option) error {
	if mux == nil {
		return fmt.Errorf("diagnostics: mux cannot be nil")
	}
	if mux == http.DefaultServeMux {
		return fmt.Errorf("diagnostics: http.DefaultServeMux cannot be protected; use a dedicated *http.ServeMux")
	}
	config, err := applyOptions(options)
	if err != nil {
		return err
	}
	instrumentation := newInstrumentation(config)
	configureProfiling(config)
	registerPprof(mux, config, instrumentation.protect)
	registerHandler(mux, config.basePath+"/debug/vars", instrumentation.protect(http.HandlerFunc(instrumentation.serveExpvars)))
	return nil
}

func configureProfiling(config config) {
	if config.blockRate > 0 {
		runtime.SetBlockProfileRate(int(config.blockRate))
	}
	if config.mutexRate > 0 {
		runtime.SetMutexProfileFraction(config.mutexRate)
	}
}

// Handler creates a self-contained diagnostics handler. It is useful when an
// application mounts handlers behind a router or middleware group.
func Handler(options ...Option) (http.Handler, error) {
	mux := http.NewServeMux()
	if err := Register(mux, options...); err != nil {
		return nil, err
	}
	return mux, nil
}

// MustHandler creates a diagnostics handler or panics on invalid options.
func MustHandler(options ...Option) http.Handler {
	handler, err := Handler(options...)
	if err != nil {
		panic(err)
	}
	return handler
}

// MustRegister exposes diagnostics endpoints on mux or panics when the
// configuration is invalid.
func MustRegister(mux *http.ServeMux, options ...Option) {
	if err := Register(mux, options...); err != nil {
		panic(err)
	}
}

type instrumentation struct {
	diagnostics *expvar.Map
	protect     func(http.Handler) http.Handler
}

func applyOptions(options []Option) (config, error) {
	config := config{}
	for _, option := range options {
		if option == nil {
			return config, fmt.Errorf("diagnostics: option cannot be nil")
		}
		if err := option(&config); err != nil {
			return config, err
		}
	}
	return config, nil
}

func newInstrumentation(config config) instrumentation {
	customVars := new(expvar.Map)
	for _, register := range config.customVars {
		register(customVars)
	}

	dbPools := new(expvar.Map)
	for _, pool := range config.dbPools {
		pool := pool
		dbPools.Set(pool.name, expvar.Func(func() any {
			stats := pool.db.Stats()
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

	diagnostics := new(expvar.Map)
	diagnostics.Set("collected_at", expvar.Func(func() any { return time.Now().UTC().Format(time.RFC3339Nano) }))
	diagnostics.Set("runtime", expvar.Func(func() any { return collectRuntimeMetrics() }))
	diagnostics.Set("process", expvar.Func(func() any { return collectProcessMetrics("/proc") }))
	diagnostics.Set("host", expvar.Func(func() any { return collectHostMetrics("/proc") }))
	diagnostics.Set("cgroup", expvar.Func(func() any { return collectCgroupMetrics("/proc", "/sys/fs/cgroup") }))
	diagnostics.Set("db_pools", dbPools)
	diagnostics.Set("custom", customVars)
	return instrumentation{diagnostics: diagnostics, protect: securityHandler(config)}
}

func registerPprof(mux *http.ServeMux, config config, protect func(http.Handler) http.Handler) {
	prefix := config.basePath + "/debug/pprof/"
	registerHandler(mux, strings.TrimSuffix(prefix, "/"), protect(http.HandlerFunc(redirectToTrailingSlash)))
	registerHandler(mux, prefix, protect(http.HandlerFunc(rewritePprofPath(config.basePath, servePprof))))
	registerHandler(mux, config.basePath+"/debug/pprof/cmdline", protect(http.HandlerFunc(serveCmdline)))
	registerHandler(mux, config.basePath+"/debug/pprof/profile", protect(http.HandlerFunc(serveCPUProfile)))
	registerHandler(mux, config.basePath+"/debug/pprof/symbol", protect(http.HandlerFunc(serveSymbol)))
	registerMethodHandler(mux, http.MethodPost, config.basePath+"/debug/pprof/symbol", protect(http.HandlerFunc(serveSymbol)))
	registerHandler(mux, config.basePath+"/debug/pprof/trace", protect(http.HandlerFunc(serveTrace)))
}

func redirectToTrailingSlash(writer http.ResponseWriter, request *http.Request) {
	http.Redirect(writer, request, request.URL.Path+"/", http.StatusTemporaryRedirect)
}

func registerHandler(mux *http.ServeMux, path string, handler http.Handler) {
	mux.Handle(http.MethodGet+" "+path, handler)
}

func registerMethodHandler(mux *http.ServeMux, method, path string, handler http.Handler) {
	mux.Handle(method+" "+path, handler)
}

func rewritePprofPath(basePath string, handler http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		clone := request.Clone(request.Context())
		clone.URL.Path = strings.TrimPrefix(request.URL.Path, basePath)
		handler(writer, clone)
	}
}

func servePprof(writer http.ResponseWriter, request *http.Request) {
	name, found := strings.CutPrefix(request.URL.Path, "/debug/pprof/")
	if !found || name == "" {
		servePprofIndex(writer)
		return
	}
	profile := pprof.Lookup(name)
	if profile == nil {
		serveNamedPprof(writer, request, name)
		return
	}
	if seconds := request.FormValue("seconds"); seconds != "" {
		serveDeltaProfile(writer, request, name, profile, seconds)
		return
	}
	if name == "heap" {
		gc, _ := strconv.Atoi(request.FormValue("gc"))
		if gc > 0 {
			runtime.GC()
		}
	}
	debug, _ := strconv.Atoi(request.FormValue("debug"))
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if debug > 0 {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	}
	if err := profile.WriteTo(writer, debug); err != nil {
		http.Error(writer, "could not write profile", http.StatusInternalServerError)
	}
}

var deltaProfiles = map[string]bool{
	"allocs": true, "block": true, "goroutine": true, "heap": true,
	"goroutineleak": true, "mutex": true, "threadcreate": true,
}

func serveDeltaProfile(writer http.ResponseWriter, request *http.Request, name string, runtimeProfile *pprof.Profile, secondsText string) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	seconds, err := strconv.ParseInt(secondsText, 10, 64)
	if err != nil || seconds <= 0 {
		http.Error(writer, `invalid value for "seconds" - must be a positive integer`, http.StatusBadRequest)
		return
	}
	if !deltaProfiles[name] {
		http.Error(writer, `"seconds" parameter is not supported for this profile type`, http.StatusBadRequest)
		return
	}
	debug, _ := strconv.Atoi(request.FormValue("debug"))
	if debug != 0 {
		http.Error(writer, "seconds and debug params are incompatible", http.StatusBadRequest)
		return
	}
	configureWriteDeadline(writer, request, float64(seconds))
	before, err := collectProfile(runtimeProfile)
	if err != nil {
		http.Error(writer, "failed to collect profile", http.StatusInternalServerError)
		return
	}
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-request.Context().Done():
		http.Error(writer, request.Context().Err().Error(), http.StatusInternalServerError)
		return
	case <-timer.C:
	}
	after, err := collectProfile(runtimeProfile)
	if err != nil {
		http.Error(writer, "failed to collect profile", http.StatusInternalServerError)
		return
	}
	timestamp := after.TimeNanos
	duration := after.TimeNanos - before.TimeNanos
	before.Scale(-1)
	delta, err := profiledata.Merge([]*profiledata.Profile{before, after})
	if err != nil {
		http.Error(writer, "failed to compute delta profile", http.StatusInternalServerError)
		return
	}
	delta.TimeNanos = timestamp
	delta.DurationNanos = duration
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-delta"`, name))
	if err := delta.Write(writer); err != nil {
		return
	}
}

func collectProfile(runtimeProfile *pprof.Profile) (*profiledata.Profile, error) {
	var output bytes.Buffer
	if err := runtimeProfile.WriteTo(&output, 0); err != nil {
		return nil, err
	}
	profile, err := profiledata.Parse(&output)
	if err != nil {
		return nil, err
	}
	profile.TimeNanos = time.Now().UnixNano()
	return profile, nil
}

func serveNamedPprof(writer http.ResponseWriter, request *http.Request, name string) {
	if name == "cmdline" {
		serveCmdline(writer, request)
		return
	}
	if name == "profile" {
		serveCPUProfile(writer, request)
		return
	}
	if name == "symbol" {
		serveSymbol(writer, request)
		return
	}
	if name == "trace" {
		serveTrace(writer, request)
		return
	}
	http.NotFound(writer, request)
}

func servePprofIndex(writer http.ResponseWriter) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("<html><head><title>/debug/pprof/</title></head><body>\n"))
	profiles := pprof.Profiles()
	for _, profile := range profiles {
		writePprofLink(writer, profile.Name())
	}
	for _, name := range []string{"cmdline", "profile", "symbol", "trace"} {
		_, _ = fmt.Fprintf(writer, "<a href='%s'>%s</a><br>\n", name, name)
	}
	_, _ = writer.Write([]byte("</body></html>\n"))
}

func writePprofLink(writer io.Writer, name string) {
	_, _ = fmt.Fprintf(writer, "<a href='%s?debug=1'>%s</a><br>\n", html.EscapeString(url.PathEscape(name)), html.EscapeString(name))
}

func serveCmdline(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = writer.Write([]byte(strings.Join(os.Args, "\x00")))
}

func serveCPUProfile(writer http.ResponseWriter, request *http.Request) {
	seconds, _ := strconv.Atoi(request.FormValue("seconds"))
	if seconds <= 0 {
		seconds = 30
	}
	configureWriteDeadline(writer, request, float64(seconds))
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Disposition", `attachment; filename="profile"`)
	if err := pprof.StartCPUProfile(writer); err != nil {
		http.Error(writer, "could not enable CPU profiling", http.StatusInternalServerError)
		return
	}
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	select {
	case <-timer.C:
	case <-request.Context().Done():
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	pprof.StopCPUProfile()
}

func serveTrace(writer http.ResponseWriter, request *http.Request) {
	seconds, _ := strconv.ParseFloat(request.FormValue("seconds"), 64)
	if seconds <= 0 {
		seconds = 1
	}
	configureWriteDeadline(writer, request, seconds)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Disposition", `attachment; filename="trace"`)
	if err := trace.Start(writer); err != nil {
		http.Error(writer, "could not enable execution tracing", http.StatusInternalServerError)
		return
	}
	timer := time.NewTimer(time.Duration(seconds * float64(time.Second)))
	select {
	case <-timer.C:
	case <-request.Context().Done():
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	trace.Stop()
}

func configureWriteDeadline(writer http.ResponseWriter, request *http.Request, seconds float64) {
	server, ok := request.Context().Value(http.ServerContextKey).(*http.Server)
	if !ok || server.WriteTimeout <= 0 {
		return
	}
	deadline := time.Now().Add(server.WriteTimeout + time.Duration(seconds*float64(time.Second)))
	_ = http.NewResponseController(writer).SetWriteDeadline(deadline)
}

func serveSymbol(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var input string
	if request.Method == http.MethodPost {
		var body bytes.Buffer
		_, _ = body.ReadFrom(request.Body)
		input = body.String()
	} else {
		input = request.URL.RawQuery
	}
	_, _ = writer.Write([]byte("num_symbols: 1\n"))
	for _, word := range strings.Split(input, "+") {
		pc, _ := strconv.ParseUint(word, 0, 64)
		if pc == 0 {
			continue
		}
		if function := runtime.FuncForPC(uintptr(pc)); function != nil {
			_, _ = fmt.Fprintf(writer, "%#x %s\n", pc, function.Name())
		}
	}
}

func (instrumentation instrumentation) serveExpvars(writer http.ResponseWriter, _ *http.Request) {
	variables := make(map[string]json.RawMessage)
	expvar.Do(func(entry expvar.KeyValue) {
		variables[entry.Key] = json.RawMessage(entry.Value.String())
	})
	variables["num_goroutine"] = json.RawMessage(strconv.Itoa(runtime.NumGoroutine()))
	variables["gaz_diagnostics"] = json.RawMessage(instrumentation.diagnostics.String())

	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(writer).Encode(variables); err != nil {
		http.Error(writer, "could not encode expvar response", http.StatusInternalServerError)
	}
}

func securityHandler(config config) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		for index := len(config.middlewares) - 1; index >= 0; index-- {
			handler = config.middlewares[index](handler)
		}
		if len(config.allowlist) == 0 {
			return handler
		}
		return ipAllowlistHandler(config.allowlist, handler)
	}
}

func ipAllowlistHandler(allowlist []netip.Prefix, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		address, valid := clientIP(request)
		if !valid || !isAllowed(address, allowlist) {
			http.Error(writer, "diagnostics access denied", http.StatusForbidden)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func isAllowed(address netip.Addr, allowlist []netip.Prefix) bool {
	for _, prefix := range allowlist {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
