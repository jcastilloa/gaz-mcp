package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	openaiLib "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	aiDomain "github.com/jcastillo/gaz-mcp/shared/ai/domain"
)

const (
	defaultModel      = "gpt-4o-mini"
	defaultProvider   = "openai-compatible"
	defaultTimeout    = 60 * time.Second
	defaultMaxRetries = 4
	defaultBaseDelay  = 750 * time.Millisecond
	maxBackoffDelay   = 8 * time.Second
)

type Logger interface {
	Debugf(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

type noopLogger struct{}

func (l noopLogger) Debugf(string, ...any) {}
func (l noopLogger) Warnf(string, ...any)  {}
func (l noopLogger) Errorf(string, ...any) {}

type Repository struct {
	client *openaiLib.Client
	config aiDomain.ProviderConfig
	logger Logger
	rng    *rand.Rand
}

func NewOpenAIRepository(cfg aiDomain.ProviderConfig, logger Logger) *Repository {
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if cfg.ProviderName == "" {
		cfg.ProviderName = defaultProvider
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultMaxRetries
	}

	// Default optimista para proveedores compatibles modernos.
	// Si uno falla con system role, lo desactivas en config.
	if !cfg.SupportsSystemRole {
		// dejamos false solo si se configuró así explícitamente
	} else {
		cfg.SupportsSystemRole = true
	}

	options := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}

	if cfg.BaseURL != "" {
		options = append(options, option.WithBaseURL(cfg.BaseURL))
	}

	client := openaiLib.NewClient(options...)

	if logger == nil {
		logger = noopLogger{}
	}

	return &Repository{
		client: &client,
		config: cfg,
		logger: logger,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *Repository) GetAIResponse(ctx context.Context, req *aiDomain.Request) (*aiDomain.Response, error) {
	if req == nil {
		return aiDomain.NewResponse("", nil, "", "", r.config.ProviderName, nil, true, "nil request")
	}

	if strings.TrimSpace(req.UserPrompt) == "" {
		return aiDomain.NewResponse("", nil, "", "", r.config.ProviderName, nil, true, "user prompt is required")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	resp, err := r.executeWithRetry(timeoutCtx, req)
	if err != nil {
		return aiDomain.NewResponse("", nil, "", resolveModel(req, r.config.Model), r.config.ProviderName, nil, true, err.Error())
	}

	var outputMap map[string]interface{}
	if req.ResponseMode == aiDomain.ResponseModeJSON {
		if jsonErr := json.Unmarshal([]byte(resp.OutputText), &outputMap); jsonErr != nil {
			return aiDomain.NewResponse(
				resp.OutputText,
				nil,
				resp.FinishReason,
				resp.Model,
				resp.Provider,
				resp.Usage,
				true,
				fmt.Sprintf("model returned non-json content in JSON mode: %v", jsonErr),
			)
		}
	}

	return aiDomain.NewResponse(
		resp.OutputText,
		outputMap,
		resp.FinishReason,
		resp.Model,
		resp.Provider,
		resp.Usage,
		false,
		"",
	)
}

type internalResponse struct {
	OutputText   string
	FinishReason string
	Model        string
	Provider     string
	Usage        *aiDomain.Usage
}

func (r *Repository) executeWithRetry(ctx context.Context, req *aiDomain.Request) (*internalResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := r.nextBackoff(attempt)
			r.logger.Warnf("retrying provider call in %v (attempt %d/%d)", delay, attempt+1, r.config.MaxRetries+1)

			if err := sleepWithContext(ctx, delay); err != nil {
				return nil, err
			}
		}

		resp, err := r.executeOnce(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		if !isRetryableError(err) || attempt == r.config.MaxRetries {
			break
		}
	}

	if lastErr == nil {
		lastErr = errors.New("unknown provider error")
	}

	return nil, fmt.Errorf("provider request failed: %w", lastErr)
}

func (r *Repository) executeOnce(ctx context.Context, req *aiDomain.Request) (*internalResponse, error) {
	messages := r.buildMessages(req.SystemPrompt, req.UserPrompt)

	params := openaiLib.ChatCompletionNewParams{
		Model:    openaiLib.ChatModel(resolveModel(req, r.config.Model)),
		Messages: messages,
	}

	if temperature := resolveTemperature(req, r.config.DefaultTemperature); temperature != nil {
		params.Temperature = openaiLib.Float(*temperature)
	}

	// Muchos compatibles soportan este campo; si tu versión concreta del SDK no lo expone,
	// elimina este bloque y deja MaxTokens en el dominio para evolución futura.
	if req.MaxTokens != nil {
		params.MaxTokens = openaiLib.Int(*req.MaxTokens)
	}

	if req.TopP != nil {
		params.TopP = openaiLib.Float(*req.TopP)
	}

	if len(req.Stop) == 1 {
		params.Stop = openaiLib.ChatCompletionNewParamsStopUnion{
			OfString: openaiLib.String(req.Stop[0]),
		}
	} else if len(req.Stop) > 1 {
		params.Stop = openaiLib.ChatCompletionNewParamsStopUnion{
			OfStringArray: req.Stop,
		}
	}

	// Intento de “JSON mode” solo si el proveedor lo soporta.
	if req.ResponseMode == aiDomain.ResponseModeJSON && r.config.SupportsJSONMode {
		params.ResponseFormat = openaiLib.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openaiLib.ResponseFormatJSONObjectParam{
				Type: "json_object",
			},
		}
	}

	response, err := r.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, classifyError(err)
	}

	if len(response.Choices) == 0 {
		return nil, errors.New("empty choices in provider response")
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return nil, errors.New("empty content in provider response")
	}

	var finishReason string
	if response.Choices[0].FinishReason != "" {
		finishReason = string(response.Choices[0].FinishReason)
	}

	var usage *aiDomain.Usage
	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 || response.Usage.TotalTokens > 0 {
		usage = &aiDomain.Usage{
			PromptTokens:     int64(response.Usage.PromptTokens),
			CompletionTokens: int64(response.Usage.CompletionTokens),
			TotalTokens:      int64(response.Usage.TotalTokens),
		}
	}

	return &internalResponse{
		OutputText:   content,
		FinishReason: finishReason,
		Model:        resolveModel(req, r.config.Model),
		Provider:     r.config.ProviderName,
		Usage:        usage,
	}, nil
}

