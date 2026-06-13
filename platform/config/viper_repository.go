package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
