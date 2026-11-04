package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"muse-harness/internal/diff"
	"muse-harness/internal/provider"
)

type Tool interface {
	Name() string
	Mutating() bool
	Schema() provider.ToolSchema
	Execute(ctx context.Context, scope *Scope, args json.RawMessage) (Result, error)
}

type Result struct {
	Text string `json:"text"`
	Diff string `json:"diff,omitempty"`
}

type Registry struct{ tools map[string]Tool }

func NewRegistry() *Registry {
	r := &Registry{tools: map[string]Tool{}}
	for _, t := range []Tool{listFiles{}, readFile{}, writeFile{}, editFile{}, searchTool{}, runCmd{}} {
		r.tools[t.Name()] = t
	}
	return r
}

func (r *Registry) Get(name string) (Tool, bool) { t, ok := r.tools[name]; return t, ok }

func (r *Registry) Schemas() []provider.ToolSchema {
	names := []string{}
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := []provider.ToolSchema{}
	for _, n := range names {
		out = append(out, r.tools[n].Schema())
	}
	return out
}

func schema(name, desc string, props map[string]any, req ...string) provider.ToolSchema {
	return provider.ToolSchema{Name: name, Description: desc, Parameters: map[string]any{"type": "object", "properties": props, "required": req}}
}
func strProp(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
func intProp(d string) map[string]any { return map[string]any{"type": "integer", "description": d} }
func capText(s string, n int) string {
	if len(s) > n {
		return s[:n] + "\n[truncated]"
	}
	return s
}

// list_files
type listFiles struct{}

func (listFiles) Name() string   { return "list_files" }
func (listFiles) Mutating() bool { return false }
func (listFiles) Schema() provider.ToolSchema {
	return schema("list_files", "List files in a workspace-relative directory (sorted, dirs end with /).", map[string]any{"path": strProp("Relative dir, default .")})
}
func (listFiles) Execute(_ context.Context, s *Scope, args json.RawMessage) (Result, error) {
	var a struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &a)
	abs, err := s.Resolve(a.Path)
	if err != nil {
		return Result{}, err
	}
	ents, err := os.ReadDir(abs)
	if err != nil {
		return Result{}, err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	lines := []string{}
	for _, e := range ents {
		if len(lines) >= 500 {
			lines = append(lines, "[capped at 500 entries]")
			break
		}
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	return Result{Text: strings.Join(lines, "\n")}, nil
}

// read_file
type readFile struct{}

func (readFile) Name() string   { return "read_file" }
func (readFile) Mutating() bool { return false }
func (readFile) Schema() provider.ToolSchema {
	return schema("read_file", "Read a workspace file with 1-based line numbers.", map[string]any{"path": strProp("Relative path"), "offset": intProp("1-based start line"), "limit": intProp("Max lines, default 500")}, "path")
}
func (readFile) Execute(_ context.Context, s *Scope, args json.RawMessage) (Result, error) {
	var a struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, err
	}
	abs, err := s.Resolve(a.Path)
	if err != nil {
		return Result{}, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return Result{}, err
	}
	lines := strings.Split(string(b), "\n")
	off := a.Offset
	if off < 1 {
		off = 1
	}
	lim := a.Limit
	if lim <= 0 {
		lim = 500
	}
	var out []string
	for i := off - 1; i < len(lines) && len(out) < lim; i++ {
		out = append(out, fmt.Sprintf("%d: %s", i+1, lines[i]))
	}
	return Result{Text: capText(strings.Join(out, "\n"), 64*1024)}, nil
}

// write_file
type writeFile struct{}

func (writeFile) Name() string   { return "write_file" }
func (writeFile) Mutating() bool { return true }
func (writeFile) Schema() provider.ToolSchema {
	return schema("write_file", "Create or replace a workspace file (parents auto-created). Requires approval unless auto-approve is on.", map[string]any{"path": strProp("Relative path"), "content": map[string]any{"type": "string"}}, "path", "content")
}
func (writeFile) Execute(_ context.Context, s *Scope, args json.RawMessage) (Result, error) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, err
	}
	abs, err := s.Resolve(a.Path)
	if err != nil {
		return Result{}, err
	}
	old := ""
	if b, err := os.ReadFile(abs); err == nil {
		old = string(b)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Result{}, err
	}
	tmp := abs + ".tmp"
	if err := os.WriteFile(tmp, []byte(a.Content), 0o644); err != nil {
		return Result{}, err
	}
	if err := os.Rename(tmp, abs); err != nil {
		return Result{}, err
	}
	return Result{Text: fmt.Sprintf("wrote %s (%d bytes)", s.Rel(abs), len(a.Content)), Diff: diff.Unified(capText(old, 32*1024), capText(a.Content, 32*1024))}, nil
}

