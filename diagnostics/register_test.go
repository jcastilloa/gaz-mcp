package diagnostics

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"expvar"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterExposesDefaultPprofAndExpvarRoutes(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux); err != nil {
		t.Fatalf("register: %v", err)
	}

	pprofResponse := request(mux, "/debug/pprof/goroutine?debug=1", "127.0.0.1:1000", "", "")
	if pprofResponse.Code != http.StatusOK {
		t.Fatalf("pprof status = %d, want %d", pprofResponse.Code, http.StatusOK)
	}
	vars := varsResponse(t, request(mux, "/debug/vars", "127.0.0.1:1000", "", ""))
	if _, ok := vars["memstats"]; !ok {
		t.Fatal("expvar response does not include memstats")
	}
	if _, ok := vars["num_goroutine"]; !ok {
		t.Fatal("expvar response does not include num_goroutine")
	}
}

func TestRegisterSupportsCustomBasePath(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux, WithBasePath("/internal/diagnostics/")); err != nil {
		t.Fatalf("register: %v", err)
	}

	if response := request(mux, "/debug/vars", "127.0.0.1:1000", "", ""); response.Code != http.StatusNotFound {
		t.Fatalf("default vars status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if response := request(mux, "/internal/diagnostics/debug/pprof/goroutine?debug=1", "127.0.0.1:1000", "", ""); response.Code != http.StatusOK {
		t.Fatalf("custom pprof status = %d, want %d", response.Code, http.StatusOK)
	}
	varsResponse(t, request(mux, "/internal/diagnostics/debug/vars", "127.0.0.1:1000", "", ""))
}

func TestHandlerSupportsCustomBasePath(t *testing.T) {
	handler, err := Handler(WithBasePath("/internal/diagnostics"))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	response := request(handler, "/internal/diagnostics/debug/vars", "127.0.0.1:1000", "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("custom handler status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestRegisterServesPprofAuxiliaryEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux); err != nil {
		t.Fatalf("register: %v", err)
	}

	index := request(mux, "/debug/pprof/", "127.0.0.1:1000", "", "")
	if index.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", index.Code, http.StatusOK)
	}
	if !bytes.Contains(index.Body.Bytes(), []byte("goroutine")) {
		t.Fatalf("index body = %q, want goroutine profile", index.Body.String())
	}

	cmdline := request(mux, "/debug/pprof/cmdline", "127.0.0.1:1000", "", "")
	if cmdline.Code != http.StatusOK {
		t.Fatalf("cmdline status = %d, want %d", cmdline.Code, http.StatusOK)
	}

	unknown := request(mux, "/debug/pprof/not-a-profile", "127.0.0.1:1000", "", "")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown profile status = %d, want %d", unknown.Code, http.StatusNotFound)
	}
}

func TestRegisterServesSymbolForGetAndPost(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux); err != nil {
		t.Fatalf("register: %v", err)
	}

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/debug/pprof/symbol?0x1", nil),
		httptest.NewRequest(http.MethodPost, "/debug/pprof/symbol", strings.NewReader("0x1")),
	} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s symbol status = %d, want %d", request.Method, response.Code, http.StatusOK)
		}
		if !bytes.HasPrefix(response.Body.Bytes(), []byte("num_symbols: 1\n")) {
			t.Fatalf("%s symbol body = %q", request.Method, response.Body.String())
		}
	}
}

func TestRegisterServesCPUProfileAndTrace(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux); err != nil {
		t.Fatalf("register: %v", err)
	}

	for _, path := range []string{"/debug/pprof/profile?seconds=1", "/debug/pprof/trace?seconds=1"} {
		context, cancel := context.WithCancel(context.Background())
		cancel()
		request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(context)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, response.Code, http.StatusOK)
		}
		if response.Header().Get("Content-Type") != "application/octet-stream" {
			t.Fatalf("%s content type = %q, want application/octet-stream", path, response.Header().Get("Content-Type"))
		}
	}
}

func TestRegisterPublishesDBPoolsAndCustomVars(t *testing.T) {
	database := sql.OpenDB(testConnector{})
	t.Cleanup(func() { _ = database.Close() })
	database.SetMaxOpenConns(7)

	mux := http.NewServeMux()
	if err := Register(mux,
		WithDBPool("test-primary", database),
		WithCustomVars(func(vars *expvar.Map) {
			vars.Set("queue_depth", expvar.Func(func() any { return 42 }))
		}),
	); err != nil {
		t.Fatalf("register: %v", err)
	}

	vars := varsResponse(t, request(mux, "/debug/vars", "127.0.0.1:1000", "", ""))
	diagnostics := object(t, vars["gaz_diagnostics"])
	pools := object(t, diagnostics["db_pools"])
	pool := object(t, pools["test-primary"])
	if maxOpenConnections, ok := pool["max_open_connections"].(float64); !ok || maxOpenConnections != 7 {
		t.Fatalf("max_open_connections = %#v, want 7", pool["max_open_connections"])
	}
	custom := object(t, diagnostics["custom"])
	if queueDepth, ok := custom["queue_depth"].(float64); !ok || queueDepth != 42 {
		t.Fatalf("queue_depth = %#v, want 42", custom["queue_depth"])
	}
}

func TestRegisterIPAllowlistUsesTrueClientIPBeforeForwardedAndRemoteAddress(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux, WithIPAllowlist("10.0.0.0/8")); err != nil {
		t.Fatalf("register: %v", err)
	}

	allowed := request(mux, "/debug/vars", "203.0.113.10:1000", "10.1.2.3", "192.0.2.5")
	if allowed.Code != http.StatusOK {
		t.Fatalf("allowed status = %d, want %d", allowed.Code, http.StatusOK)
	}
	denied := request(mux, "/debug/vars", "10.1.2.3:1000", "203.0.113.10", "10.1.2.3")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("denied status = %d, want %d", denied.Code, http.StatusForbidden)
	}
}

