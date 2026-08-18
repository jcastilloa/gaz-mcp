package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestImageGenerationConfigReadsClosedModelAllowlist(t *testing.T) {
	v := viper.New()
	v.Set("image_generation.api_key", "secret-key")
	v.Set("image_generation.base_url", "https://openrouter.example/api/v1")
	v.Set("image_generation.timeout", "2m")
	v.Set("image_generation.models", []string{"openai/gpt-image-1", "google/gemini-2.5-flash-image"})

	cfg := (&ViperRepository{v: v}).ImageGenerationConfig()
	if cfg.APIKey != "secret-key" || cfg.BaseURL != "https://openrouter.example/api/v1" {
		t.Fatalf("config = %#v, want configured API values", cfg)
	}
	if cfg.Timeout != 2*time.Minute {
		t.Fatalf("config = %#v, want configured timeout", cfg)
	}
	if len(cfg.Models) != 2 || cfg.Models[1] != "google/gemini-2.5-flash-image" {
		t.Fatalf("models = %#v, want configured closed allowlist", cfg.Models)
	}
}

func TestImageGenerationConfigExpandsAPIKeyEnvironmentVariableAndNeverMarshalsIt(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "secret-key")
	v := viper.New()
	v.Set("image_generation.api_key", "${OPENROUTER_API_KEY}")

	cfg := (&ViperRepository{v: v}).ImageGenerationConfig()
	if cfg.APIKey != os.Getenv("OPENROUTER_API_KEY") {
		t.Fatalf("APIKey = %q, want expanded environment value", cfg.APIKey)
	}
	serialized, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if strings.Contains(string(serialized), "secret-key") {
		t.Fatalf("serialized config leaks API key: %s", serialized)
	}
}
