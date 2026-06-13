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

func (r *ViperRepository) MySQLConfig() configDomain.MySQLConfig {
	return configDomain.MySQLConfig{
		Host:     r.v.GetString("mysql.host"),
		Port:     r.v.GetInt("mysql.port"),
		User:     r.v.GetString("mysql.user"),
		Password: r.v.GetString("mysql.password"),
	}
}

func (r *ViperRepository) PostgresConfig() configDomain.PostgresConfig {
	return configDomain.PostgresConfig{
		Host:     r.v.GetString("postgres.host"),
		Port:     r.v.GetInt("postgres.port"),
		User:     r.v.GetString("postgres.user"),
		Password: r.v.GetString("postgres.password"),
	}
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
