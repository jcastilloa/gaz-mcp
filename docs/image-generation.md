# Image generation

`gaz-mcp` generates images through OpenRouter's Image API. The server exposes no image tools until the `image_generation.models` configuration list contains at least one model. That list is a closed allowlist: consumers must select one listed model for each generation, which makes side-by-side model comparisons explicit and controlled.

## Configuration

```yaml
image_generation:
  api_key: "${OPENROUTER_API_KEY}"
  base_url: https://openrouter.ai/api/v1
  timeout: 2m
  models:
    - openai/gpt-image-1
    - google/gemini-2.5-flash-image
```

`api_key` is required whenever `models` is non-empty. It is masked in JSON output. `base_url` and `timeout` are optional; their defaults are `https://openrouter.ai/api/v1` and two minutes. Image generation is non-idempotent, so each MCP request makes one OpenRouter request and is never retried automatically.

## Tools

### `image_model_list`

Returns the configured model allowlist. Call it before a comparison workflow to obtain the exact model IDs available to this server.

### `image_generate`

Creates one or more images. Required parameters are `model` and `prompt`.

| Parameter | Description |
|---|---|
| `model` | Required. One exact model ID returned by `image_model_list`. |
| `prompt` | Required image description. |
| `input_references` | Optional seed images. Each item is an HTTP(S) `url`, or base64 `data` plus `media_type`. |
| `n` | 1–10 images; defaults to 1. Support for multiple images varies by model. |
| `size` | Resolution tier or exact pixels, such as `2K` or `2048x2048`. Cannot be mixed with `resolution` or `aspect_ratio`. |
| `resolution`, `aspect_ratio` | Normalized output dimensions, such as `2K` and `16:9`. |
| `quality`, `output_format`, `background`, `output_compression` | Output controls; actual availability depends on the selected model. |
| `seed` | Optional provider seed for reproducible output, where supported. |
| `provider` | Optional OpenRouter routing object with `only`, `order`, `ignore`, `sort`, `allow_fallbacks`, and provider-specific `options`. |

The result contains a structured metadata object followed by MCP `image` content blocks holding base64 image bytes and their real MIME type. The tool does not return temporary download URLs.

For each model comparison, issue one `image_generate` call per allowed model with equivalent prompt and options; model support and outputs differ, so unsupported provider-specific values can still be rejected by OpenRouter.
