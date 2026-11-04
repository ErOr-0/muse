package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"muse-harness/internal/agent"
	"muse-harness/internal/config"
	"muse-harness/internal/musecli"
	"muse-harness/internal/provider"
	"muse-harness/internal/session"
	"muse-harness/internal/tools"
)

type App struct {
	ctx      context.Context
	mu       sync.Mutex
	cancel   context.CancelFunc
	pending  map[string]chan bool
	settings config.Settings
	store    session.Store
}

func NewApp() *App { return &App{pending: map[string]chan bool{}} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	s, _ := config.Load()
	a.settings = s
}
func (a *App) shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
}

type SettingsView struct {
	Backend             string  `json:"backend"`
	BaseURL             string  `json:"baseURL"`
	Model               string  `json:"model"`
	Temperature         float64 `json:"temperature"`
	HasTemperature      bool    `json:"hasTemperature"`
	MaxSteps            int     `json:"maxSteps"`
	ContextBudgetTokens int     `json:"contextBudgetTokens"`
	AutoApprove         bool    `json:"autoApprove"`
	LastWorkspace       string  `json:"lastWorkspace"`
	HasAPIKey           bool    `json:"hasAPIKey"`
	APIKeyLast4         string  `json:"apiKeyLast4"`
	APIKeyFromEnv       bool    `json:"apiKeyFromEnv"`
}

type SettingsInput struct {
	Backend             string  `json:"backend"`
	BaseURL             string  `json:"baseURL"`
	APIKey              string  `json:"apiKey"`
	Model               string  `json:"model"`
	Temperature         float64 `json:"temperature"`
	HasTemperature      bool    `json:"hasTemperature"`
	MaxSteps            int     `json:"maxSteps"`
	ContextBudgetTokens int     `json:"contextBudgetTokens"`
	AutoApprove         bool    `json:"autoApprove"`
	LastWorkspace       string  `json:"lastWorkspace"`
}

func (a *App) current() config.Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.settings.BaseURL == "" {
		s, _ := config.Load()
		a.settings = s
	}
	return a.settings
}

func (a *App) GetSettings() SettingsView {
	s := a.current()
	key := s.ResolveAPIKey()
	last4 := ""
	if len(key) >= 4 {
		last4 = key[len(key)-4:]
	}
	v := SettingsView{Backend: s.Backend, BaseURL: s.BaseURL, Model: s.Model, MaxSteps: s.MaxSteps, ContextBudgetTokens: s.ContextBudgetTokens, AutoApprove: s.AutoApprove, LastWorkspace: s.LastWorkspace, HasAPIKey: key != "", APIKeyLast4: last4, APIKeyFromEnv: s.APIKey == "" && key != ""}
	if s.Temperature != nil {
		v.Temperature, v.HasTemperature = *s.Temperature, true
	}
	return v
}

func (a *App) SaveSettings(in SettingsInput) error {
	s := a.current()
	if in.Backend == "cli" || in.Backend == "api" {
		s.Backend = in.Backend
	}
	s.BaseURL, s.Model = strings.TrimSpace(in.BaseURL), strings.TrimSpace(in.Model)
	if strings.TrimSpace(in.APIKey) != "" {
		s.APIKey = strings.TrimSpace(in.APIKey)
	}
	if in.HasTemperature {
		t := in.Temperature
		s.Temperature = &t
	} else {
		s.Temperature = nil
	}
	s.MaxSteps, s.ContextBudgetTokens, s.AutoApprove = in.MaxSteps, in.ContextBudgetTokens, in.AutoApprove
	if in.LastWorkspace != "" {
		s.LastWorkspace = in.LastWorkspace
	}
	if err := config.Save(s); err != nil {
		return err
	}
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	return nil
}

func (a *App) PickWorkspace() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose workspace folder"})
	if err != nil || dir == "" {
		return dir, err
	}
	if _, err := tools.NewScope(dir); err != nil {
		return "", err
	}
	s := a.current()
	s.LastWorkspace = dir
	_ = config.Save(s)
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	return dir, nil
}

