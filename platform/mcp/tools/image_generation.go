package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	imageApp "github.com/jcastillo/gaz-mcp/mcp/application/image"
	domain "github.com/jcastillo/gaz-mcp/mcp/domain/image"
	"github.com/mark3labs/mcp-go/mcp"
)

type ImageGeneration struct {
	service imageApp.Service
}

type imageGenerateResult struct {
	Model   string                 `json:"model"`
	Created int64                  `json:"created,omitempty"`
	Images  []generatedImageResult `json:"images"`
	Usage   domain.Usage           `json:"usage,omitempty"`
}

type generatedImageResult struct {
	Index     int    `json:"index"`
	MediaType string `json:"media_type"`
}

func NewImageGeneration(service imageApp.Service) ImageGeneration {
	return ImageGeneration{service: service}
}

func (g ImageGeneration) ModelListDefinition() mcp.Tool {
	return mcp.NewTool("image_model_list",
		mcp.WithDescription("List the configured allowlist of OpenRouter image-generation models. Only these models may be used with image_generate."),
		readOnlyTool(),
	)
}

func (g ImageGeneration) ModelListHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultJSON(g.service.ListModels())
}

func (g ImageGeneration) GenerateDefinition() mcp.Tool {
	models := g.service.ListModels()
	modelIDs := make([]string, 0, len(models))
	for _, model := range models {
		modelIDs = append(modelIDs, model.ID)
	}

	return mcp.NewTool("image_generate",
		mcp.WithDescription("Generate one or more images through OpenRouter. model must be chosen from the configured allowlist. input_references accepts public HTTP(S) image URLs or base64 image data with its MIME type."),
		mcp.WithString("model", mcp.Required(), mcp.Enum(modelIDs...), mcp.Description("Configured image model to use.")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("Description of the desired image.")),
		mcp.WithArray("input_references", mcp.Description("Optional reference images. Each item has exactly one of url or data; data requires media_type."), mcp.MaxItems(10), mcp.Items(imageReferenceSchema())),
		mcp.WithNumber("n", mcp.Description("Number of images, from 1 to 10. Defaults to 1."), mcp.Min(1), mcp.Max(10), mcp.MultipleOf(1)),
		mcp.WithString("size", mcp.Description("Output size tier or explicit dimensions, for example 2K or 2048x2048. Cannot be combined with resolution or aspect_ratio.")),
		mcp.WithString("resolution", mcp.Description("Normalized resolution tier, for example 512, 1K, 2K, or 4K; model support varies.")),
		mcp.WithString("aspect_ratio", mcp.Description("Requested aspect ratio, for example 1:1, 16:9, or 9:16; model support varies.")),
		mcp.WithString("quality", mcp.Enum("auto", "low", "medium", "high"), mcp.Description("Requested quality; unsupported models may ignore it.")),
		mcp.WithString("output_format", mcp.Enum("png", "jpeg", "webp", "svg"), mcp.Description("Requested output format; model support varies.")),
		mcp.WithString("background", mcp.Enum("auto", "transparent", "opaque"), mcp.Description("Requested background; transparent output requires a compatible format and model.")),
		mcp.WithNumber("output_compression", mcp.Min(0), mcp.Max(100), mcp.MultipleOf(1), mcp.Description("Compression from 0 to 100; applies to JPEG or WebP when supported.")),
		mcp.WithNumber("seed", mcp.MultipleOf(1), mcp.Description("Optional provider seed for reproducible output when supported.")),
		mcp.WithObject("provider",
			mcp.Description("Optional OpenRouter routing options."),
			mcp.Properties(providerPropertiesSchema()),
			mcp.AdditionalProperties(false),
		),
		imageGenerationTool(),
	)
}

func (g ImageGeneration) GenerateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input domain.GenerateRequest
	if err := request.BindArguments(&input); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("image_generate: invalid arguments: %v", err)), nil
	}
	response, err := g.service.Generate(ctx, input)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("image_generate: %v", err)), nil
	}
	return imageToolResult(input.Model, response)
}

func imageReferenceSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":        map[string]any{"type": "string", "description": "Public HTTP(S) image URL."},
			"data":       map[string]any{"type": "string", "description": "Base64-encoded image bytes."},
			"media_type": map[string]any{"type": "string", "description": "MIME type for data, for example image/png."},
		},
		"additionalProperties": false,
	}
}

func providerPropertiesSchema() map[string]any {
	return map[string]any{
		"only":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"order":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"ignore":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"sort":            map[string]any{"type": "string", "enum": []string{"price", "throughput", "latency"}},
		"allow_fallbacks": map[string]any{"type": "boolean"},
		"options":         map[string]any{"type": "object", "description": "Provider-specific options allowed by OpenRouter for the selected model endpoint."},
	}
}

func imageGenerationTool() mcp.ToolOption {
	return mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    mcp.ToBoolPtr(false),
		DestructiveHint: mcp.ToBoolPtr(false),
		IdempotentHint:  mcp.ToBoolPtr(false),
		OpenWorldHint:   mcp.ToBoolPtr(true),
	})
}

func imageToolResult(model string, response domain.GenerateResponse) (*mcp.CallToolResult, error) {
	result := imageGenerateResult{
		Model: strings.TrimSpace(model), Created: response.Created, Usage: response.Usage,
		Images: make([]generatedImageResult, 0, len(response.Data)),
	}
	for index, image := range response.Data {
		result.Images = append(result.Images, generatedImageResult{Index: index + 1, MediaType: image.MediaType})
	}
	metadata, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal image generation result: %w", err)
	}
	mcpResult := &mcp.CallToolResult{
		Content:           []mcp.Content{mcp.NewTextContent(string(metadata))},
		StructuredContent: result,
	}
	for _, image := range response.Data {
		mcpResult.Content = append(mcpResult.Content, mcp.NewImageContent(image.Data, image.MediaType))
	}
	return mcpResult, nil
}
