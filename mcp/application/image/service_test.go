package image_test

import (
	"context"
	"strings"
	"testing"

	imageApp "github.com/jcastillo/gaz-mcp/mcp/application/image"
	domain "github.com/jcastillo/gaz-mcp/mcp/domain/image"
)

func TestListModelsReturnsTheConfiguredAllowlist(t *testing.T) {
	service := imageApp.NewService(
		[]string{"openai/gpt-image-1", "google/gemini-2.5-flash-image", "openai/gpt-image-1", " "},
		&fakeRepository{},
	)

	models := service.ListModels()
	if len(models) != 2 {
		t.Fatalf("models = %#v, want exactly the two configured distinct models", models)
	}
	if models[0].ID != "openai/gpt-image-1" || models[1].ID != "google/gemini-2.5-flash-image" {
		t.Fatalf("models = %#v, want input order preserved", models)
	}
}

func TestGenerateRejectsModelsOutsideTheConfiguredAllowlist(t *testing.T) {
	repository := &fakeRepository{}
	service := imageApp.NewService([]string{"openai/gpt-image-1"}, repository)

	_, err := service.Generate(context.Background(), domain.GenerateRequest{
		Model:  "unapproved/model",
		Prompt: "A quiet forest",
	})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("Generate error = %v, want an allowlist error", err)
	}
	if repository.called {
		t.Fatal("repository was called for an unapproved model")
	}
}

func TestGenerateNormalizesAndForwardsSupportedOptions(t *testing.T) {
	repository := fakeRepository{response: domain.GenerateResponse{Data: []domain.GeneratedImage{{Data: "aW1hZ2U=", MediaType: "image/png"}}}}
	service := imageApp.NewService([]string{"openai/gpt-image-1"}, &repository)
	compression := 85
	seed := int64(42)
	fallbacks := false

	response, err := service.Generate(context.Background(), domain.GenerateRequest{
		Model:             " openai/gpt-image-1 ",
		Prompt:            "  a watercolor fox  ",
		N:                 0,
		Resolution:        "2K",
		AspectRatio:       "16:9",
		Quality:           "high",
		OutputFormat:      "png",
		OutputCompression: &compression,
		Seed:              &seed,
		InputReferences: []domain.InputReference{{
			Data:      "aW1hZ2U=",
			MediaType: "image/png",
		}},
		Provider: domain.ProviderRouting{Only: []string{"openai"}, AllowFallbacks: &fallbacks},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(response.Data) != 1 {
		t.Fatalf("response = %#v, want generated image", response)
	}
	if !repository.called {
		t.Fatal("repository was not called")
	}
	if repository.request.Model != "openai/gpt-image-1" || repository.request.Prompt != "a watercolor fox" {
		t.Fatalf("request = %#v, want trimmed model and prompt", repository.request)
	}
	if repository.request.N != 1 {
		t.Fatalf("request.N = %d, want default 1", repository.request.N)
	}
}

func TestGenerateRejectsInvalidRequestsBeforeCallingTheRepository(t *testing.T) {
	tests := []struct {
		name     string
		request  domain.GenerateRequest
		contains string
	}{
		{name: "missing prompt", request: domain.GenerateRequest{Model: "openai/gpt-image-1"}, contains: "prompt is required"},
		{name: "too many images", request: domain.GenerateRequest{Model: "openai/gpt-image-1", Prompt: "x", N: 11}, contains: "n must be between 1 and 10"},
		{name: "conflicting sizing", request: domain.GenerateRequest{Model: "openai/gpt-image-1", Prompt: "x", Size: "1024x1024", Resolution: "2K"}, contains: "size cannot be combined"},
		{name: "bad compression", request: domain.GenerateRequest{Model: "openai/gpt-image-1", Prompt: "x", OutputCompression: intPtr(101)}, contains: "output_compression must be between 0 and 100"},
		{name: "bad reference", request: domain.GenerateRequest{Model: "openai/gpt-image-1", Prompt: "x", InputReferences: []domain.InputReference{{URL: "file:///secret.png"}}}, contains: "input reference"},
		{name: "invalid base64 reference", request: domain.GenerateRequest{Model: "openai/gpt-image-1", Prompt: "x", InputReferences: []domain.InputReference{{Data: "not base64", MediaType: "image/png"}}}, contains: "must be valid base64"},
		{name: "non-image reference", request: domain.GenerateRequest{Model: "openai/gpt-image-1", Prompt: "x", InputReferences: []domain.InputReference{{Data: "aW1hZ2U=", MediaType: "text/plain"}}}, contains: "must be an image MIME type"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeRepository{}
			service := imageApp.NewService([]string{"openai/gpt-image-1"}, repository)
			_, err := service.Generate(context.Background(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Generate error = %v, want %q", err, test.contains)
			}
			if repository.called {
				t.Fatal("repository was called for an invalid request")
			}
		})
	}
}

func intPtr(value int) *int { return &value }

type fakeRepository struct {
	called   bool
	request  domain.GenerateRequest
	response domain.GenerateResponse
	err      error
}

func (r *fakeRepository) Generate(_ context.Context, request domain.GenerateRequest) (domain.GenerateResponse, error) {
	r.called = true
	r.request = request
	return r.response, r.err
}