func TestRegisterIPAllowlistUsesForwardedIPThenRemoteAddress(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux, WithIPAllowlist("10.0.0.0/8")); err != nil {
		t.Fatalf("register: %v", err)
	}

	forwarded := request(mux, "/debug/vars", "203.0.113.10:1000", "", "10.1.2.3")
	if forwarded.Code != http.StatusOK {
		t.Fatalf("forwarded status = %d, want %d", forwarded.Code, http.StatusOK)
	}
	remote := request(mux, "/debug/vars", "10.1.2.3:1000", "", "")
	if remote.Code != http.StatusOK {
		t.Fatalf("remote status = %d, want %d", remote.Code, http.StatusOK)
	}
}

func TestRegisterIPAllowlistRejectsInvalidClientHeader(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux, WithIPAllowlist("10.0.0.0/8")); err != nil {
		t.Fatalf("register: %v", err)
	}

	response := request(mux, "/debug/vars", "10.1.2.3:1000", "not-an-ip", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestRegisterIPAllowlistProtectsPprofRedirect(t *testing.T) {
	mux := http.NewServeMux()
	if err := Register(mux, WithIPAllowlist("10.0.0.0/8")); err != nil {
		t.Fatalf("register: %v", err)
	}

	response := request(mux, "/debug/pprof", "203.0.113.10:1000", "", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestRegisterAppliesCustomMiddlewareToEveryEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("X-Diagnostics-Middleware", "applied")
			next.ServeHTTP(writer, request)
		})
	}
	if err := Register(mux, WithMiddleware(middleware)); err != nil {
		t.Fatalf("register: %v", err)
	}

	for _, path := range []string{"/debug/vars", "/debug/pprof/goroutine?debug=1"} {
		response := request(mux, path, "127.0.0.1:1000", "", "")
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, response.Code, http.StatusOK)
		}
		if got := response.Header().Get("X-Diagnostics-Middleware"); got != "applied" {
			t.Fatalf("%s middleware header = %q, want applied", path, got)
		}
	}
}

func TestRegisterRejectsInvalidConfiguration(t *testing.T) {
	if err := Register(nil); err == nil {
		t.Fatal("expected nil mux error")
	}
	if err := Register(http.NewServeMux(), WithBasePath("relative")); err == nil {
		t.Fatal("expected invalid base path error")
	}
	if err := Register(http.NewServeMux(), WithBasePath("/internal/diagnostics?debug=1")); err == nil {
		t.Fatal("expected base path with query delimiter error")
	}
	if err := Register(http.NewServeMux(), WithBasePath("/internal/diagnostics#fragment")); err == nil {
		t.Fatal("expected base path with fragment delimiter error")
	}
	if err := Register(http.NewServeMux(), WithBasePath("/internal/{diagnostics}")); err == nil {
		t.Fatal("expected base path with mux wildcard delimiter error")
	}
	if err := Register(http.NewServeMux(), WithIPAllowlist("not-a-network")); err == nil {
		t.Fatal("expected invalid CIDR error")
	}
	if err := Register(http.NewServeMux(), WithIPAllowlist()); err == nil {
		t.Fatal("expected empty allowlist error")
	}
	if err := Register(http.NewServeMux(), WithDBPool("primary", nil)); err == nil {
		t.Fatal("expected nil database error")
	}
	if err := Register(http.NewServeMux(), WithCustomVars(nil)); err == nil {
		t.Fatal("expected nil custom vars error")
	}
	if err := Register(http.NewServeMux(), WithMiddleware(nil)); err == nil {
		t.Fatal("expected nil middleware error")
	}
	if err := Register(http.NewServeMux(), nil); err == nil {
		t.Fatal("expected nil option error")
	}
	if err := Register(http.DefaultServeMux); err == nil {
		t.Fatal("expected default mux error")
	}
}

func TestImportDoesNotRegisterGlobalPprofRoutes(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	response := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("default mux pprof status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestStandardExpvarRegistersDefaultMuxRoute(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/debug/vars", nil)
	response := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("default mux expvar status = %d, want %d", response.Code, http.StatusOK)
	}
}

func request(handler http.Handler, path, remoteAddress, trueClientIP, forwardedFor string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = remoteAddress
	if trueClientIP != "" {
		request.Header.Set("True-Client-IP", trueClientIP)
	}
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func varsResponse(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("vars status = %d, want %d", response.Code, http.StatusOK)
	}
	vars := make(map[string]any)
	if err := json.Unmarshal(response.Body.Bytes(), &vars); err != nil {
		t.Fatalf("decode vars: %v", err)
	}
	return vars
}

func object(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want JSON object", value)
	}
	return object
}

type testConnector struct{}

func (testConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("not used")
}

func (testConnector) Driver() driver.Driver { return testDriver{} }

type testDriver struct{}

func (testDriver) Open(string) (driver.Conn, error) { return nil, errors.New("not used") }

func TestMustRegisterPanicsOnInvalidConfiguration(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister did not panic")
		}
	}()
	MustRegister(http.NewServeMux(), WithBasePath(strings.Repeat("x", 1)))
}

func TestHandlerAndMustHandlerRejectInvalidConfiguration(t *testing.T) {
	if handler, err := Handler(WithBasePath("relative")); err == nil || handler != nil {
		t.Fatalf("handler = %v, error = %v, want nil handler and error", handler, err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("MustHandler did not panic")
		}
	}()
	MustHandler(WithBasePath("relative"))
}

func TestMustHandlerCreatesHandler(t *testing.T) {
	handler := MustHandler()
	response := request(handler, "/debug/vars", "127.0.0.1:1000", "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}
