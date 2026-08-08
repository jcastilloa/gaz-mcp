package diagnostics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

func TestResponseSizeLimitIs256MiB(t *testing.T) {
	const expectedResponseLimit = 256 << 20
	if maxResponseBytes != expectedResponseLimit {
		t.Fatalf("response size limit = %d, want %d", maxResponseBytes, expectedResponseLimit)
	}
}

func TestRepositoryFetchesExpectedDiagnosticEndpoints(t *testing.T) {
	requests := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.RequestURI())
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("response"))
	}))
	defer server.Close()

	repository := NewRepository()
	if _, err := repository.FetchProfile(context.Background(), server.URL, "cpu", 12); err != nil {
		t.Fatalf("fetch CPU profile: %v", err)
	}
	if _, err := repository.FetchGoroutines(context.Background(), server.URL); err != nil {
		t.Fatalf("fetch goroutines: %v", err)
	}
	if _, err := repository.FetchExpvars(context.Background(), server.URL); err != nil {
		t.Fatalf("fetch expvars: %v", err)
	}

	want := []string{"/debug/pprof/profile?seconds=12", "/debug/pprof/goroutine?debug=2", "/debug/vars"}
	for index, uri := range want {
		if requests[index] != uri {
			t.Fatalf("request %d = %q, want %q", index, requests[index], uri)
		}
	}
}

func TestRepositoryFetchesSelfSignedHTTPSTarget(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/debug/vars" {
			t.Fatalf("path = %q, want /debug/vars", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"num_goroutine": 1}`))
	}))
	defer server.Close()

	result, err := NewRepository().FetchExpvars(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch expvars from self-signed HTTPS target: %v", err)
	}
	if string(result.Data) != `{"num_goroutine": 1}` {
		t.Fatalf("response = %q", result.Data)
	}
	if len(result.Alerts) != 1 || result.Alerts[0].Code != selfSignedTLSAlertCode || result.Alerts[0].Severity != "info" {
		t.Fatalf("alerts = %#v, want one informational self-signed TLS alert", result.Alerts)
	}
}

func TestRepositoryFetchesAllEndpointsFromSelfSignedHTTPSTarget(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("response"))
	}))
	defer server.Close()

	repository := NewRepository()
	results := make([]domain.FetchResult, 0, 3)
	profile, err := repository.FetchProfile(context.Background(), server.URL, "heap", 0)
	if err != nil {
		t.Fatalf("fetch profile: %v", err)
	}
	results = append(results, profile)
	goroutines, err := repository.FetchGoroutines(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch goroutines: %v", err)
	}
	results = append(results, goroutines)
	expvars, err := repository.FetchExpvars(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch expvars: %v", err)
	}
	results = append(results, expvars)

	for index, result := range results {
		if len(result.Alerts) != 1 || result.Alerts[0].Severity != "info" {
			t.Fatalf("result %d alerts = %#v, want one informational alert", index, result.Alerts)
		}
	}
}

func TestRepositoryDoesNotReportTLSAlertForTrustedHTTPS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("{}"))
	}))
	defer server.Close()

	repository := NewRepository()
	repository.client = server.Client()
	result, err := repository.FetchExpvars(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch expvars: %v", err)
	}
	if len(result.Alerts) != 0 {
		t.Fatalf("alerts = %#v, want no alert for trusted HTTPS", result.Alerts)
	}
}

func TestRepositoryDoesNotReportTLSAlertForHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("{}"))
	}))
	defer server.Close()

	result, err := NewRepository().FetchExpvars(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch expvars: %v", err)
	}
	if len(result.Alerts) != 0 {
		t.Fatalf("alerts = %#v, want no alert for HTTP", result.Alerts)
	}
}

func TestRepositoryRejectsInvalidTarget(t *testing.T) {
	_, err := NewRepository().FetchExpvars(context.Background(), "service:6060")
	if err == nil {
		t.Fatal("expected an invalid target error")
	}
}

func TestRepositoryPreservesTargetBasePath(t *testing.T) {
	requests := make([]string, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.RequestURI())
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("{}"))
	}))
	defer server.Close()

	if _, err := NewRepository().FetchExpvars(context.Background(), server.URL+"/internal/diagnostics"); err != nil {
		t.Fatalf("fetch expvars: %v", err)
	}
	if got, want := requests[0], "/internal/diagnostics/debug/vars"; got != want {
		t.Fatalf("request = %q, want %q", got, want)
	}
}
