package tools

import (
	"context"
	"testing"

	imageApp "github.com/jcastillo/gaz-mcp/mcp/application/image"
	domain "github.com/jcastillo/gaz-mcp/mcp/domain/image"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestImageModelListReturnsOnlyTheConfiguredAllowlist(t *testing.T) {
	tool := NewImageGeneration(imageApp.NewService([]string{"openai/gpt-image-1", "google/gemini-2.5-flash-image"}, &imageRepository{}))
	result, err := tool.ModelListHandler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("ModelListHandler: %v", err)
	}
	models, ok := result.StructuredContent.([]domain.Model)
	if !ok || len(models) != 2 || models[1].ID != "google/gemini-2.5-flash-image" {
		t.Fatalf("models = %#v", result.StructuredContent)
	}
	if definition := tool.ModelListDefinition(); definition.Annotations.ReadOnlyHint == nil || !*definition.Annotations.ReadOnlyHint {
		t.Fatalf("model list annotations = %#v, want read only", definition.Annotations)
	}
}

func TestImageGenerateDefinitionHasValidProviderProperties(t *testing.T) {
	tool := NewImageGeneration(imageApp.NewService([]string{"openai/gpt-image-1"}, &imageRepository{}))
	definition := tool.GenerateDefinition()

	provider, ok := definition.InputSchema.Properties["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider schema = %#v, want object schema", definition.InputSchema.Properties["provider"])
	}
	properties, ok := provider["properties"].(map[string]any)
	if !ok {
		t.Fatalf("provider properties = %#v, want property map", provider["properties"])
	}

	for _, name := range []string{"only", "order", "ignore", "sort", "allow_fallbacks", "options"} {
		if _, ok := properties[name]; !ok {
			t.Errorf("provider properties missing %q: %#v", name, properties)
		}
	}
	for name, schema := range properties {
		switch schema.(type) {
		case map[string]any, bool:
		default:
			t.Errorf("provider property %q has invalid schema type %T", name, schema)
		}
	}

	additionalProperties, ok := provider["additionalProperties"].(bool)
	if !ok || additionalProperties {
		t.Errorf("provider additionalProperties = %#v, want false", provider["additionalProperties"])
	}
}

func TestImageGenerateReturnsMCPImageContent(t *testing.T) {
	repository := &imageRepository{response: domain.GenerateResponse{
		Created: 123,
		Data:    []domain.GeneratedImage{{Data: "aW1hZ2U=", MediaType: "image/png"}},
	}}
	tool := NewImageGeneration(imageApp.NewService([]string{"openai/gpt-image-1"}, repository))
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"model": "openai/gpt-image-1", "prompt": "A watercolor fox", "n": 1,
	}}}

	result, err := tool.GenerateHandler(context.Background(), request)
	if err != nil || result.IsError {
		t.Fatalf("GenerateHandler = %#v, %v", result, err)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %#v, want metadata and an image", result.Content)
	}
	image, ok := result.Content[1].(mcp.ImageContent)
	if !ok || image.Data != "aW1hZ2U=" || image.MIMEType != "image/png" {
		t.Fatalf("image content = %#v", result.Content[1])
	}
	if repository.request.Model != "openai/gpt-image-1" {
		t.Fatalf("repository request = %#v", repository.request)
	}
	definition := tool.GenerateDefinition()
	if definition.Annotations.ReadOnlyHint == nil || *definition.Annotations.ReadOnlyHint || definition.Annotations.IdempotentHint == nil || *definition.Annotations.IdempotentHint {
		t.Fatalf("generate annotations = %#v, want non-readonly and non-idempotent", definition.Annotations)
	}
}

func TestImageGenerateRejectsUnconfiguredModelBeforeCallingProvider(t *testing.T) {
	repository := &imageRepository{}
	tool := NewImageGeneration(imageApp.NewService([]string{"openai/gpt-image-1"}, repository))
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"model": "unconfigured/model", "prompt": "A watercolor fox",
	}}}

	result, err := tool.GenerateHandler(context.Background(), request)
	if err != nil || !result.IsError || repository.called {
		t.Fatalf("GenerateHandler = %#v, %v; called=%v", result, err, repository.called)
	}
}

type imageRepository struct {
	called   bool
	request  domain.GenerateRequest
	response domain.GenerateResponse
}

func (r *imageRepository) Generate(_ context.Context, request domain.GenerateRequest) (domain.GenerateResponse, error) {
	r.called = true
	r.request = request
	return r.response, nil
}
