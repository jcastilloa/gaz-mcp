package image

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/image"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"
)

const (
	defaultBaseURL   = "https://openrouter.ai/api/v1"
	defaultTimeout   = 2 * time.Minute
	maxResponseBytes = 128 << 20
)

type Repository struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewRepository(cfg configDomain.ImageGenerationConfig) *Repository {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Repository{
		apiKey:     strings.TrimSpace(cfg.APIKey),
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (r *Repository) Generate(ctx context.Context, request domain.GenerateRequest) (domain.GenerateResponse, error) {
	body, err := json.Marshal(toAPIRequest(request))
	if err != nil {
		return domain.GenerateResponse{}, fmt.Errorf("marshal image generation request: %w", err)
	}
	return r.generateOnce(ctx, body)
}

func (r *Repository) generateOnce(ctx context.Context, body []byte) (domain.GenerateResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/images", bytes.NewReader(body))
	if err != nil {
		return domain.GenerateResponse{}, fmt.Errorf("create image generation request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+r.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := r.httpClient.Do(request)
	if err != nil {
		return domain.GenerateResponse{}, fmt.Errorf("send image generation request: %w", err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return domain.GenerateResponse{}, fmt.Errorf("read image generation response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return domain.GenerateResponse{}, fmt.Errorf("openrouter image generation: status %d: %s", response.StatusCode, responseError(data))
	}

	var payload apiResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return domain.GenerateResponse{}, fmt.Errorf("decode image generation response: %w", err)
	}
	return payload.toDomain()
}

func responseError(data []byte) string {
	message := strings.TrimSpace(string(data))
	if len(message) > 1024 {
		return message[:1024]
	}
	return message
}

type apiRequest struct {
	Model             string                 `json:"model"`
	Prompt            string                 `json:"prompt"`
	InputReferences   []apiInputReference    `json:"input_references,omitempty"`
	N                 int                    `json:"n"`
	Size              string                 `json:"size,omitempty"`
	Resolution        string                 `json:"resolution,omitempty"`
	AspectRatio       string                 `json:"aspect_ratio,omitempty"`
	Quality           string                 `json:"quality,omitempty"`
	OutputFormat      string                 `json:"output_format,omitempty"`
	Background        string                 `json:"background,omitempty"`
	OutputCompression *int                   `json:"output_compression,omitempty"`
	Seed              *int64                 `json:"seed,omitempty"`
	Provider          domain.ProviderRouting `json:"provider,omitempty"`
}

type apiInputReference struct {
	Type     string `json:"type"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

func toAPIRequest(request domain.GenerateRequest) apiRequest {
	payload := apiRequest{
		Model: request.Model, Prompt: request.Prompt, N: request.N,
		Size: request.Size, Resolution: request.Resolution, AspectRatio: request.AspectRatio,
		Quality: request.Quality, OutputFormat: request.OutputFormat, Background: request.Background,
		OutputCompression: request.OutputCompression, Seed: request.Seed, Provider: request.Provider,
	}
	for _, reference := range request.InputReferences {
		input := apiInputReference{Type: "image_url"}
		if reference.URL != "" {
			input.ImageURL.URL = reference.URL
		} else {
			input.ImageURL.URL = "data:" + reference.MediaType + ";base64," + reference.Data
		}
		payload.InputReferences = append(payload.InputReferences, input)
	}
	return payload
}

type apiResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		Data      string `json:"b64_json"`
		MediaType string `json:"media_type"`
	} `json:"data"`
	Usage domain.Usage `json:"usage"`
}

func (r apiResponse) toDomain() (domain.GenerateResponse, error) {
	result := domain.GenerateResponse{Created: r.Created, Usage: r.Usage, Data: make([]domain.GeneratedImage, 0, len(r.Data))}
	for index, image := range r.Data {
		if _, err := base64.StdEncoding.DecodeString(image.Data); err != nil {
			return domain.GenerateResponse{}, fmt.Errorf("decode generated image %d: %w", index+1, err)
		}
		if strings.TrimSpace(image.MediaType) == "" {
			return domain.GenerateResponse{}, fmt.Errorf("generated image %d has no media_type", index+1)
		}
		result.Data = append(result.Data, domain.GeneratedImage{Data: image.Data, MediaType: image.MediaType})
	}
	if len(result.Data) == 0 {
		return domain.GenerateResponse{}, errors.New("openrouter returned no generated images")
	}
	return result, nil
}
