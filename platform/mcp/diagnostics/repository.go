package diagnostics

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/diagnostics"
)

const maxResponseBytes = 256 << 20

const (
	selfSignedTLSAlertCode = "TLS_SELF_SIGNED_CERTIFICATE"
	privateCATLSAlertCode  = "TLS_UNTRUSTED_PRIVATE_CA"
)

type Repository struct {
	client *http.Client
}

func NewRepository() *Repository {
	return &Repository{
		client: &http.Client{Timeout: 70 * time.Second},
	}
}

func (r *Repository) FetchProfile(ctx context.Context, request domain.ProfileRequest) (domain.FetchResult, error) {
	endpointName := request.Type
	if request.Type == "cpu" {
		endpointName = "profile"
	}
	endpoint, err := targetEndpoint(request.Target, path.Join("debug/pprof", endpointName))
	if err != nil {
		return domain.FetchResult{}, err
	}
	if request.Seconds > 0 || request.ForceGC {
		query := endpoint.Query()
		if request.Seconds > 0 {
			query.Set("seconds", strconv.Itoa(request.Seconds))
		}
		if request.ForceGC {
			query.Set("gc", "1")
		}
		endpoint.RawQuery = query.Encode()
	}
	return r.fetch(ctx, endpoint.String())
}

func (r *Repository) FetchGoroutines(ctx context.Context, target string) (domain.FetchResult, error) {
	endpoint, err := targetEndpoint(target, "debug/pprof/goroutine")
	if err != nil {
		return domain.FetchResult{}, err
	}
	query := endpoint.Query()
	query.Set("debug", "2")
	endpoint.RawQuery = query.Encode()
	return r.fetch(ctx, endpoint.String())
}

func (r *Repository) FetchExpvars(ctx context.Context, target string) (domain.FetchResult, error) {
	endpoint, err := targetEndpoint(target, "debug/vars")
	if err != nil {
		return domain.FetchResult{}, err
	}
	return r.fetch(ctx, endpoint.String())
}

func (r *Repository) fetch(ctx context.Context, endpoint string) (domain.FetchResult, error) {
	data, _, err := r.fetchWithClient(ctx, r.client, endpoint)
	if err == nil || !isUnknownAuthority(err) {
		return domain.FetchResult{Data: data}, err
	}

	untrustedClient, err := untrustedTLSClient(endpoint)
	if err != nil {
		return domain.FetchResult{}, err
	}
	defer untrustedClient.CloseIdleConnections()
	data, state, err := r.fetchWithClient(ctx, untrustedClient, endpoint)
	if err != nil {
		return domain.FetchResult{}, err
	}
	alert := untrustedTLSAlert(state)
	return domain.FetchResult{
		Data:   data,
		Alerts: []domain.InspectionAlert{alert},
	}, nil
}

func (r *Repository) fetchWithClient(ctx context.Context, client *http.Client, endpoint string) ([]byte, *tls.ConnectionState, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, response.TLS, fmt.Errorf("GET %s: unexpected HTTP status %s", endpoint, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, response.TLS, err
	}
	if len(data) > maxResponseBytes {
		return nil, response.TLS, fmt.Errorf("GET %s: response exceeds %d bytes", endpoint, maxResponseBytes)
	}
	return data, response.TLS, nil
}

func untrustedTLSClient(endpoint string) (*http.Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	hostname := parsed.Hostname()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{ //nolint:gosec
		InsecureSkipVerify: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			return verifyUntrustedCertificate(state, hostname)
		},
	}
	return &http.Client{Timeout: 70 * time.Second, Transport: transport}, nil
}

func verifyUntrustedCertificate(state tls.ConnectionState, hostname string) error {
	if len(state.PeerCertificates) == 0 {
		return errors.New("TLS server did not provide a certificate")
	}
	last := len(state.PeerCertificates) - 1
	roots := x509.NewCertPool()
	roots.AddCert(state.PeerCertificates[last])
	intermediates := x509.NewCertPool()
	for index := 1; index < last; index++ {
		intermediates.AddCert(state.PeerCertificates[index])
	}
	_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{
		DNSName:       hostname,
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err
}

func untrustedTLSAlert(state *tls.ConnectionState) domain.InspectionAlert {
	alert := domain.InspectionAlert{
		Code:        privateCATLSAlertCode,
		Severity:    "info",
		Description: "Accepted a valid HTTPS certificate issued by an untrusted private CA for this inspection; this is not a process health finding.",
	}
	if state != nil && len(state.PeerCertificates) > 0 && isSelfSigned(state.PeerCertificates[0]) {
		alert.Code = selfSignedTLSAlertCode
		alert.Description = "Accepted a valid self-signed HTTPS certificate for this inspection; this is not a process health finding."
	}
	return alert
}

func isSelfSigned(certificate *x509.Certificate) bool {
	return bytes.Equal(certificate.RawSubject, certificate.RawIssuer) &&
		certificate.CheckSignature(certificate.SignatureAlgorithm, certificate.RawTBSCertificate, certificate.Signature) == nil
}

func isUnknownAuthority(err error) bool {
	var unknownAuthority x509.UnknownAuthorityError
	return errors.As(err, &unknownAuthority)
}

func targetEndpoint(target, endpoint string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(target))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("target must be an absolute HTTP(S) URL")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, fmt.Errorf("target scheme must be http or https")
	}
	if base.User != nil {
		return nil, fmt.Errorf("target must not contain user credentials")
	}
	if base.RawQuery != "" || base.Fragment != "" || base.RawFragment != "" {
		return nil, fmt.Errorf("target must not contain a query or fragment")
	}
	base.Path = path.Join(base.Path, endpoint)
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	base.RawFragment = ""
	return base, nil
}
