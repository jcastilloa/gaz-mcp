package image

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "github.com/jcastillo/gaz-mcp/mcp/domain/image"
	configDomain "github.com/jcastillo/gaz-mcp/shared/config/domain"
)

func TestGenerateUsesOpenRouterImageAPIAndConvertsReferences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/images" {
			t.Fatalf("request = %s %s, want POST /api/v1/images", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != "openai/gpt-image-1" || body["prompt"] != "watercolor fox" || body["n"] != float64(1) {
			t.Fatalf("body = %#v", body)
		}
		references := body["input_references"].([]any)
		reference := references[0].(map[string]any)["image_url"].(map[string]any)["url"]
		if reference != "data:image/png;base64,aW1hZ2U=" {
			t.Fatalf("reference = %q", reference)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"created":123,"data":[{"b64_json":"aW1hZ2U=","media_type":"image/png"}],"usage":{"total_tokens":12,"cost":0.04}}`))
	}))
	defer server.Close()

	repository := NewRepository(configDomain.ImageGenerationConfig{APIKey: "secret", BaseURL: server.URL + "/api/v1", Timeout: time.Second})
	response, err := repository.Generate(context.Background(), domain.GenerateRequest{
		Model: "openai/gpt-image-1", Prompt: "watercolor fox", N: 1,
		InputReferences: []domain.InputReference{{Data: "aW1hZ2U=", MediaType: "image/png"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if response.Created != 123 || len(response.Data) != 1 || response.Data[0].MediaType != "image/png" || response.Usage.Cost != 0.04 {
		t.Fatalf("response = %#v", response)
	}
}

func TestGenerateReturnsOpenRouterErrorsWithoutLeakingCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"message":"unsupported quality"}}`))
	}))
	defer server.Close()

	repository := NewRepository(configDomain.ImageGenerationConfig{APIKey: "never-leak", BaseURL: server.URL})
	_, err := repository.Generate(context.Background(), domain.GenerateRequest{Model: "model", Prompt: "prompt", N: 1})
	if err == nil || strings.Contains(err.Error(), "never-leak") || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("Generate error = %v", err)
	}
}
