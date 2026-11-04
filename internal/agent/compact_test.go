package agent

import (
	"context"
	"strings"
	"testing"

	"muse-harness/internal/provider"
)

type fakeProvider struct {
	summary string
	err     error
	got     []provider.Message
}

func (f *fakeProvider) ID() string { return "fake" }
func (f *fakeProvider) StreamChat(_ context.Context, _ provider.ChatRequest, _ func(provider.Delta) error) (provider.ChatResult, error) {
	return provider.ChatResult{}, nil
}
func (f *fakeProvider) Chat(_ context.Context, req provider.ChatRequest) (provider.ChatResult, error) {
	f.got = req.Messages
	if f.err != nil {
		return provider.ChatResult{}, f.err
	}
	return provider.ChatResult{Message: provider.Message{Role: "assistant", Content: f.summary}}, nil
}

func TestNeedsCompactionThreshold(t *testing.T) {
	big := strings.Repeat("x", 4000) // ~1000 tokens
	msgs := []provider.Message{{Role: "system", Content: "s"}}
	for i := 0; i < 90; i++ {
		msgs = append(msgs, provider.Message{Role: "user", Content: big})
	}
	if !NeedsCompaction(msgs, 96000) {
		t.Error("expected pressure at ~90k tokens of a 96k budget")
	}
	if NeedsCompaction(msgs[:5], 96000) {
		t.Error("small history must not trigger compaction")
	}
}

func TestCompactOldestKeepsTailWhole(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "a", ToolCalls: []provider.ToolCall{{ID: "1", Name: "read_file", Arguments: "{}"}}},
		{Role: "tool", ToolCallID: "1", Content: "contents"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "third"},
		{Role: "assistant", Content: "c"},
	}
	fp := &fakeProvider{summary: "Goals: test. Files: none."}
	out, droppedMsgs, droppedGroups, ok := CompactOldest(context.Background(), fp, "m", msgs, 1)
	if !ok {
		t.Fatal("expected compaction to succeed")
	}
	if droppedGroups != 2 || droppedMsgs != 5 {
		t.Errorf("dropped=%d msgs in %d groups, want 5 in 2", droppedMsgs, droppedGroups)
	}
	if len(out) != 3 || !strings.Contains(out[0].Content, "checkpoint") || !strings.Contains(out[0].Content, "Goals: test") {
		t.Fatalf("checkpoint head wrong: %+v", out)
	}
	// Tail preserved verbatim, tool call never split from its result.
	if out[1].Content != "third" || out[2].Content != "c" {
		t.Errorf("tail not preserved: %+v", out[1:])
	}
	// The summarization prompt must replay the same prefix shape the model
	// saw (system + oldest) with the instruction appended at the tail.
	if len(fp.got) == 0 || fp.got[0].Role != "system" {
		t.Error("compaction prompt must start with the system prefix")
	}
	last := fp.got[len(fp.got)-1]
	if last.Role != "user" || !strings.Contains(last.Content, "checkpoint") {
		t.Error("compaction instruction must be appended at the tail")
	}
}

func TestCompactOldestFailureFallsBack(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Content: "a"}, {Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"}, {Role: "assistant", Content: "d"},
	}
	fp := &fakeProvider{summary: "   "}
	if _, _, _, ok := CompactOldest(context.Background(), fp, "m", msgs, 1); ok {
		t.Error("empty summary must report failure so the caller falls back to Truncate")
	}
}

func TestFormatCheckpointDeterministic(t *testing.T) {
	a := FormatCheckpoint("s", 10, 2)
	b := FormatCheckpoint("s", 10, 2)
	if a.Content != b.Content || a.Role != "user" {
		t.Error("checkpoint framing must be deterministic")
	}
}
