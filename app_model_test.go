package main

import (
	"testing"

	"muse-harness/internal/session"
)

// Regression 1: changing the model in Settings mid-conversation had no
// effect because SendMessage preferred the model frozen on the session.
// Regression 2: dropping the CLI session on a model change destroyed the
// model's view of earlier messages. Verified live that resuming the same
// CLI session with --model <new> switches model and keeps context, so
// the session must be preserved.
func TestResolveModelSettingsWin(t *testing.T) {
	sess := &session.Session{Model: "muse-spark-1.3", CliSessionID: "cli-1"}
	effective, stored := resolveModel(sess, "muse-wonder-1.0")
	if effective != "muse-wonder-1.0" || stored != "muse-wonder-1.0" {
		t.Fatalf("effective=%q stored=%q, want muse-wonder-1.0", effective, stored)
	}
	if sess.CliSessionID != "cli-1" {
		t.Fatal("CLI session must be kept across a model change (per-run --model switches it, context survives)")
	}
}

func TestResolveModelSameModelKeepsCliSession(t *testing.T) {
	sess := &session.Session{Model: "muse-spark-1.3", CliSessionID: "cli-1"}
	effective, _ := resolveModel(sess, "muse-spark-1.3")
	if effective != "muse-spark-1.3" || sess.CliSessionID != "cli-1" {
		t.Fatalf("effective=%q cliSession=%q", effective, sess.CliSessionID)
	}
}

func TestResolveModelFallbackToSession(t *testing.T) {
	sess := &session.Session{Model: "muse-spark-1.2"}
	effective, _ := resolveModel(sess, "")
	if effective != "muse-spark-1.2" {
		t.Fatalf("effective=%q, want session fallback", effective)
	}
}
