package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"muse-harness/internal/config"
	"muse-harness/internal/diff"
	"muse-harness/internal/provider"
	"muse-harness/internal/session"
	"muse-harness/internal/tools"
)

func readFileBytes(p string) ([]byte, error) { return os.ReadFile(p) }
func diffUnified(a, b string) string {
	if len(a) > 32*1024 {
		a = a[:32*1024]
	}
	if len(b) > 32*1024 {
		b = b[:32*1024]
	}
	return diff.Unified(a, b)
}

type Event struct {
	Kind    string // delta|tool|done|error|compact
	Text    string
	Name    string
	Args    string
	Status  string // running|done|error
	Result  string
	Diff    string
	Message string
	Usage   provider.Usage
}

// ApprovalFunc returns true if the mutating action may proceed.
type ApprovalFunc func(toolName string, argsJSON string, diffPreview string) bool

type Loop struct {
	Provider provider.Provider
	Settings config.Settings
	Registry *tools.Registry
	Scope    *tools.Scope
	Approve  ApprovalFunc
}

// SystemPrompt is the stable prefix start. It must stay byte-identical
// across calls so the provider's automatic prefix cache keeps hitting.
const SystemPrompt = "You are muse-harness, a coding agent working inside the user's chosen workspace folder. Use the tools to inspect and modify files. All file paths are relative to the workspace root. Prefer read/search before editing, make minimal changes, and explain briefly what you did. Never ask for absolute paths."

func (l *Loop) Run(ctx context.Context, sess *session.Session, userText string, emit func(Event)) error {
	store := session.Store{}
	sess.Messages = append(sess.Messages, provider.Message{Role: "user", Content: userText})
	_ = store.Save(sess)
	maxSteps := l.Settings.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 25
	}
	budget := l.Settings.ContextBudgetTokens
	if budget <= 0 {
		budget = 96000
	}
	keepGroups := l.Settings.CompactionKeepGroups
	if keepGroups <= 0 {
		keepGroups = 4
	}
	for step := 0; step < maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			emit(Event{Kind: "error", Message: "cancelled"})
			return err
		}
		// Token-pressure-triggered one-shot checkpoint compaction. On
		// success the session is rewritten once (checkpoint + recent tail)
		// and the new prefix stays stable and cacheable; on failure fall
		// back to lossless-order truncation for this turn only.
		if NeedsCompaction(viewMessages(sess), budget) {
			if replacement, droppedMsgs, droppedGroups, ok := CompactOldest(ctx, l.Provider, l.Settings.Model, sess.Messages, keepGroups); ok {
				sess.Messages = replacement
				sess.CompactedMsgs += droppedMsgs
				sess.CompactedTurns += droppedGroups
				_ = store.Save(sess)
				emit(Event{Kind: "compact", Text: fmt.Sprintf("Compacted %d messages in %d turns into a checkpoint summary.", droppedMsgs, droppedGroups)})
			}
		}
		msgs := Truncate(viewMessages(sess), budget)
		res, err := l.Provider.StreamChat(ctx, provider.ChatRequest{
			Model: l.Settings.Model, Messages: msgs, Tools: l.Registry.Schemas(), Temperature: l.Settings.Temperature,
		}, func(d provider.Delta) error {
			emit(Event{Kind: "delta", Text: d.Content})
			return nil
		})
		if err != nil {
			emit(Event{Kind: "error", Message: err.Error()})
			return err
		}
		sess.AddUsage(res.Usage)
		res.Message.Model = l.Settings.Model
		sess.Messages = append(sess.Messages, res.Message)
		_ = store.Save(sess)
		if len(res.Message.ToolCalls) == 0 {
			emit(Event{Kind: "done", Usage: res.Usage})
			return nil
		}
		for _, tc := range res.Message.ToolCalls {
			emit(Event{Kind: "tool", Name: tc.Name, Args: tc.Arguments, Status: "running"})
			text, diffText := l.execTool(ctx, tc)
			status := "done"
			if strings.HasPrefix(text, "error:") || strings.HasPrefix(text, "user denied") {
				status = "error"
			}
			emit(Event{Kind: "tool", Name: tc.Name, Args: tc.Arguments, Status: status, Result: text, Diff: diffText})
			sess.Messages = append(sess.Messages, provider.Message{Role: "tool", ToolCallID: tc.ID, Name: tc.Name, Content: text})
			_ = store.Save(sess)
		}
	}
	emit(Event{Kind: "error", Message: fmt.Sprintf("stopped after %d steps", maxSteps)})
	return nil
}

func (l *Loop) execTool(ctx context.Context, tc provider.ToolCall) (string, string) {
	tool, ok := l.Registry.Get(tc.Name)
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", tc.Name), ""
	}
	var raw json.RawMessage = []byte(tc.Arguments)
	if !json.Valid(raw) {
		return "error: invalid JSON arguments: " + tc.Arguments, ""
	}
	if tool.Mutating() && !l.Settings.AutoApprove {
		preview := ""
		// Best-effort diff preview for write/edit before approval.
		if tc.Name == "write_file" || tc.Name == "edit_file" {
			preview = previewDiff(l.Scope, tc)
		}
		if l.Approve == nil || !l.Approve(tc.Name, tc.Arguments, preview) {
			return "user denied this action", ""
		}
	}
	if err := ctx.Err(); err != nil {
		return "error: cancelled", ""
	}
	res, err := tool.Execute(ctx, l.Scope, raw)
	if err != nil {
		return "error: " + err.Error(), ""
	}
	return res.Text, res.Diff
}

func previewDiff(scope *tools.Scope, tc provider.ToolCall) string {
	if scope == nil {
		return ""
	}
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Old     string `json:"old_string"`
		New     string `json:"new_string"`
	}
	if json.Unmarshal([]byte(tc.Arguments), &a) != nil || a.Path == "" {
		return ""
	}
	abs, err := scope.Resolve(a.Path)
	if err != nil {
		return ""
	}
	old := ""
	if b, err := readFileBytes(abs); err == nil {
		old = string(b)
	}
	switch tc.Name {
	case "write_file":
		return diffUnified(old, a.Content)
	case "edit_file":
		if strings.Count(old, a.Old) == 1 {
			return diffUnified(old, strings.Replace(old, a.Old, a.New, 1))
		}
	}
	return ""
}

func viewMessages(sess *session.Session) []provider.Message {
	out := []provider.Message{{Role: "system", Content: SystemPrompt}}
	out = append(out, sess.Messages...)
	return out
}
