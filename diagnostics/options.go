package diagnostics

import (
	"database/sql"
	"expvar"
	"fmt"
	"net/http"
	"net/netip"
	"path"
	"strings"
)

// Middleware wraps all diagnostics endpoints.
type Middleware func(http.Handler) http.Handler

// Option configures endpoint registration.
type Option func(*config) error

type config struct {
	basePath    string
	dbPools     []dbPool
	customVars  []func(*expvar.Map)
	allowlist   []netip.Prefix
	middlewares []Middleware
}

type dbPool struct {
	name string
	db   *sql.DB
}

// WithBasePath mounts endpoints below an absolute path. The default is the
// root path, which exposes /debug/pprof/ and /debug/vars.
func WithBasePath(basePath string) Option {
	return func(config *config) error {
		normalized, err := normalizeBasePath(basePath)
		if err != nil {
			return err
		}
		config.basePath = normalized
		return nil
	}
}

// WithDBPool publishes db.Stats under gaz_diagnostics.db_pools[name].
func WithDBPool(name string, db *sql.DB) Option {
	return func(config *config) error {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("diagnostics: database pool name cannot be empty")
		}
		if db == nil {
			return fmt.Errorf("diagnostics: database pool %q is nil", name)
		}
		config.dbPools = append(config.dbPools, dbPool{name: name, db: db})
		return nil
	}
}

// WithCustomVars adds application metrics below gaz_diagnostics.custom.
func WithCustomVars(register func(*expvar.Map)) Option {
	return func(config *config) error {
		if register == nil {
			return fmt.Errorf("diagnostics: custom variable hook cannot be nil")
		}
		config.customVars = append(config.customVars, register)
		return nil
	}
}

// WithIPAllowlist permits only the listed client IPs or CIDR ranges.
func WithIPAllowlist(entries ...string) Option {
	return func(config *config) error {
		if len(entries) == 0 {
			return fmt.Errorf("diagnostics: allowlist cannot be empty")
		}
		for _, entry := range entries {
			prefix, err := parseAllowlistEntry(entry)
			if err != nil {
				return err
			}
			config.allowlist = append(config.allowlist, prefix)
		}
		return nil
	}
}

// WithMiddleware adds a handler wrapper to every diagnostics endpoint.
func WithMiddleware(middleware Middleware) Option {
	return func(config *config) error {
		if middleware == nil {
			return fmt.Errorf("diagnostics: middleware cannot be nil")
		}
		config.middlewares = append(config.middlewares, middleware)
		return nil
	}
}

func normalizeBasePath(basePath string) (string, error) {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" || basePath == "/" {
		return "", nil
	}
	if !strings.HasPrefix(basePath, "/") {
		return "", fmt.Errorf("diagnostics: base path %q must be absolute", basePath)
	}
	if strings.ContainsAny(basePath, "?#{}") {
		return "", fmt.Errorf("diagnostics: base path %q cannot contain query, fragment, or wildcard delimiters", basePath)
	}
	return strings.TrimSuffix(path.Clean(basePath), "/"), nil
}

func parseAllowlistEntry(entry string) (netip.Prefix, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return netip.Prefix{}, fmt.Errorf("diagnostics: allowlist entry cannot be empty")
	}
	prefix, err := netip.ParsePrefix(entry)
	if err == nil {
		return prefix.Masked(), nil
	}
	address, addressErr := netip.ParseAddr(entry)
	if addressErr != nil {
		return netip.Prefix{}, fmt.Errorf("diagnostics: invalid allowlist entry %q", entry)
	}
	return netip.PrefixFrom(address, address.BitLen()), nil
}
