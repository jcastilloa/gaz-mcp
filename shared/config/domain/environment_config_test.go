package domain

import (
	"net/url"
	"testing"
)

func TestEnvironmentConfigPostgresDSNUsesConnectionValues(t *testing.T) {
	cfg := EnvironmentConfig{
		Engine:   "postgresql",
		Host:     "db.example.test",
		Port:     5432,
		User:     "repit user",
		Password: "p@ss:/word?x=1",
	}

	parsed, err := url.Parse(cfg.PostgresDSN("repit"))
	if err != nil {
		t.Fatalf("parse postgres DSN: %v", err)
	}
	if parsed.Scheme != "postgres" {
		t.Errorf("scheme = %q, want postgres", parsed.Scheme)
	}
	if got := parsed.User.Username(); got != cfg.User {
		t.Errorf("username = %q, want %q", got, cfg.User)
	}
	password, ok := parsed.User.Password()
	if !ok || password != cfg.Password {
		t.Errorf("password = %q, present = %t; want configured password", password, ok)
	}
	if parsed.Hostname() != cfg.Host {
		t.Errorf("hostname = %q, want %q", parsed.Hostname(), cfg.Host)
	}
	if parsed.Port() != "5432" {
		t.Errorf("port = %q, want 5432", parsed.Port())
	}
	if parsed.Path != "/repit" {
		t.Errorf("path = %q, want /repit", parsed.Path)
	}
	if got := parsed.Query().Get("sslmode"); got != "disable" {
		t.Errorf("sslmode = %q, want disable", got)
	}
}

func TestEnvironmentConfigIsPostgresRecognizesAliases(t *testing.T) {
	for _, engine := range []string{"postgres", "postgresql", " PostgreSQL "} {
		t.Run(engine, func(t *testing.T) {
			if !((EnvironmentConfig{Engine: engine}).IsPostgres()) {
				t.Errorf("IsPostgres() = false for engine %q", engine)
			}
		})
	}
	if (EnvironmentConfig{Engine: "mysql"}).IsPostgres() {
		t.Error("IsPostgres() = true for mysql")
	}
}

func TestEnvironmentConfigDSNRecognizesPostgresqlEngine(t *testing.T) {
	cfg := EnvironmentConfig{Engine: "postgresql", User: "repit_user"}
	parsed, err := url.Parse(cfg.DSN())
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	if parsed.Scheme != "postgres" {
		t.Fatalf("scheme = %q, want postgres for engine %q", parsed.Scheme, cfg.Engine)
	}
	if parsed.Hostname() != "127.0.0.1" || parsed.Port() != "5432" {
		t.Errorf("address = %q, want 127.0.0.1:5432", parsed.Host)
	}
}
