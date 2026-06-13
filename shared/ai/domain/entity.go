package domain

type ResponseMode string

const (
	ResponseModeText ResponseMode = "text"
	ResponseModeJSON ResponseMode = "json"
)

type Request struct {
	SystemPrompt string
	UserPrompt   string

	// Optional runtime overrides
	Model       string
	Temperature *float64
	MaxTokens   *int64
	TopP        *float64
	Stop        []string

	// Output behavior
	ResponseMode ResponseMode
}

func NewRequest(systemPrompt, userPrompt string) *Request {
	return &Request{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		ResponseMode: ResponseModeText,
	}
}

type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type Response struct {
	OutputText   string                 `json:"output_text"`
	OutputMap    map[string]interface{} `json:"output_map,omitempty"`
	FinishReason string                 `json:"finish_reason,omitempty"`
	Model        string                 `json:"model,omitempty"`
	Provider     string                 `json:"provider,omitempty"`
	Usage        *Usage                 `json:"usage,omitempty"`

	HasError bool   `json:"has_error"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

func NewResponse(
	outputText string,
	outputMap map[string]interface{},
	finishReason string,
	model string,
	provider string,
	usage *Usage,
	hasError bool,
	errorMsg string,
) (*Response, error) {
	return &Response{
		OutputText:   outputText,
		OutputMap:    outputMap,
		FinishReason: finishReason,
		Model:        model,
		Provider:     provider,
		Usage:        usage,
		HasError:     hasError,
		ErrorMsg:     errorMsg,
	}, nil
}