func (r *Repository) buildMessages(systemPrompt, userPrompt string) []openaiLib.ChatCompletionMessageParamUnion {
	systemPrompt = strings.TrimSpace(systemPrompt)
	userPrompt = strings.TrimSpace(userPrompt)

	if r.config.SupportsSystemRole {
		messages := make([]openaiLib.ChatCompletionMessageParamUnion, 0, 2)
		if systemPrompt != "" {
			messages = append(messages, openaiLib.SystemMessage(systemPrompt))
		}
		messages = append(messages, openaiLib.UserMessage(userPrompt))
		return messages
	}

	// Fallback para proveedores compatibles que no manejan bien el role "system".
	if systemPrompt != "" {
		userPrompt = "Instructions:\n" + systemPrompt + "\n\nUser request:\n" + userPrompt
	}

	return []openaiLib.ChatCompletionMessageParamUnion{
		openaiLib.UserMessage(userPrompt),
	}
}

func resolveModel(req *aiDomain.Request, fallback string) string {
	if req != nil && strings.TrimSpace(req.Model) != "" {
		return strings.TrimSpace(req.Model)
	}
	if strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback)
	}
	return defaultModel
}

func resolveTemperature(req *aiDomain.Request, fallback *float64) *float64 {
	if req != nil && req.Temperature != nil {
		return req.Temperature
	}
	return fallback
}

func (r *Repository) nextBackoff(attempt int) time.Duration {
	// exponential backoff con jitter
	base := defaultBaseDelay * time.Duration(1<<(attempt-1))
	if base > maxBackoffDelay {
		base = maxBackoffDelay
	}

	// jitter entre 50% y 100% del delay calculado
	jitterFactor := 0.5 + r.rng.Float64()*0.5
	return time.Duration(float64(base) * jitterFactor)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func classifyError(err error) error {
	if err == nil {
		return nil
	}

	// errores de red / timeout
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return fmt.Errorf("provider timeout: %w", err)
		}
		return fmt.Errorf("provider network error: %w", err)
	}

	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "401"), strings.Contains(msg, "unauthorized"), strings.Contains(msg, "invalid api key"):
		return fmt.Errorf("provider unauthorized: %w", err)
	case strings.Contains(msg, "403"), strings.Contains(msg, "forbidden"):
		return fmt.Errorf("provider forbidden: %w", err)
	case strings.Contains(msg, "408"), strings.Contains(msg, "timeout"):
		return fmt.Errorf("provider timeout: %w", err)
	case strings.Contains(msg, "409"):
		return fmt.Errorf("provider conflict: %w", err)
	case strings.Contains(msg, "429"), strings.Contains(msg, "rate limit"), strings.Contains(msg, "too many requests"):
		return fmt.Errorf("provider rate limited: %w", err)
	case strings.Contains(msg, "500"), strings.Contains(msg, "502"), strings.Contains(msg, "503"), strings.Contains(msg, "504"):
		return fmt.Errorf("provider temporary server error: %w", err)
	case strings.Contains(msg, "400"), strings.Contains(msg, "bad request"):
		return fmt.Errorf("provider bad request: %w", err)
	default:
		return err
	}
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	if strings.Contains(msg, "rate limited") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "408") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporary server error") ||
		strings.Contains(msg, "500") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "504") ||
		strings.Contains(msg, "network error") {
		return true
	}

	return false
}
