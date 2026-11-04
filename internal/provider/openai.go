package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type OpenAIProvider struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func (p *OpenAIProvider) ID() string { return "openai-compatible" }

func (e *ProviderError) Error() string {
	if e == nil {
		return "provider error"
	}
	return fmt.Sprintf("provider HTTP %d: %s", e.Status, e.BodySnippet)
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}
type wireMsg struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []wireCall `json:"tool_calls,omitempty"`
}
type wireCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func (p *OpenAIProvider) client() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func stableTools(in []ToolSchema) []wireTool {
	// Cache-everything: tool definitions are part of the cached prefix, so
	// they must be byte-identical across calls. Sort by name (the registry
	// already does, but re-sort here so callers can't break the prefix).
	cp := append([]ToolSchema(nil), in...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Name < cp[j].Name })
	var out []wireTool
	for _, t := range cp {
		var wt wireTool
		wt.Type = "function"
		wt.Function.Name, wt.Function.Description, wt.Function.Parameters = t.Name, t.Description, t.Parameters
		out = append(out, wt)
	}
	return out
}

func wireMessages(in []Message) []wireMsg {
	msgs := make([]wireMsg, 0, len(in))
	for _, m := range in {
		wm := wireMsg{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			var c wireCall
			c.ID, c.Type = tc.ID, "function"
			c.Function.Name, c.Function.Arguments = tc.Name, tc.Arguments
			wm.ToolCalls = append(wm.ToolCalls, c)
		}
		msgs = append(msgs, wm)
	}
	return msgs
}

// parseUsage accepts DeepSeek-style (prompt_cache_hit_tokens /
// prompt_cache_miss_tokens) and OpenAI-style (prompt_tokens_details.cached_tokens)
// shapes. Missing fields stay zero.
func parseUsage(raw json.RawMessage) Usage {
	var u struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		PromptCacheHit   int `json:"prompt_cache_hit_tokens"`
		PromptCacheMiss  int `json:"prompt_cache_miss_tokens"`
		PromptDetails    struct {
			Cached int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	}
	if json.Unmarshal(raw, &u) != nil {
		return Usage{}
	}
	hit := u.PromptCacheHit
	if hit == 0 {
		hit = u.PromptDetails.Cached
	}
	miss := u.PromptCacheMiss
	if miss == 0 && u.PromptTokens > 0 && hit > 0 {
		miss = u.PromptTokens - hit
		if miss < 0 {
			miss = 0
		}
	}
	return Usage{
		PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens,
		TotalTokens: u.TotalTokens, CacheHitTokens: hit, CacheMissTokens: miss,
	}
}

func (p *OpenAIProvider) base() string {
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = "https://api.meta.ai/v1"
	}
	return base
}

func (p *OpenAIProvider) StreamChat(ctx context.Context, req ChatRequest, onDelta func(Delta) error) (ChatResult, error) {
	base := p.base()
	msgs := wireMessages(req.Messages)
	tools := stableTools(req.Tools)
	body := map[string]any{"model": req.Model, "messages": msgs, "stream": true,
		"stream_options": map[string]any{"include_usage": true}}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	raw, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return ChatResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := p.client().Do(httpReq)
	if err != nil {
		return ChatResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return ChatResult{}, &ProviderError{Status: resp.StatusCode, BodySnippet: strings.TrimSpace(string(snip))}
	}
	var content strings.Builder
	var usage Usage
	byIndex := map[int]*ToolCall{}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if len(chunk.Usage) > 0 {
			usage = parseUsage(chunk.Usage)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta
		if d.Content != "" {
			content.WriteString(d.Content)
			if onDelta != nil {
				if err := onDelta(Delta{Content: d.Content}); err != nil {
					return ChatResult{}, err
				}
			}
		}
		for _, tc := range d.ToolCalls {
			cur, ok := byIndex[tc.Index]
			if !ok {
				cur = &ToolCall{}
				byIndex[tc.Index] = cur
			}
			if tc.ID != "" {
				cur.ID = tc.ID
			}
			if tc.Function.Name != "" {
				cur.Name += tc.Function.Name
			}
			cur.Arguments += tc.Function.Arguments
		}
	}
	if err := sc.Err(); err != nil {
		return ChatResult{}, err
	}
	idx := make([]int, 0, len(byIndex))
	for i := range byIndex {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	msg := Message{Role: "assistant", Content: content.String()}
	for _, i := range idx {
		msg.ToolCalls = append(msg.ToolCalls, *byIndex[i])
	}
	return ChatResult{Message: msg, Usage: usage}, nil
}

// Chat performs one non-streaming turn (used for compaction summaries).
func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (ChatResult, error) {
	base := p.base()
	body := map[string]any{"model": req.Model, "messages": wireMessages(req.Messages), "stream": false}
	if tools := stableTools(req.Tools); len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	raw, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return ChatResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := p.client().Do(httpReq)
	if err != nil {
		return ChatResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return ChatResult{}, &ProviderError{Status: resp.StatusCode, BodySnippet: strings.TrimSpace(string(snip))}
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ChatResult{}, err
	}
	if len(out.Choices) == 0 {
		return ChatResult{}, fmt.Errorf("provider returned no choices")
	}
	msg := Message{Role: "assistant", Content: out.Choices[0].Message.Content}
	for _, tc := range out.Choices[0].Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments})
	}
	return ChatResult{Message: msg, Usage: parseUsage(out.Usage)}, nil
}

// ListModels fetches GET {base}/models; never logs the key.
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]string, error) {
	base := strings.TrimRight(p.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, &ProviderError{Status: resp.StatusCode, BodySnippet: strings.TrimSpace(string(snip))}
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := []string{}
	for _, d := range out.Data {
		ids = append(ids, d.ID)
	}
	sort.Strings(ids)
	return ids, nil
}
