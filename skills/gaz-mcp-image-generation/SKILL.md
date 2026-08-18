---
name: gaz-mcp-image-generation
description: Use the gaz-mcp OpenRouter image-generation proxy to list configured models, generate images from prompts and reference images, and compare outputs across models. Use when an MCP server exposes image_model_list or image_generate and the task requires creating, iterating, or evaluating images.
---

# gaz-mcp image generation

Generate images through the configured `gaz-mcp` MCP server. The server uses a closed model allowlist; never invent a model ID or assume that every OpenRouter image model is enabled.

## Start safely

1. Call `image_model_list` and use only the exact returned model IDs.
2. Confirm the requested model, prompt, dimensions, format, and number of outputs before expensive generations.
3. Call `image_generate` once per requested model. Generation is non-idempotent and may incur provider cost; do not retry automatically after an ambiguous timeout.
4. Return or inspect the MCP `image` content blocks. Structured metadata identifies the model, MIME type, creation time, and usage/cost when supplied.

## Tool contracts

### `image_model_list`

Returns the configured allowlist. Use it as the source of truth for model selection and comparisons.

### `image_generate`

Required arguments:

| Argument | Meaning |
|---|---|
| `model` | Exact ID from `image_model_list`. |
| `prompt` | Non-empty description of the desired image. |

Optional arguments:

| Argument | Meaning |
|---|---|
| `input_references` | Up to 10 seed images. Each item is either an HTTP(S) `url`, or base64 `data` plus an image `media_type` such as `image/png`. |
| `n` | 1–10 images; default 1. Provider support varies. |
| `size` | Exact pixels or a tier such as `2K`; do not combine with `resolution` or `aspect_ratio`. |
| `resolution`, `aspect_ratio` | Normalized dimensions such as `2K` and `16:9`; support varies by model. |
| `quality` | `auto`, `low`, `medium`, or `high`, when supported. |
| `output_format` | `png`, `jpeg`, `webp`, or `svg`, when supported. |
| `background` | `auto`, `transparent`, or `opaque`, when supported. |
| `output_compression` | 0–100 for JPEG/WebP where supported. |
| `seed` | Integer provider seed where supported. |
| `provider` | OpenRouter routing: `only`, `order`, `ignore`, `sort`, `allow_fallbacks`, and provider-specific `options`. |

Example request:

```text
image_generate(
  model="bytedance-seed/seedream-5-0-lite",
  prompt="Editorial photograph of a red bicycle beside a canal at sunrise",
  size="2048x2048",
  quality="high",
  n=1
)
```

Model limits are enforced by the provider. If rejected, preserve the prompt and adjust only the reported incompatible option; some models require a minimum pixel count or do not support a requested format.

## Reference images

Use a public HTTPS URL when the provider can fetch it. For local bytes, send valid standard base64 and an `image/*` MIME type:

```json
{"input_references":[{"data":"<base64>","media_type":"image/png"}]}
```

Do not place secrets in prompts, URLs, provider options, or reference data. Do not expose API keys or reproduce large base64 payloads in prose.

## Compare models

1. Call `image_model_list`.
2. Select two or more exact IDs.
3. Send equivalent `prompt`, references, dimensions, quality, and seed to one `image_generate` call per model.
4. Compare returned image blocks and report model IDs, option differences, provider errors, and usage/cost separately from subjective visual judgment.

If no models are configured, the image tools are not registered; report the configuration prerequisite instead of guessing a fallback model.
