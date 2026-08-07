package diagnostics

import (
	"bytes"
	"encoding/json"
	"expvar"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"time"
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
	registerPprof(mux, config, instrumentation.protect)
	registerHandler(mux, config.basePath+"/debug/vars", instrumentation.protect(http.HandlerFunc(instrumentation.serveExpvars)))
	return nil
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
	debug, _ := strconv.Atoi(request.FormValue("debug"))
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if debug > 0 {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		writer.Header().Set("Content-Type", "application/octet-stream")
	}
	if err := profile.WriteTo(writer, debug); err != nil {
		http.Error(writer, "could not write profile", http.StatusInternalServerError)
	}
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
		_, _ = fmt.Fprintf(writer, "<a href='%s?debug=1'>%s</a><br>\n", profile.Name(), profile.Name())
	}
	for _, name := range []string{"cmdline", "profile", "symbol", "trace"} {
		_, _ = fmt.Fprintf(writer, "<a href='%s'>%s</a><br>\n", name, name)
	}
	_, _ = writer.Write([]byte("</body></html>\n"))
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
	if seconds > 60 {
		seconds = 60
	}
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
	if seconds > 10 {
		seconds = 10
	}
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "application/octet-stream")
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
