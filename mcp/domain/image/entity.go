package image

type Model struct {
	ID string `json:"id"`
}

type InputReference struct {
	URL       string `json:"url,omitempty"`
	Data      string `json:"data,omitempty"`
	MediaType string `json:"media_type,omitempty"`
}

type ProviderRouting struct {
	Only           []string       `json:"only,omitempty"`
	Order          []string       `json:"order,omitempty"`
	Ignore         []string       `json:"ignore,omitempty"`
	Sort           string         `json:"sort,omitempty"`
	AllowFallbacks *bool          `json:"allow_fallbacks,omitempty"`
	Options        map[string]any `json:"options,omitempty"`
}

type GenerateRequest struct {
	Model             string           `json:"model"`
	Prompt            string           `json:"prompt"`
	InputReferences   []InputReference `json:"input_references,omitempty"`
	N                 int              `json:"n,omitempty"`
	Size              string           `json:"size,omitempty"`
	Resolution        string           `json:"resolution,omitempty"`
	AspectRatio       string           `json:"aspect_ratio,omitempty"`
	Quality           string           `json:"quality,omitempty"`
	OutputFormat      string           `json:"output_format,omitempty"`
	Background        string           `json:"background,omitempty"`
	OutputCompression *int             `json:"output_compression,omitempty"`
	Seed              *int64           `json:"seed,omitempty"`
	Provider          ProviderRouting  `json:"provider,omitempty"`
}

type GeneratedImage struct {
	Data      string `json:"-"`
	MediaType string `json:"media_type"`
}

type Usage struct {
	PromptTokens     int64   `json:"prompt_tokens,omitempty"`
	CompletionTokens int64   `json:"completion_tokens,omitempty"`
	TotalTokens      int64   `json:"total_tokens,omitempty"`
	Cost             float64 `json:"cost,omitempty"`
}

type GenerateResponse struct {
	Created int64            `json:"created,omitempty"`
	Data    []GeneratedImage `json:"data"`
	Usage   Usage            `json:"usage,omitempty"`
}
