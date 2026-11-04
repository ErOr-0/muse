package agent

import (
	"strings"
	"testing"

	"muse-harness/internal/provider"
)

func TestTruncateKeepsGroups(t *testing.T) {
	big := strings.Repeat("x", 400)
	msgs := []provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: big, ToolCalls: []provider.ToolCall{{ID: "1", Name: "read_file", Arguments: "{}"}}},
		{Role: "tool", ToolCallID: "1", Content: big},
		{Role: "user", Content: big},
		{Role: "assistant", Content: big},
		{Role: "user", Content: "last question"},
		{Role: "assistant", Content: "answer"},
	}
	out := Truncate(msgs, 300)
	if out[0].Role != "system" {
		t.Fatal("system lost")
	}
	// No tool message without its preceding assistant tool_call in output.
	seenCall := map[string]bool{}
	for _, m := range out {
		for _, tc := range m.ToolCalls {
			seenCall[tc.ID] = true
		}
		if m.Role == "tool" && !seenCall[m.ToolCallID] {
			t.Error("tool result split from its call")
		}
	}
	if out[len(out)-1].Content != "answer" {
		t.Error("latest turn lost")
	}
}
