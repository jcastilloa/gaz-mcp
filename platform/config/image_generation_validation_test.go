package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestValidateImageGenerationConfigRequiresAPIKeyWhenModelsAreEnabled(t *testing.T) {
	v := viper.New()
	v.Set("image_generation.models", []string{"openai/gpt-image-1"})

	err := validateImageGenerationConfig(v)
	if err == nil || !strings.Contains(err.Error(), "api_key is required") {
		t.Fatalf("validation error = %v", err)
	}
}

func TestValidateImageGenerationConfigAllowsAnEmptyModelList(t *testing.T) {
	if err := validateImageGenerationConfig(viper.New()); err != nil {
		t.Fatalf("validation error = %v", err)
	}
}