// edit_file
type editFile struct{}

func (editFile) Name() string   { return "edit_file" }
func (editFile) Mutating() bool { return true }
func (editFile) Schema() provider.ToolSchema {
	return schema("edit_file", "Replace old_string with new_string; old_string must occur exactly once. Requires approval unless auto-approve is on.", map[string]any{"path": strProp("Relative path"), "old_string": map[string]any{"type": "string"}, "new_string": map[string]any{"type": "string"}}, "path", "old_string", "new_string")
}
func (editFile) Execute(_ context.Context, s *Scope, args json.RawMessage) (Result, error) {
	var a struct {
		Path string `json:"path"`
		Old  string `json:"old_string"`
		New  string `json:"new_string"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, err
	}
	abs, err := s.Resolve(a.Path)
	if err != nil {
		return Result{}, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return Result{}, err
	}
	old := string(b)
	n := strings.Count(old, a.Old)
	if n != 1 {
		return Result{}, fmt.Errorf("old_string must occur exactly once, found %d occurrences in %s", n, s.Rel(abs))
	}
	updated := strings.Replace(old, a.Old, a.New, 1)
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return Result{}, err
	}
	return Result{Text: fmt.Sprintf("edited %s", s.Rel(abs)), Diff: diff.Unified(capText(old, 32*1024), capText(updated, 32*1024))}, nil
}

// search
type searchTool struct{}

func (searchTool) Name() string   { return "search" }
func (searchTool) Mutating() bool { return false }
func (searchTool) Schema() provider.ToolSchema {
	return schema("search", "Regex search (RE2) over workspace text files. Skips .git/node_modules/dist and binaries.", map[string]any{"pattern": strProp("Regex pattern"), "path": strProp("Relative subdir, default ."), "glob": strProp("Optional filename glob e.g. *.go")}, "pattern")
}
func (searchTool) Execute(_ context.Context, s *Scope, args json.RawMessage) (Result, error) {
	var a struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Glob    string `json:"glob"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, err
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return Result{}, err
	}
	root, err := s.Resolve(a.Path)
	if err != nil {
		return Result{}, err
	}
	skip := map[string]bool{".git": true, "node_modules": true, "dist": true}
	matches := []string{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			// Never descend into a symlinked/junction directory that
			// resolves outside the workspace.
			if p != root {
				if rel, rerr := filepath.Rel(s.Root, p); rerr != nil {
					return filepath.SkipDir
				} else if _, rerr := s.Resolve(rel); rerr != nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		// Resolve-check every walked file: a junction/symlink entry
		// pointing outside must never be read through the walk path.
		if rel, rerr := filepath.Rel(s.Root, p); rerr != nil {
			return nil
		} else if _, rerr := s.Resolve(rel); rerr != nil {
			return nil
		}
		if a.Glob != "" {
			if ok, _ := filepath.Match(a.Glob, d.Name()); !ok {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil || info.Size() > 2*1024*1024 {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for i := 0; i < len(b) && i < 512; i++ {
			if b[i] == 0 {
				return nil
			}
		}
		for ln, line := range strings.Split(string(b), "\n") {
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", s.Rel(p), ln+1, strings.TrimSpace(line)))
				if len(matches) >= 250 {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if len(matches) == 0 {
		return Result{Text: "no matches"}, nil
	}
	return Result{Text: capText(strings.Join(matches, "\n"), 64*1024)}, nil
}

// run_command
type runCmd struct{}

func (runCmd) Name() string   { return "run_command" }
func (runCmd) Mutating() bool { return true }
func (runCmd) Schema() provider.ToolSchema {
	return schema("run_command", "Run a shell command in the workspace root (Windows: cmd /C). Timeout default 120s, max 600s. Requires approval unless auto-approve is on.", map[string]any{"command": strProp("Command line"), "timeout_seconds": intProp("Timeout seconds")}, "command")
}
func (runCmd) Execute(ctx context.Context, s *Scope, args json.RawMessage) (Result, error) {
	var a struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, err
	}
	to := a.Timeout
	if to <= 0 {
		to = 120
	}
	if to > 600 {
		to = 600
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(to)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "cmd", "/C", a.Command)
	hideWindow(cmd)
	cmd.Dir = s.Root
	cmd.Env = append(os.Environ(), "CI=1")
	out, err := cmd.CombinedOutput()
	text := capText(string(out), 64*1024)
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return Result{Text: text + fmt.Sprintf("\n[error: %v]", err)}, nil
		}
	}
	return Result{Text: fmt.Sprintf("%s\n[exit code: %d]", text, code)}, nil
}
