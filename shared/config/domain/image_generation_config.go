package domain

import (
	"encoding/json"
	"time"
)

type ImageGenerationConfig struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
	Models  []string
}

func (c ImageGenerationConfig) MarshalJSON() ([]byte, error) {
	type safeConfig struct {
		APIKey  string        `json:"api_key"`
		BaseURL string        `json:"base_url"`
		Timeout time.Duration `json:"timeout"`
		Models  []string      `json:"models"`
	}
	return json.Marshal(safeConfig{
		APIKey:  "****",
		BaseURL: c.BaseURL,
		Timeout: c.Timeout,
		Models:  c.Models,
	})
}