func (a *App) NewSession(workspace string) (session.Meta, error) {
	if workspace == "" {
		workspace = a.current().LastWorkspace
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	s := &session.Session{ID: id, Workspace: workspace, Model: a.current().Model, CreatedAt: time.Now(), UpdatedAt: time.Now(), Messages: []provider.Message{}}
	if err := a.store.Save(s); err != nil {
		return session.Meta{}, err
	}
	return session.Meta{ID: s.ID, Title: s.Title, Workspace: s.Workspace, Model: s.Model, UpdatedAt: s.UpdatedAt}, nil
}

func (a *App) ListSessions() []session.Meta {
	list, _ := a.store.List()
	if list == nil {
		return []session.Meta{}
	}
	return list
}

func (a *App) LoadSession(id string) (*session.Session, error) { return a.store.Load(id) }
func (a *App) DeleteSession(id string) error                   { return a.store.Delete(id) }

// ContextStats reports estimated context pressure and cumulative cache
// usage for a session so the UI can show budget + hit-rate affordances.
func (a *App) ContextStats(id string) (map[string]any, error) {
	sess, err := a.store.Load(id)
	if err != nil {
		return nil, err
	}
	s := a.current()
	budget := s.ContextBudgetTokens
	if budget <= 0 {
		budget = 96000
	}
	est := agent.EstimateTokens(append([]provider.Message{{Role: "system", Content: agent.SystemPrompt}}, sess.Messages...))
	return map[string]any{
		"sessionID": id, "estimatedTokens": est, "budgetTokens": budget,
		"compactedMsgs": sess.CompactedMsgs, "compactedTurns": sess.CompactedTurns,
		"usage": sess.TotalUsage,
	}, nil
}

func (a *App) FetchModels() ([]string, error) {
	s := a.current()
	// CLI (subscription) backend: models come from the Muse catalog, not
	// from an HTTP endpoint — no API key is involved, so never call /models.
	if s.Backend == "cli" {
		if _, err := musecli.Find(); err != nil {
			return nil, err
		}
		return musecli.KnownModels(), nil
	}
	p := &provider.OpenAIProvider{BaseURL: s.BaseURL, APIKey: s.ResolveAPIKey()}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	models, err := p.ListModels(ctx)
	if err != nil {
		var perr *provider.ProviderError
		if errors.As(err, &perr) && (perr.Status == 401 || perr.Status == 403) {
			return nil, fmt.Errorf("API key rejected (%d) by %s: this is not a valid Model API key for that endpoint. A Muse subscription is not an API key — switch Backend to 'Muse subscription (CLI)' to fetch models with no key, or paste a pay-as-you-go Model API key", perr.Status, s.BaseURL)
		}
		return nil, err
	}
	return models, nil
}

type CliStatus struct {
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

// MuseStatus reports whether the official Muse Code CLI (the
// subscription backend) is installed. Sign-in itself happens in that CLI.
func (a *App) MuseStatus() CliStatus {
	bin, err := musecli.Find()
	if err != nil {
		return CliStatus{}
	}
	return CliStatus{Found: true, Path: bin, Version: musecli.Version(bin)}
}

func (a *App) CancelRun() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) ResolveApproval(id string, approved bool) {
	a.mu.Lock()
	ch, ok := a.pending[id]
	if ok {
		delete(a.pending, id)
	}
	a.mu.Unlock()
	if ok {
		ch <- approved
	}
}

// cliPromptFor decides what prompt a CLI turn actually sends. The app's
// transcript is the real conversation history; a CLI session only knows
// what it was resumed with. A fresh or never-seeded CLI session therefore
// gets the prior app transcript prepended (seeding), so changing model,
// restarting, or a lost CLI session can never make earlier messages
// invisible to the model. Seeded, resumable sessions send the bare text.
// cliSeedVersion is bumped when the seeding framing changes, so existing
// CLI sessions are re-seeded once with the new framing.
const cliSeedVersion = 2

func cliPromptFor(sess *session.Session, text string) (prompt string, seeding bool) {
	if sess.CliSessionID != "" && sess.CliSeeded && sess.CliSeedV >= cliSeedVersion {
		return text, false
	}
	// Prior history = all messages before the current user message (which
	// runCliTurn has just appended).
	prior := sess.Messages[:max(0, len(sess.Messages)-1)]
	var turns []string
	budget := 48000
	for i := len(prior) - 1; i >= 0 && budget > 0; i-- {
		m := prior[i]
		if (m.Role != "user" && m.Role != "assistant") || strings.TrimSpace(m.Content) == "" {
			continue
		}
		content := m.Content
		if len(content) > 3000 {
			content = content[:3000] + "…[truncated]"
		}
		if len(content) > budget {
			content = content[:budget]
		}
		budget -= len(content)
		who := "User"
		if m.Role == "assistant" {
			who = "Assistant"
		}
		turns = append(turns, who+": "+content)
	}
	if len(turns) == 0 {
		return text, true
	}
	// Collected newest-first; restore conversation order.
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	return "This conversation is being resumed in a fresh session. The lines below are the actual history of THIS conversation: lines marked Assistant are your own earlier replies, lines marked User are what the user actually said earlier in this chat. Treat them exactly as earlier messages in this conversation — if asked about earlier messages, answer directly from them; do not call them a pasted or quoted transcript, and do not say you cannot confirm them.\n\n" +
		strings.Join(turns, "\n\n") +
		"\n\n---\nThe user's new message in this same conversation is:\n\n" + text, true
}

// runCliTurn runs one turn through the official Muse Code CLI. The
// CLI owns auth (subscription), tools, approvals and its sandbox; this app
// streams its JSONL output and persists the transcript.
func (a *App) runCliTurn(ctx context.Context, sess *session.Session, text string, runner musecli.Runner) {
	emitErr := func(msg string) {
		runtime.EventsEmit(a.ctx, "chat:error", map[string]any{"sessionID": sess.ID, "message": msg})
	}
	sess.Messages = append(sess.Messages, provider.Message{Role: "user", Content: text})
	if err := a.store.Save(sess); err != nil {
		emitErr(err.Error())
		return
	}
	prompt, seeding := cliPromptFor(sess, text)
	resumeID := sess.CliSessionID
	if seeding {
		resumeID = "" // start a CLI session that actually has the history
	}
	final, cliSID, err := runner.Run(ctx, sess.Workspace, prompt, resumeID, func(ev musecli.Event) {
		switch ev.Kind {
		case "delta":
			runtime.EventsEmit(a.ctx, "chat:delta", map[string]any{"sessionID": sess.ID, "text": ev.Text, "model": runner.Model})
		case "tool":
			runtime.EventsEmit(a.ctx, "chat:tool", map[string]any{"sessionID": sess.ID, "name": ev.Name, "args": "", "status": ev.Status, "result": ev.Text})
		}
	})
	if cliSID != "" {
		sess.CliSessionID = cliSID
		if seeding {
			sess.CliSeeded = true
			sess.CliSeedV = cliSeedVersion
		}
	}
	if final != "" {
		sess.Messages = append(sess.Messages, provider.Message{Role: "assistant", Content: final, Model: runner.Model})
	}
	_ = a.store.Save(sess)
	if err != nil {
		emitErr(err.Error())
		return
	}
	runtime.EventsEmit(a.ctx, "chat:done", map[string]any{"sessionID": sess.ID, "model": runner.Model})
}

// resolveModel decides the model for the next turn. The Settings model
// (settingsModel) wins; the session's stored model is only a fallback.
// The CLI session is deliberately kept across a model change: verified
// live, `muse exec --session-id <id> --model <new>` switches the model
// for that run (run_usage reports the new model) while preserving the
// CLI conversation. Dropping the session here was the bug that made
// earlier messages unreadable after switching models.
func resolveModel(sess *session.Session, settingsModel string) (effective, sessionModel string) {
	effective = settingsModel
	if effective == "" {
		effective = sess.Model
	}
	return effective, effective
}

func (a *App) SendMessage(sessionID, text string) error {
	sess, err := a.store.Load(sessionID)
	if err != nil {
		return err
	}
	if sess.Workspace == "" {
		return fmt.Errorf("choose a workspace folder first")
	}
	scope, err := tools.NewScope(sess.Workspace)
	if err != nil {
		return err
	}
	settings := a.current()
	// The current Settings model wins on every turn: a model changed
	// mid-conversation must apply to the next message instead of staying
	// frozen at whatever model the session was created with.
	settings.Model, sess.Model = resolveModel(sess, settings.Model)
	if sess.Model != "" {
		_ = a.store.Save(sess) // persist the switch (and CLI-session reset)
	}
	prov := &provider.OpenAIProvider{BaseURL: settings.BaseURL, APIKey: settings.ResolveAPIKey()}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.cancel = cancel
	a.mu.Unlock()

	if settings.Backend == "cli" {
		runner := musecli.Runner{Model: settings.Model, MaxSteps: settings.MaxSteps}
		go func() {
			defer cancel()
			a.runCliTurn(ctx, sess, text, runner)
		}()
		return nil
	}

	loop := &agent.Loop{
		Provider: prov, Settings: settings, Registry: tools.NewRegistry(), Scope: scope,
		Approve: func(toolName, argsJSON, diffPreview string) bool {
			id := fmt.Sprintf("%d", time.Now().UnixNano())
			ch := make(chan bool, 1)
			a.mu.Lock()
			a.pending[id] = ch
			a.mu.Unlock()
			runtime.EventsEmit(a.ctx, "approval:request", map[string]any{"id": id, "tool": toolName, "args": argsJSON, "diff": diffPreview, "sessionID": sessionID})
			select {
			case ok := <-ch:
				return ok
			case <-time.After(5 * time.Minute):
				return false
			case <-ctx.Done():
				return false
			}
		},
	}
	go func() {
		defer cancel()
		_ = loop.Run(ctx, sess, text, func(ev agent.Event) {
			switch ev.Kind {
			case "delta":
				runtime.EventsEmit(a.ctx, "chat:delta", map[string]any{"sessionID": sessionID, "text": ev.Text, "model": settings.Model})
			case "tool":
				runtime.EventsEmit(a.ctx, "chat:tool", map[string]any{"sessionID": sessionID, "name": ev.Name, "args": ev.Args, "status": ev.Status, "result": ev.Result, "diff": ev.Diff})
			case "compact":
				runtime.EventsEmit(a.ctx, "chat:compact", map[string]any{"sessionID": sessionID, "text": ev.Text})
			case "done":
				runtime.EventsEmit(a.ctx, "chat:done", map[string]any{"sessionID": sessionID, "usage": ev.Usage, "model": settings.Model})
			case "error":
				runtime.EventsEmit(a.ctx, "chat:error", map[string]any{"sessionID": sessionID, "message": ev.Message})
			}
		})
	}()
	return nil
}
