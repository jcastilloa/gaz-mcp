package diagnostics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 128 << 20

type Repository struct {
	client *http.Client
}

func NewRepository() *Repository {
	return &Repository{client: &http.Client{Timeout: 70 * time.Second}}
}

func (r *Repository) FetchProfile(ctx context.Context, target, profileType string, seconds int) ([]byte, error) {
	endpointName := profileType
	if profileType == "cpu" {
		endpointName = "profile"
	}
	endpoint, err := targetEndpoint(target, path.Join("debug/pprof", endpointName))
	if err != nil {
		return nil, err
	}
	if profileType == "cpu" {
		query := endpoint.Query()
		query.Set("seconds", strconv.Itoa(seconds))
		endpoint.RawQuery = query.Encode()
	}
	return r.fetch(ctx, endpoint.String())
}

func (r *Repository) FetchGoroutines(ctx context.Context, target string) ([]byte, error) {
	endpoint, err := targetEndpoint(target, "debug/pprof/goroutine")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("debug", "2")
	endpoint.RawQuery = query.Encode()
	return r.fetch(ctx, endpoint.String())
}

func (r *Repository) FetchExpvars(ctx context.Context, target string) ([]byte, error) {
	endpoint, err := targetEndpoint(target, "debug/vars")
	if err != nil {
		return nil, err
	}
	return r.fetch(ctx, endpoint.String())
}

func (r *Repository) fetch(ctx context.Context, endpoint string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := r.client.Do(request)
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
