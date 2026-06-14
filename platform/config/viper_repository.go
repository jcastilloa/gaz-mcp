package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	aiDomain "github.com/jcastillo/gaz-mcp/shared/ai/domain"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type ViperRepository struct {
	v *viper.Viper
}

func New(serviceName string) (configDomain.Repository, error) {
	_ = godotenv.Load()

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	v.AddConfigPath(filepath.Join(home, ".config", serviceName))
	v.AddConfigPath(".")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		if !os.IsNotExist(err) && !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	return &ViperRepository{v: v}, nil
}

func (r *ViperRepository) OpenAIProviderConfig() aiDomain.ProviderConfig {
	return aiDomain.ProviderConfig{
		APIKey:             r.v.GetString("openai.api_key"),
		BaseURL:            r.v.GetString("openai.base_url"),
		Model:              r.v.GetString("openai.model"),
		ProviderName:       r.v.GetString("openai.provider_name"),
		Timeout:            r.v.GetDuration("openai.timeout"),
		MaxRetries:         r.v.GetInt("openai.max_retries"),
		SupportsSystemRole: r.v.GetBool("openai.supports_system_role"),
		SupportsJSONMode:   r.v.GetBool("openai.supports_json_mode"),
	}
}

func (r *ViperRepository) Environments() map[string]configDomain.EnvironmentConfig {
	raw := r.v.GetStringMap("environments")
	envs := make(map[string]configDomain.EnvironmentConfig, len(raw))
	for name, val := range raw {
		m, ok := val.(map[string]any)
		if !ok {
			continue
		}
		cfg := configDomain.EnvironmentConfig{
			Engine:   stringVal(m, "engine"),
			Host:     stringVal(m, "host"),
			Port:     intVal(m, "port"),
			User:     stringVal(m, "user"),
			Password: stringVal(m, "password"),
		}
		if cfg.Engine == "" {
			cfg.Engine = "mysql"
		}
		envs[name] = cfg
	}
	return envs
}

func (r *ViperRepository) ServiceConfig() configDomain.ServiceConfig {
	version := r.v.GetString("service.version")
	if version == "" {
		version = "0.1.0"
	}

	return configDomain.ServiceConfig{
		Host:      r.v.GetString("service.host"),
		Port:      r.v.GetInt("service.port"),
		APIPrefix: r.v.GetString("service.api_prefix"),
		Version:   version,
		Transport: r.v.GetString("service.transport"),
	}
}

func (r *ViperRepository) JenkinsEnvironments() map[string]configDomain.JenkinsEnvironmentConfig {
	raw := r.v.GetStringMap("jenkins")
	envs := make(map[string]configDomain.JenkinsEnvironmentConfig, len(raw))
	for name, val := range raw {
		m, ok := val.(map[string]any)
		if !ok {
			continue
		}
		cfg := configDomain.JenkinsEnvironmentConfig{
			URL:      stringVal(m, "url"),
			User:     stringVal(m, "user"),
			APIKey:   stringVal(m, "api_key"),
			Timeout:  durationVal(m, "timeout", 30*time.Second),
			Insecure: boolVal(m, "insecure"),
		}
		envs[name] = cfg
	}
	return envs
}

func (r *ViperRepository) SnapshotConfig() configDomain.SnapshotConfig {
	home, _ := os.UserHomeDir()
	defaultDBPath := filepath.Join(home, ".config", "gaz-mcp", "jenkins_history.db")

	return configDomain.SnapshotConfig{
		Enabled:     r.v.GetBool("snapshot.enabled"),
		DBPath:      stringOrDefault(r.v.GetString("snapshot.db_path"), defaultDBPath),
		MaxVersions: intOrDefault(r.v.GetInt("snapshot.max_versions"), 50),
		AutoPrune:   r.v.GetBool("snapshot.auto_prune"),
	}
}

func stringVal(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intVal(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func boolVal(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func durationVal(m map[string]any, key string, defaultVal time.Duration) time.Duration {
	if v, ok := m[key]; ok {
		switch d := v.(type) {
		case string:
			parsed, err := time.ParseDuration(d)
			if err == nil {
				return parsed
			}
		case int:
			return time.Duration(d) * time.Second
		case float64:
			return time.Duration(d) * time.Second
		}
	}
	return defaultVal
}

func stringOrDefault(val, defaultVal string) string {
	if val == "" {
		return defaultVal
	}
	return val
}

func intOrDefault(val, defaultVal int) int {
	if val <= 0 {
		return defaultVal
	}
	return val
}
