package diagnostics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
