package musecli

import (
	"context"
	"strings"
	"testing"
)

// Lines are trimmed copies of real `muse exec --provider echo --json`
// output from Muse Code 1.4.14.
const deltaLine = `{"schema_version":1,"stream":{"kind":"session","id":"01a1a477-dfef-7123-bcb4-ece57c73d83a"},"payload_type":"run.output.delta","payload":{"kind":"run_output_delta","text":"echo: Say hello"}}`
const terminalLine = `{"schema_version":1,"stream":{"kind":"session","id":"01a1a477-dfef-7123-bcb4-ece57c73d83a"},"payload_type":"run.terminal.completed","payload":{"kind":"run_terminal","terminal":"completed","text":"echo: Say hello","reason":null}}`
const failedLine = `{"schema_version":1,"stream":{"kind":"session","id":"s1"},"payload_type":"run.terminal.failed","payload":{"kind":"run_terminal","terminal":"failed","text":"","reason":"boom"}}`

func TestParseLineDeltaCarriesSession(t *testing.T) {
	ev, sid, ok := ParseLine([]byte(deltaLine))
	if !ok || ev.Kind != "delta" || ev.Text != "echo: Say hello" {
		t.Fatalf("delta: ok=%v ev=%+v", ok, ev)
	}
	if sid != "01a1a477-dfef-7123-bcb4-ece57c73d83a" {
		t.Fatalf("session id = %q", sid)
	}
}

func TestParseLineTerminal(t *testing.T) {
	ev, _, ok := ParseLine([]byte(terminalLine))
	if !ok || ev.Kind != "done" || ev.Text != "echo: Say hello" {
		t.Fatalf("terminal: ok=%v ev=%+v", ok, ev)
	}
	ev, _, ok = ParseLine([]byte(failedLine))
	if !ok || ev.Kind != "error" || ev.Text != "boom" {
		t.Fatalf("failed terminal: ok=%v ev=%+v", ok, ev)
	}
}

// The GUI must never spawn the muse.cmd -> PowerShell launcher (it opens a
// visible terminal on every run); Find must resolve the real binary.
func TestFindResolvesRealBinary(t *testing.T) {
	bin, err := Find()
	if err != nil {
		t.Skip("muse CLI not on PATH")
	}
	lower := strings.ToLower(bin)
	if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
		t.Fatalf("Find returned launcher shim %q, want the real muse-bin exe", bin)
	}
}

// Live end-to-end check of the subprocess plumbing against the real CLI's
// echo provider (no model, no auth, no cost). Skips when `muse` is absent.
func TestRunnerEchoLive(t *testing.T) {
	bin, err := Find()
	if err != nil {
		t.Skip("muse CLI not on PATH")
	}
	r := Runner{Bin: bin, Provider: "echo"}
	var deltas strings.Builder
	final, sid, err := r.Run(context.Background(), t.TempDir(), "ping", "", func(ev Event) {
		if ev.Kind == "delta" {
			deltas.WriteString(ev.Text)
		}
	})
	if err != nil {
		t.Fatalf("echo run: %v", err)
	}
	if !strings.Contains(final, "echo: ping") || !strings.Contains(deltas.String(), "echo: ping") {
		t.Fatalf("final=%q deltas=%q", final, deltas.String())
	}
	if sid == "" {
		t.Fatal("expected a CLI session id for resume")
	}
}

// Trimmed copies of real tool events from a `muse exec --json` run
// (CLI 1.4.14, prompt "Read hello.txt").
const toolTaskLine = `{"stream":{"kind":"session","id":"s1"},"payload_type":"task.lifecycle.proposed","payload":{"kind":"task_lifecycle","event":{"kind":"proposed","task_kind":"tool.read_file"}}}`
const toolResultLine = `{"stream":{"kind":"session","id":"s1"},"payload_type":"tool.result","payload":{"kind":"tool_result","call_id":"call_1","text":"Read text file ` + "`hello.txt`" + `.\n1|probe","correlation_facts":{"tool_name":"read_file","outcome":"success"}}}`

func TestParseLineToolEvents(t *testing.T) {
	ev, _, ok := ParseLine([]byte(toolTaskLine))
	if !ok || ev.Kind != "tool" || ev.Name != "read_file" || ev.Status != "running" {
		t.Fatalf("tool task: ok=%v ev=%+v", ok, ev)
	}
	ev, _, ok = ParseLine([]byte(toolResultLine))
	if !ok || ev.Kind != "tool" || ev.Name != "read_file" || ev.Status != "done" || !strings.Contains(ev.Text, "1|probe") {
		t.Fatalf("tool result: ok=%v ev=%+v", ok, ev)
	}
	// Non-tool tasks (reminders, model) must not become cards.
	noise := `{"payload_type":"task.lifecycle.proposed","payload":{"kind":"task_lifecycle","event":{"kind":"proposed","task_kind":"reminder.agent.skill-reminder"}}}`
	if _, _, ok := ParseLine([]byte(noise)); ok {
		t.Fatal("reminder task must be ignored")
	}
	// task.lifecycle.output duplicates tool.result; must not double-render.
	dup := `{"payload_type":"task.lifecycle.output","payload":{"kind":"task_lifecycle","event":{"kind":"output","task_kind":"tool.read_file","chunk":"x","final_result":true}}}`
	if _, _, ok := ParseLine([]byte(dup)); ok {
		t.Fatal("output chunk must be ignored (tool.result covers it)")
	}
}

func TestKnownModels(t *testing.T) {
	models := KnownModels()
	if len(models) == 0 || models[0] != "muse-spark-1.3" {
		t.Fatalf("models = %v", models)
	}
	seen := map[string]bool{}
	for _, m := range models {
		if seen[m] {
			t.Fatalf("duplicate model %q", m)
		}
		seen[m] = true
	}
}

func TestParseLineIgnoresNoise(t *testing.T) {
	if _, _, ok := ParseLine([]byte("not json")); ok {
		t.Fatal("non-JSON line must be ignored")
	}
	if _, _, ok := ParseLine([]byte(`{"payload_type":"session.run.linked","payload":{"kind":"session_run_linked"}}`)); ok {
		t.Fatal("unrelated event must be ignored")
	}
}
