package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func scope(t *testing.T) *Scope {
	t.Helper()
	s, err := NewScope(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Regression: a renamed/deleted workspace used to surface only as a
// cryptic fork/exec "directory name is invalid" from the CLI child process.
func TestNewScopeMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "renamed-away")
	if _, err := NewScope(missing); err == nil {
		t.Fatal("NewScope on missing folder must fail")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error should name the problem, got: %v", err)
	}
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewScope(file); err == nil {
		t.Fatal("NewScope on a file must fail")
	}
}

func TestScopeRejectsEscapes(t *testing.T) {
	s := scope(t)
	for _, p := range []string{"../escape.txt", "..\\escape.txt", "a/../../escape.txt", "/etc/passwd", "C:\\Windows\\x.txt"} {
		if _, err := s.Resolve(p); err == nil {
			t.Errorf("Resolve(%q) should fail", p)
		}
	}
	if _, err := s.Resolve("sub/ok.txt"); err != nil {
		t.Errorf("inside path failed: %v", err)
	}
}

func TestScopeSymlinkEscape(t *testing.T) {
	s := scope(t)
	outside := t.TempDir()
	link := filepath.Join(s.Root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := s.Resolve("link/evil.txt"); err == nil {
		t.Error("symlink escape should fail")
	}
}

// Junctions (mklink /J) need no privilege on Windows, unlike os.Symlink,
// so they are the realistic reparse-point escape vector here.
func TestScopeJunctionEscape(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction test is Windows-only")
	}
	s := scope(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("junction-needle"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.Root, "jdir")
	if out, err := exec.Command("cmd", "/C", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Skipf("mklink /J unavailable: %v (%s)", err, out)
	}
	defer exec.Command("cmd", "/C", "rmdir", link).Run()
	if _, err := s.Resolve("jdir"); err == nil {
		t.Error("Resolve(jdir) should fail for junction escaping workspace")
	}
	if _, err := s.Resolve("jdir/secret.txt"); err == nil {
		t.Error("Resolve(jdir/secret.txt) should fail for junction escaping workspace")
	}
	reg := NewRegistry()
	rf, _ := reg.Get("read_file")
	if _, err := rf.Execute(context.Background(), s, json.RawMessage(`{"path":"jdir/secret.txt"}`)); err == nil {
		t.Error("read_file through junction should fail")
	}
	w, _ := reg.Get("write_file")
	if _, err := w.Execute(context.Background(), s, json.RawMessage(`{"path":"jdir/pwn.txt","content":"x"}`)); err == nil {
		t.Error("write_file through junction should fail")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwn.txt")); err == nil {
		t.Error("write through junction created file outside workspace")
	}
	srch, _ := reg.Get("search")
	res, err := srch.Execute(context.Background(), s, json.RawMessage(`{"pattern":"junction-needle"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "no matches" {
		t.Errorf("search leaked junction content: %s", res.Text)
	}
	if _, err := srch.Execute(context.Background(), s, json.RawMessage(`{"pattern":"junction-needle","path":"jdir"}`)); err == nil {
		t.Error("search rooted at junction should fail")
	}
}

func TestEditUniqueMatch(t *testing.T) {
	s := scope(t)
	reg := NewRegistry()
	w, _ := reg.Get("write_file")
	if _, err := w.Execute(context.Background(), s, json.RawMessage(`{"path":"a.txt","content":"hello hello"}`)); err != nil {
		t.Fatal(err)
	}
	e, _ := reg.Get("edit_file")
	if _, err := e.Execute(context.Background(), s, json.RawMessage(`{"path":"a.txt","old_string":"hello","new_string":"hi"}`)); err == nil {
		t.Error("non-unique edit should fail")
	}
	if _, err := e.Execute(context.Background(), s, json.RawMessage(`{"path":"a.txt","old_string":"absent","new_string":"hi"}`)); err == nil {
		t.Error("absent edit should fail")
	}
	res, err := e.Execute(context.Background(), s, json.RawMessage(`{"path":"a.txt","old_string":"hello hello","new_string":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Diff == "" {
		t.Error("edit should return diff")
	}
	b, _ := os.ReadFile(filepath.Join(s.Root, "a.txt"))
	if string(b) != "hi" {
		t.Errorf("content=%q", b)
	}
}

func TestWriteConfinedAndSearch(t *testing.T) {
	s := scope(t)
	reg := NewRegistry()
	w, _ := reg.Get("write_file")
	if _, err := w.Execute(context.Background(), s, json.RawMessage(`{"path":"../evil.txt","content":"x"}`)); err == nil {
		t.Error("escape write should fail")
	}
	if _, err := w.Execute(context.Background(), s, json.RawMessage(`{"path":"sub/code.go","content":"package x\n// needle here\n"}`)); err != nil {
		t.Fatal(err)
	}
	srch, _ := reg.Get("search")
	res, err := srch.Execute(context.Background(), s, json.RawMessage(`{"pattern":"needle"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Text == "no matches" {
		t.Errorf("search missed: %s", res.Text)
	}
	lf, _ := reg.Get("list_files")
	res, _ = lf.Execute(context.Background(), s, json.RawMessage(`{"path":"."}`))
	if res.Text == "" {
		t.Error("list_files empty")
	}
	rf, _ := reg.Get("read_file")
	res, err = rf.Execute(context.Background(), s, json.RawMessage(`{"path":"sub/code.go"}`))
	if err != nil || res.Text == "" {
		t.Errorf("read_file: %v %q", err, res.Text)
	}
}

func TestRunCommandExitCode(t *testing.T) {
	s := scope(t)
	reg := NewRegistry()
	rc, _ := reg.Get("run_command")
	res, err := rc.Execute(context.Background(), s, json.RawMessage(`{"command":"echo hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Text) == 0 {
		t.Error("empty output")
	}
}
