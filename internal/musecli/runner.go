// Package musecli drives Meta's official Muse Code CLI headlessly.
//
// This is the subscription path: the user signs in once inside the official
// CLI (`muse`, browser sign-in with a Muse Code subscription). That
// credential never leaves the CLI — this package only spawns
// `muse exec --json`, feeds it a prompt file, and translates its JSONL
// event stream. No token is read, minted, or stored here.
package musecli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Event is one translated CLI event for the UI.
type Event struct {
	Kind   string // delta | tool | done | error
	Text   string // delta text, or tool result text
	Name   string // tool name (tool events)
	Status string // running | done | error (tool events)
}

type Runner struct {
	Bin      string // resolved `muse` binary; empty = LookPath
	Provider string // meta (default) or echo (tests)
	Model    string
	MaxSteps int
}

func Find() (string, error) {
	p, err := exec.LookPath("muse")
	if err != nil {
		return "", fmt.Errorf("Muse CLI not found on PATH: install Muse Code (dev.meta.ai/docs/muse-code), run `muse` once and sign in with your subscription, then retry")
	}
	// On Windows `muse` is muse.cmd -> powershell.exe launcher -> the real
	// binary. Spawning the shim from a GUI opens a PowerShell window on every
	// run, so resolve the sibling muse-bin-<version>.exe directly instead.
	if strings.HasSuffix(strings.ToLower(p), ".cmd") || strings.HasSuffix(strings.ToLower(p), ".bat") {
		if exe := siblingBinary(p); exe != "" {
			return exe, nil
		}
	}
	return p, nil
}

