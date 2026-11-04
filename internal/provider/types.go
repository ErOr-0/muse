package provider

import "context"

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
	// Model records which model produced an assistant message, so the UI
	// can caption the reply card. Empty for user/tool messages.
	Model      string     `json:"model,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ChatRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolSchema
	Temperature *float64
	MaxTokens   int
}

type Delta struct {
	Content string
}

type Usage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
	// CacheHit/Miss track prefix-cache effectiveness (DeepSeek-style
	// prompt_cache_hit_tokens / prompt_cache_miss_tokens, plus OpenAI
	// cached_tokens variants). Zero when the provider omits usage.
	CacheHitTokens  int `json:"cacheHitTokens"`
	CacheMissTokens int `json:"cacheMissTokens"`
}

type ChatResult struct {
	Message Message
	Usage   Usage
}

type ProviderError struct {
	Status      int
	BodySnippet string
}

type Provider interface {
	ID() string
	StreamChat(ctx context.Context, req ChatRequest, onDelta func(Delta) error) (ChatResult, error)
	// Chat is a single non-streaming turn, used for compaction summaries.
	Chat(ctx context.Context, req ChatRequest) (ChatResult, error)
}
