package main

import (
	"strings"
	"testing"

	"muse-harness/internal/provider"
	"muse-harness/internal/session"
)

// Regression (screenshot report): after a model switch reset the CLI
// session, the model could no longer see earlier messages even though the
// app transcript held them. A fresh/unseeded CLI session must be seeded
// with that transcript.
func TestCliPromptSeedsHistory(t *testing.T) {
	sess := &session.Session{
		CliSessionID: "old-cli-session", // unseeded: CliSeeded=false
		Messages: []provider.Message{
			{Role: "user", Content: "what did I chat about in the top ?"},
			{Role: "assistant", Content: "an earlier answer", Model: "muse-spark-1.3"},
			{Role: "tool", Name: "read_file", Content: "tool noise"},
			{Role: "user", Content: "yes. previously what I SAID ?"}, // just appended
		},
	}
	prompt, seeding := cliPromptFor(sess, "yes. previously what I SAID ?")
	if !seeding {
		t.Fatal("unseeded session must seed")
	}
	for _, want := range []string{"what did I chat about in the top ?", "an earlier answer", "previously what I SAID ?"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("seeded prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "tool noise") {
		t.Fatal("tool results must not be seeded as conversation")
	}
	if strings.Index(prompt, "what did I chat about") > strings.Index(prompt, "an earlier answer") {
		t.Fatal("history must be in conversation order")
	}
}

func TestCliPromptResumeSendsBareText(t *testing.T) {
	sess := &session.Session{
		CliSessionID: "cli-1", CliSeeded: true, CliSeedV: cliSeedVersion,
		Messages: []provider.Message{{Role: "user", Content: "hi"}, {Role: "user", Content: "next"}},
	}
	prompt, seeding := cliPromptFor(sess, "next")
	if seeding || prompt != "next" {
		t.Fatalf("seeded resume must send bare text, got seeding=%v prompt=%q", seeding, prompt)
	}
}

// A session seeded with an older framing version is re-seeded once.
func TestCliPromptReseedsOldVersion(t *testing.T) {
	sess := &session.Session{
		CliSessionID: "cli-1", CliSeeded: true, CliSeedV: cliSeedVersion - 1,
		Messages: []provider.Message{{Role: "user", Content: "Hi !"}, {Role: "user", Content: "again"}},
	}
	prompt, seeding := cliPromptFor(sess, "again")
	if !seeding || !strings.Contains(prompt, "actual history of THIS conversation") {
		t.Fatalf("old seed version must re-seed with new framing: seeding=%v prompt=%q", seeding, prompt)
	}
}

func TestCliPromptNoHistory(t *testing.T) {
	sess := &session.Session{Messages: []provider.Message{{Role: "user", Content: "first"}}}
	prompt, seeding := cliPromptFor(sess, "first")
	if !seeding || prompt != "first" {
		t.Fatalf("first turn: seeding=%v prompt=%q", seeding, prompt)
	}
}