// siblingBinary finds muse-bin-<version>.exe next to the launcher shim,
// preferring the version named in .muse-version (what the launcher runs),
// else the most recently modified one.
func siblingBinary(shim string) string {
	dir := filepath.Dir(shim)
	if v, err := os.ReadFile(filepath.Join(dir, ".muse-version")); err == nil {
		candidate := filepath.Join(dir, "muse-bin-"+strings.TrimSpace(string(v))+".exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "muse-bin-*.exe"))
	var best string
	var bestTime time.Time
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil && fi.ModTime().After(bestTime) {
			best, bestTime = m, fi.ModTime()
		}
	}
	return best
}

// KnownModels is the Muse catalog the CLI accepts as --model values
// (dev.meta.ai/docs/models). The CLI has no list command and
// `model-profile show` accepts any id, so this is a static catalog —
// the CLI itself rejects a bad model at run time. No API key is involved.
func KnownModels() []string {
	return []string{
		"muse-spark-1.3",
		"muse-spark-1.2",
		"muse-spark-1.1",
		"muse-spark-1.3-contributor",
		"muse-spark-1.2-contributor",
		"muse-wonder-1.0",
	}
}

func Version(bin string) string {
	out, err := command(context.Background(), bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// command wraps .cmd/.bat shims on Windows, which CreateProcess cannot run
// directly, and hides the console window of every child it creates.
func command(ctx context.Context, bin string, args ...string) *exec.Cmd {
	lower := strings.ToLower(bin)
	var cmd *exec.Cmd
	if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
		full := append([]string{"/C", bin}, args...)
		cmd = exec.CommandContext(ctx, "cmd.exe", full...)
	} else {
		cmd = exec.CommandContext(ctx, bin, args...)
	}
	hideWindow(cmd)
	return cmd
}

// jsonlLine is the subset of the `muse exec --json` wire format we consume.
// Verified against Muse Code 1.4.14 `--provider echo` output.
type jsonlLine struct {
	Stream struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"stream"`
	PayloadType string `json:"payload_type"`
	Payload     struct {
		Kind     string `json:"kind"`
		Text     string `json:"text"`
		Terminal string `json:"terminal"`
		Reason   string `json:"reason"`
		Event    struct {
			Kind     string `json:"kind"`
			TaskKind string `json:"task_kind"`
			Reason   string `json:"reason"`
		} `json:"event"`
		CorrelationFacts struct {
			ToolName string `json:"tool_name"`
			Outcome  string `json:"outcome"`
		} `json:"correlation_facts"`
	} `json:"payload"`
}

// ParseLine translates one JSONL line. sessionID is returned when the line
// carries the CLI session stream id. ok=false means "ignore this line".
func ParseLine(line []byte) (ev Event, sessionID string, ok bool) {
	var l jsonlLine
	if json.Unmarshal(line, &l) != nil {
		return Event{}, "", false
	}
	if l.Stream.Kind == "session" && l.Stream.ID != "" {
		sessionID = l.Stream.ID
	}
	// Tool calls surface two ways (verified on CLI 1.4.14): a task lifecycle
	// event with task_kind "tool.<name>", and a final tool.result carrying
	// the tool name, outcome and result text. task.lifecycle.output chunks
	// duplicate tool.result, so they are not emitted separately.
	if name, isTool := strings.CutPrefix(l.Payload.Event.TaskKind, "tool."); isTool {
		switch l.Payload.Event.Kind {
		case "proposed", "started":
			return Event{Kind: "tool", Name: name, Status: "running"}, sessionID, true
		case "failed", "rejected":
			return Event{Kind: "tool", Name: name, Status: "error", Text: l.Payload.Event.Reason}, sessionID, true
		}
		return Event{}, sessionID, false
	}
	switch {
	case l.PayloadType == "tool.result":
		status := "done"
		if l.Payload.CorrelationFacts.Outcome != "" && l.Payload.CorrelationFacts.Outcome != "success" {
			status = "error"
		}
		name := l.Payload.CorrelationFacts.ToolName
		if name == "" {
			name = "tool"
		}
		return Event{Kind: "tool", Name: name, Status: status, Text: l.Payload.Text}, sessionID, true
	case l.PayloadType == "run.output.delta" && l.Payload.Text != "":
		return Event{Kind: "delta", Text: l.Payload.Text}, sessionID, true
	case l.Payload.Kind == "run_terminal":
		if l.Payload.Terminal == "completed" {
			return Event{Kind: "done", Text: l.Payload.Text}, sessionID, true
		}
		reason := l.Payload.Reason
		if reason == "" {
			reason = l.Payload.Terminal
		}
		return Event{Kind: "error", Text: reason}, sessionID, true
	}
	return Event{}, sessionID, sessionID != ""
}

// Run executes one headless turn in workspace and streams translated events.
// Approvals: a headless run has nobody to answer the CLI's prompts, so we
// pass --disable-approval (prompts off, the CLI's OS sandbox stays ON).
// --yolo is never used: it would also disable the sandbox.
func (r Runner) Run(ctx context.Context, workspace, prompt, cliSessionID string, emit func(Event)) (finalText, sessionID string, err error) {
	bin := r.Bin
	if bin == "" {
		if bin, err = Find(); err != nil {
			return "", "", err
		}
	}
	pf, err := os.CreateTemp("", "muse-harness-prompt-*.txt")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(pf.Name())
	if _, err := pf.WriteString(prompt); err != nil {
		pf.Close()
		return "", "", err
	}
	pf.Close()

	provider := r.Provider
	if provider == "" {
		provider = "meta"
	}
	args := []string{"exec", "--json", "--provider", provider,
		"--workspace", workspace, "--prompt-file", pf.Name(), "--disable-approval"}
	if cliSessionID != "" {
		args = append(args, "--session-id", cliSessionID, "--allow-workspace-switch")
	}
	if r.Model != "" && provider == "meta" {
		args = append(args, "--model", r.Model)
	}
	if r.MaxSteps > 0 {
		args = append(args, "--max-model-steps", fmt.Sprint(r.MaxSteps))
	}
	if fi, err := os.Stat(workspace); err != nil || !fi.IsDir() {
		return "", "", fmt.Errorf("workspace folder does not exist: %s — choose the workspace folder again in the app", workspace)
	}
	cmd := command(ctx, bin, args...)
	cmd.Dir = workspace
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("start muse CLI: %w", err)
	}
	var toolErr string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue // CLI also writes plain status lines; stdout JSONL only
		}
		ev, sid, ok := ParseLine(line)
		if sid != "" {
			sessionID = sid
		}
		if !ok {
			continue
		}
		switch ev.Kind {
		case "delta":
			finalText += ev.Text
			emit(ev)
		case "done":
			if ev.Text != "" {
				finalText = ev.Text
			}
		case "error":
			toolErr = ev.Text
		default:
			emit(ev)
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return finalText, sessionID, ctx.Err()
	}
	if toolErr != "" {
		return finalText, sessionID, fmt.Errorf("muse run failed: %s", toolErr)
	}
	if waitErr != nil {
		tail := strings.TrimSpace(stderr.String())
		if len(tail) > 600 {
			tail = tail[len(tail)-600:]
		}
		return finalText, sessionID, fmt.Errorf("muse exec failed (%v): %s", waitErr, tail)
	}
	return finalText, sessionID, nil
}
