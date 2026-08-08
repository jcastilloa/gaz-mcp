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

const selfSignedTLSAlertCode = "TLS_SELF_SIGNED_CERTIFICATE"

type Repository struct {
	client           *http.Client
	selfSignedClient *http.Client
}

func NewRepository() *Repository {
	return &Repository{
		client:           &http.Client{Timeout: 70 * time.Second},
		selfSignedClient: &http.Client{Timeout: 70 * time.Second, Transport: selfSignedTransport()},
	}
}

func (r *Repository) FetchProfile(ctx context.Context, target, profileType string, seconds int) (domain.FetchResult, error) {
	endpointName := profileType
	if profileType == "cpu" {
		endpointName = "profile"
	}
	endpoint, err := targetEndpoint(target, path.Join("debug/pprof", endpointName))
	if err != nil {
		return domain.FetchResult{}, err
	}
	if profileType == "cpu" {
		query := endpoint.Query()
		query.Set("seconds", strconv.Itoa(seconds))
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
	data, err := r.fetchWithClient(ctx, r.client, endpoint)
	if err == nil || !isUnknownAuthority(err) {
		return domain.FetchResult{Data: data}, err
	}

	data, err = r.fetchWithClient(ctx, r.selfSignedClient, endpoint)
	if err != nil {
		return domain.FetchResult{}, err
	}
	return domain.FetchResult{
		Data: data,
		Alerts: []domain.InspectionAlert{{
			Code:        selfSignedTLSAlertCode,
			Severity:    "info",
			Description: "Accepted a valid self-signed HTTPS certificate for this inspection; this is not a process health finding.",
		}},
	}, nil
}

func (r *Repository) fetchWithClient(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("GET %s: unexpected HTTP status %s", endpoint, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("GET %s: response exceeds %d bytes", endpoint, maxResponseBytes)
	}
	return data, nil
}

func selfSignedTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{ //nolint:gosec
		InsecureSkipVerify: true,
		VerifyConnection:   verifySelfSignedCertificate,
	}
	return transport
}

func verifySelfSignedCertificate(state tls.ConnectionState) error {
	if len(state.PeerCertificates) == 0 {
		return errors.New("TLS server did not provide a certificate")
	}
	certificate := state.PeerCertificates[0]
	if !isSelfSigned(certificate) {
		return x509.UnknownAuthorityError{Cert: certificate}
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	_, err := certificate.Verify(x509.VerifyOptions{
		DNSName:       state.ServerName,
		Roots:         roots,
		Intermediates: certificatePool(state.PeerCertificates[1:]),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err
}

func isSelfSigned(certificate *x509.Certificate) bool {
	return bytes.Equal(certificate.RawSubject, certificate.RawIssuer) && certificate.CheckSignatureFrom(certificate) == nil
}

func certificatePool(certificates []*x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, certificate := range certificates {
		pool.AddCert(certificate)
	}
	return pool
}

func isUnknownAuthority(err error) bool {
	var verificationError *tls.CertificateVerificationError
	if errors.As(err, &verificationError) {
		return true
	}
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
	base.Path = path.Join(base.Path, endpoint)
	base.RawQuery = ""
	return base, nil
}
