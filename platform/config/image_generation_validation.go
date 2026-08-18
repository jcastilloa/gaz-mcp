package config

import (
	"errors"
	"strings"

	"github.com/spf13/viper"
)

func validateImageGenerationConfig(v *viper.Viper) error {
	models := v.GetStringSlice("image_generation.models")
	if len(models) == 0 {
		return nil
	}
	if strings.TrimSpace(expandEnvironment(v.GetString("image_generation.api_key"))) == "" {
		return errors.New("api_key is required when models are configured")
	}
	return nil
}
