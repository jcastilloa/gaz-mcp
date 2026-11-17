package domain

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type EnvironmentConfig struct {
	Engine   string
	Host     string
	Port     int
	User     string
	Password string
}

func (c EnvironmentConfig) IsPostgres() bool {
	engine := strings.ToLower(strings.TrimSpace(c.Engine))
	return engine == "postgres" || engine == "postgresql"
}

func (c EnvironmentConfig) DSN() string {
	port := c.Port
	if port <= 0 {
		if c.IsPostgres() {
			port = 5432
		} else {
			port = 3306
		}
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	if c.IsPostgres() {
		connectionURL := url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(c.User, "***"),
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
			Path:   "/",
		}
		query := connectionURL.Query()
		query.Set("sslmode", "disable")
		connectionURL.RawQuery = query.Encode()
		return connectionURL.String()
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/?parseTime=true", c.User, c.Password, host, port)
}

func (c EnvironmentConfig) PostgresDSN(database string) string {
	port := c.Port
	if port <= 0 {
		port = 5432
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	connectionURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/" + database,
	}
	query := connectionURL.Query()
	query.Set("sslmode", "disable")
	connectionURL.RawQuery = query.Encode()
	return connectionURL.String()
}

// JenkinsEnvironmentConfig holds connection details for a Jenkins instance.
type JenkinsEnvironmentConfig struct {
	URL      string        // Jenkins base URL, e.g. https://jenkins.example.com
	User     string        // Jenkins username for API auth
	APIKey   string        // API key or password — hidden in logs and JSON output
	Timeout  time.Duration // HTTP client timeout
	Insecure bool          // Allow self-signed TLS certificates
}

// MarshalJSON masks the APIKey for safe logging.
func (c JenkinsEnvironmentConfig) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(
		`{"url":%q,"user":%q,"api_key":"****","timeout":%q,"insecure":%v}`,
		c.URL, c.User, c.Timeout.String(), c.Insecure,
	)), nil
}

// SnapshotConfig holds settings for the automatic backup/versioning system.
type SnapshotConfig struct {
	Enabled     bool   // Enable snapshot capture on writes
	DBPath      string // SQLite database file path
	MaxVersions int    // Max versions to keep per object (0 = unlimited)
	AutoPrune   bool   // Automatically prune old versions after each write
}
