package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSSEToolCallReassembly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing bearer")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi \"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"read_\",\"arguments\":\"{\\\"pa\"}}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"file\",\"arguments\":\"th\\\":\\\"a.txt\\\"}\"}}]}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p := &OpenAIProvider{BaseURL: srv.URL, APIKey: "test-key"}
	var streamed string
	res, err := p.StreamChat(context.Background(), ChatRequest{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}}, func(d Delta) error { streamed += d.Content; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if streamed != "hi " || res.Message.Content != "hi " {
		t.Errorf("content=%q streamed=%q", res.Message.Content, streamed)
	}
	if len(res.Message.ToolCalls) != 1 || res.Message.ToolCalls[0].Name != "read_file" || res.Message.ToolCalls[0].Arguments != `{"path":"a.txt"}` {
		t.Errorf("toolcalls=%+v", res.Message.ToolCalls)
	}
}

func TestProviderErrorNoKeyLeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, "bad key")
	}))
	defer srv.Close()
	p := &OpenAIProvider{BaseURL: srv.URL, APIKey: "secret-value"}
	_, err := p.StreamChat(context.Background(), ChatRequest{Model: "m"}, nil)
	if err == nil || err.Error() == "" {
		t.Fatal("expected error")
	}
	if got := err.Error(); len(got) > 0 && contains(got, "secret-value") {
		t.Error("error leaked api key")
	}
}
func TestParseUsageDeepSeekAndOpenAI(t *testing.T) {
	u := parseUsage([]byte(`{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_cache_hit_tokens":80,"prompt_cache_miss_tokens":20}`))
	if u.CacheHitTokens != 80 || u.CacheMissTokens != 20 || u.PromptTokens != 100 {
		t.Errorf("deepseek usage wrong: %+v", u)
	}
	u = parseUsage([]byte(`{"prompt_tokens":50,"prompt_tokens_details":{"cached_tokens":30}}`))
	if u.CacheHitTokens != 30 || u.CacheMissTokens != 20 {
		t.Errorf("openai cached_tokens wrong: %+v", u)
	}
	if got := parseUsage([]byte(`{}`)); got.CacheHitTokens != 0 || got.PromptTokens != 0 {
		t.Errorf("empty usage must stay zero: %+v", got)
	}
}

func TestChatNonStreamingParsesUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"summary"}}],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_cache_hit_tokens":80,"prompt_cache_miss_tokens":20}}`)
	}))
	defer srv.Close()
	p := &OpenAIProvider{BaseURL: srv.URL, APIKey: "k"}
	res, err := p.Chat(context.Background(), ChatRequest{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Message.Content != "summary" {
		t.Errorf("content=%q", res.Message.Content)
	}
	if res.Usage.CacheHitTokens != 80 || res.Usage.TotalTokens != 105 {
		t.Errorf("usage=%+v", res.Usage)
	}
}

func TestStreamChatCapturesUsageChunk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":40,\"completion_tokens\":2,\"total_tokens\":42,\"prompt_cache_hit_tokens\":30}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p := &OpenAIProvider{BaseURL: srv.URL, APIKey: "k"}
	res, err := p.StreamChat(context.Background(), ChatRequest{Model: "m", Messages: []Message{{Role: "user", Content: "x"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Message.Content != "hi" {
		t.Errorf("content=%q", res.Message.Content)
	}
	if res.Usage.CacheHitTokens != 30 || res.Usage.TotalTokens != 42 {
		t.Errorf("usage=%+v", res.Usage)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
