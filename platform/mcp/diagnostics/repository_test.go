package diagnostics

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	if _, err := repository.FetchProfile(context.Background(), domain.ProfileRequest{Target: server.URL, Type: "cpu", Seconds: 12}); err != nil {
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

func TestRepositoryForwardsDeltaAndHeapGCProfileOptions(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.RequestURI())
		_, _ = writer.Write([]byte("response"))
	}))
	defer server.Close()
	repository := NewRepository()
	if _, err := repository.FetchProfile(context.Background(), domain.ProfileRequest{Target: server.URL, Type: "mutex", Seconds: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FetchProfile(context.Background(), domain.ProfileRequest{Target: server.URL, Type: "heap", ForceGC: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"/debug/pprof/mutex?seconds=3", "/debug/pprof/heap?gc=1"}
	for index := range want {
		if requests[index] != want[index] {
			t.Errorf("request %d = %q, want %q", index, requests[index], want[index])
		}
	}
}

func TestRepositoryFetchesSelfSignedHTTPSTarget(t *testing.T) {
	server := newSelfSignedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/debug/vars" {
			t.Fatalf("path = %q, want /debug/vars", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"num_goroutine": 1}`))
	}))

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

func TestRepositoryFetchesAllEndpointsWithUnknownPrivateCAByDefault(t *testing.T) {
	server := newPrivateCATLSServer(t, privateCAServerCertificateOptions{})
	repository := NewRepository()

	expvars, err := repository.FetchExpvars(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch expvars from private CA HTTPS target: %v", err)
	}
	goroutines, err := repository.FetchGoroutines(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch goroutines from private CA HTTPS target: %v", err)
	}
	profile, err := repository.FetchProfile(context.Background(), domain.ProfileRequest{Target: server.URL, Type: "heap"})
	if err != nil {
		t.Fatalf("fetch profile from private CA HTTPS target: %v", err)
	}

	for index, result := range []domain.FetchResult{expvars, goroutines, profile} {
		if string(result.Data) != `{"num_goroutine": 1}` {
			t.Fatalf("result %d response = %q", index, result.Data)
		}
		if len(result.Alerts) != 1 || result.Alerts[0].Code != "TLS_UNTRUSTED_PRIVATE_CA" || result.Alerts[0].Severity != "info" {
			t.Fatalf("result %d alerts = %#v, want one informational private CA TLS alert", index, result.Alerts)
		}
	}
}

func TestRepositoryRejectsUnknownPrivateCAWithWrongHostname(t *testing.T) {
	server := newPrivateCATLSServer(t, privateCAServerCertificateOptions{dnsNames: []string{"other.test"}})

	if _, err := NewRepository().FetchExpvars(context.Background(), server.URL); err == nil {
		t.Fatal("expected hostname verification error")
	}
}

func TestRepositoryRejectsExpiredUnknownPrivateCACertificate(t *testing.T) {
	now := time.Now()
	server := newPrivateCATLSServer(t, privateCAServerCertificateOptions{
		notBefore: now.Add(-2 * time.Hour),
		notAfter:  now.Add(-time.Hour),
	})

	if _, err := NewRepository().FetchExpvars(context.Background(), server.URL); err == nil {
		t.Fatal("expected expired certificate error")
	}
}

func TestUnknownPrivateCAWildcardMatchesDevelopmentHostname(t *testing.T) {
	server := newPrivateCATLSServer(t, privateCAServerCertificateOptions{dnsNames: []string{"*.dev1.test"}})
	leaf, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}

	if err := verifyUntrustedCertificate(state, "editor.dev1.test"); err != nil {
		t.Fatalf("verify editor.dev1.test: %v", err)
	}
	if err := verifyUntrustedCertificate(state, "nested.editor.dev1.test"); err == nil {
		t.Fatal("expected nested hostname outside wildcard scope to fail")
	}
}

func TestRepositoryFetchesAllEndpointsFromSelfSignedHTTPSTarget(t *testing.T) {
	server := newSelfSignedTLSServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("response"))
	}))

	repository := NewRepository()
	results := make([]domain.FetchResult, 0, 3)
	profile, err := repository.FetchProfile(context.Background(), domain.ProfileRequest{Target: server.URL, Type: "heap"})
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

func TestRepositoryRejectsTargetCredentialsAndQuery(t *testing.T) {
	for _, target := range []string{
		"https://user:secret@service.example",
		"https://service.example?token=secret",
		"https://service.example#fragment",
	} {
		if _, err := targetEndpoint(target, "debug/vars"); err == nil {
			t.Errorf("target %q should be rejected", target)
		}
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

type privateCAServerCertificateOptions struct {
	dnsNames  []string
	notBefore time.Time
	notAfter  time.Time
}

func newPrivateCATLSServer(t *testing.T, options privateCAServerCertificateOptions) *httptest.Server {
	t.Helper()
	now := time.Now()
	if options.notBefore.IsZero() {
		options.notBefore = now.Add(-time.Hour)
	}
	if options.notAfter.IsZero() {
		options.notAfter = now.Add(time.Hour)
	}

	_, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Development CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	_, leafKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "diagnostics.dev1.test"},
		DNSNames:     options.dnsNames,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    options.notBefore,
		NotAfter:     options.notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if len(options.dnsNames) > 0 {
		leafTemplate.IPAddresses = nil
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCertificate, leafKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"num_goroutine": 1}`))
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{
		Certificate: [][]byte{leafDER},
		PrivateKey:  leafKey,
	}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func newSelfSignedTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	now := time.Now()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(3),
		Subject:               pkix.Name{CommonName: "diagnostics.dev1.test"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{
		Certificate: [][]byte{certificateDER},
		PrivateKey:  privateKey,
	}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}
