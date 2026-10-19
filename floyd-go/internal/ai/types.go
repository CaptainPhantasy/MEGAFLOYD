package ai

type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
)

type Message struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

type AIModel struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Version           string         `json:"version,omitempty"`
	MaxContextLength  int            `json:"max_context_length,omitempty"`
	SupportsStreaming bool           `json:"supports_streaming,omitempty"`
	DefaultParams     map[string]any `json:"default_params,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens,omitempty"`
}

type CompletionOptions struct {
	Model         string   `json:"model,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	MaxTokens     *int     `json:"max_tokens,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`
	StopSequences []string `json:"stop_sequences,omitempty"`
	System        string   `json:"system,omitempty"`
	Stream        bool     `json:"stream,omitempty"`
}

type CompletionRequest struct {
	Model         string    `json:"model"`
	Messages      []Message `json:"messages"`
	Temperature   *float64  `json:"temperature,omitempty"`
	MaxTokens     *int      `json:"max_tokens,omitempty"`
	TopP          *float64  `json:"top_p,omitempty"`
	TopK          *int      `json:"top_k,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
	Stream        bool      `json:"stream,omitempty"`
	System        string    `json:"system,omitempty"`
}

type CompletionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Usage   Usage  `json:"usage"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}

type StreamEvent struct {
	Type          string              `json:"type"`
	Message       *CompletionResponse `json:"message,omitempty"`
	Index         *int                `json:"index,omitempty"`
	Delta         map[string]any      `json:"delta,omitempty"`
	UsageMetadata map[string]int      `json:"usage_metadata,omitempty"`
}
