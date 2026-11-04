package agent

import (
	"fmt"
	"math"

	"muse-harness/internal/provider"
)

func estimate(m provider.Message) int {
	n := int(math.Ceil(float64(len(m.Content))/4)) + 8
	for _, tc := range m.ToolCalls {
		n += int(math.Ceil(float64(len(tc.Arguments))/4)) + 8
	}
	return n
}

// Truncate keeps system + first user, drops oldest middle groups (user..tool results kept whole).
func Truncate(msgs []provider.Message, budget int) []provider.Message {
	if budget <= 0 {
		budget = 96000
	}
	total := 0
	for _, m := range msgs {
		total += estimate(m)
	}
	if total <= budget || len(msgs) <= 2 {
		return msgs
	}
	// Group: index 0 system kept, rest grouped by user boundary.
	head := msgs[:1]
	rest := msgs[1:]
	// Find groups: each starts at a user message.
	var groups [][]provider.Message
	for _, m := range rest {
		if m.Role == "user" || len(groups) == 0 {
			groups = append(groups, []provider.Message{m})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], m)
		}
	}
	dropped := 0
	for total > budget && len(groups) > 1 {
		// drop oldest group after the first
		g := groups[1]
		for _, m := range g {
			total -= estimate(m)
			dropped++
		}
		groups = append(groups[:1], groups[2:]...)
	}
	out := append([]provider.Message{}, head...)
	if dropped > 0 {
		out = append(out, provider.Message{Role: "system", Content: fmt.Sprintf("[%d earlier messages truncated]", dropped)})
	}
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}
