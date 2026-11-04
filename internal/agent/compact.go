package agent

import (
	"context"
	"fmt"
	"strings"

	"muse-harness/internal/provider"
)

// DeepSeek-harness style context management: stable prefix + append-only
// history + token-pressure-triggered one-shot checkpoint summarization.
//
// Cache-everything invariant: the model-visible prefix (system prompt, tool
// schemas, then history) must stay byte-identical across calls so the
// provider's automatic prefix cache hits. History is therefore append-only;
// the only rewrite allowed is a compaction checkpoint, which replaces the
// oldest groups with a single deterministic summary message and then becomes
// the new stable prefix start.

// EstimateTokens sums the chars/4 heuristic over messages.
func EstimateTokens(msgs []provider.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimate(m)
	}
	return total
}

// SplitGroups groups messages by user boundary so a tool call is never split
// from its results. Groups[0] is the oldest.
func SplitGroups(msgs []provider.Message) [][]provider.Message {
	var groups [][]provider.Message
	for _, m := range msgs {
		if m.Role == "user" || len(groups) == 0 {
			groups = append(groups, []provider.Message{m})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], m)
		}
	}
	return groups
}

// NeedsCompaction reports token pressure at >=80% of budget.
func NeedsCompaction(msgs []provider.Message, budget int) bool {
	if budget <= 0 {
		budget = 96000
	}
	return EstimateTokens(msgs) >= budget*4/5
}

const compactionInstruction = "Summarize this conversation prefix into a structured checkpoint for continued coding work. " +
	"Be concise and factual. Sections: Goals, Key decisions, Files changed (path + what), " +
	"Tool results worth keeping (paths, test outcomes, errors), Open todos, Current user request. " +
	"Do not invent file contents; keep names/paths exact."

// BuildCompactionPrompt replays the exact prefix the model already saw and
// appends the compaction instruction at the tail (DeepSeek-harness style),
// maximizing prefix-cache reuse for the summarization call itself.
func BuildCompactionPrompt(prefix []provider.Message) []provider.Message {
	out := append([]provider.Message(nil), prefix...)
	out = append(out, provider.Message{Role: "user", Content: compactionInstruction})
	return out
}

// FormatCheckpoint renders a deterministic checkpoint message. Deterministic
// framing keeps the new prefix stable across retries.
func FormatCheckpoint(summary string, droppedMsgs, droppedGroups int) provider.Message {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "(empty summary)"
	}
	return provider.Message{
		Role: "user",
		Content: fmt.Sprintf("[Context checkpoint: %d earlier messages in %d turns compacted into the summary below. "+
			"Continue from this summary plus the recent turns that follow.]\n%s",
			droppedMsgs, droppedGroups, summary),
	}
}

// CompactOldest summarizes the oldest groups via one provider call and
// returns the replacement session messages: checkpoint + kept recent groups.
// keepGroups recent groups are preserved verbatim (append-only tail).
// On summarization failure it returns ok=false so the caller can fall back
// to Truncate.
func CompactOldest(ctx context.Context, p provider.Provider, model string, sessMsgs []provider.Message, keepGroups int) (replacement []provider.Message, droppedMsgs, droppedGroups int, ok bool) {
	if keepGroups < 1 {
		keepGroups = 1
	}
	groups := SplitGroups(sessMsgs)
	if len(groups) <= keepGroups {
		return nil, 0, 0, false
	}
	cut := len(groups) - keepGroups
	var oldest []provider.Message
	for _, g := range groups[:cut] {
		oldest = append(oldest, g...)
	}
	// Replay the same prefix shape the model saw: system + oldest.
	prefix := append([]provider.Message{{Role: "system", Content: SystemPrompt}}, oldest...)
	res, err := p.Chat(ctx, provider.ChatRequest{
		Model:    model,
		Messages: BuildCompactionPrompt(prefix),
	})
	if err != nil || strings.TrimSpace(res.Message.Content) == "" {
		return nil, 0, 0, false
	}
	checkpoint := FormatCheckpoint(res.Message.Content, len(oldest), cut)
	replacement = append([]provider.Message{checkpoint}, flatten(groups[cut:])...)
	return replacement, len(oldest), cut, true
}

func flatten(groups [][]provider.Message) []provider.Message {
	var out []provider.Message
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}
