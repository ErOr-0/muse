package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"muse-harness/internal/config"
	"muse-harness/internal/provider"
)

type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Workspace string `json:"workspace"`
	Model     string `json:"model"`
	// CliSessionID is the Muse Code CLI session id (CLI backend only),
	// passed back as `muse exec --session-id` to resume the conversation.
	// CliSeeded records that this CLI session began with the app's full
	// transcript in its first prompt; unseeded sessions are re-seeded once,
	// which also repairs chats whose CLI session started mid-way (e.g.
	// after a model switch reset it).
	CliSessionID string `json:"cliSessionID,omitempty"`
	CliSeeded    bool   `json:"cliSeeded,omitempty"`
	// CliSeedV is the seeding-prompt version the CLI session was seeded
	// with; older versions are re-seeded once when the framing improves.
	CliSeedV  int                `json:"cliSeedV,omitempty"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
	Messages  []provider.Message `json:"messages"`
	// Compaction checkpoint counters + cumulative token usage.
	CompactedMsgs  int            `json:"compactedMsgs,omitempty"`
	CompactedTurns int            `json:"compactedTurns,omitempty"`
	TotalUsage     provider.Usage `json:"totalUsage,omitempty"`
}

func (s *Session) AddUsage(u provider.Usage) {
	s.TotalUsage.PromptTokens += u.PromptTokens
	s.TotalUsage.CompletionTokens += u.CompletionTokens
	s.TotalUsage.TotalTokens += u.TotalTokens
	s.TotalUsage.CacheHitTokens += u.CacheHitTokens
	s.TotalUsage.CacheMissTokens += u.CacheMissTokens
}

type Meta struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Workspace string    `json:"workspace"`
	Model     string    `json:"model"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Store struct{}

func dir() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "sessions"), nil
}

func (Store) Save(s *Session) error {
	d, err := dir()
	if err != nil {
		return err
	}
	s.UpdatedAt = time.Now()
	if s.Title == "" {
		for _, m := range s.Messages {
			if m.Role == "user" {
				t := strings.TrimSpace(m.Content)
				if len(t) > 60 {
					t = t[:60]
				}
				s.Title = t
				break
			}
		}
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	p := filepath.Join(d, s.ID+".json")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (Store) Load(id string) (*Session, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(d, filepath.Base(id)+".json"))
	if err != nil {
		return nil, err
	}
	var s Session
	return &s, json.Unmarshal(b, &s)
}

func (Store) List() ([]Meta, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	ents, _ := os.ReadDir(d)
	out := []Meta{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		s, err := Store{}.Load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue
		}
		out = append(out, Meta{ID: s.ID, Title: s.Title, Workspace: s.Workspace, Model: s.Model, UpdatedAt: s.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (Store) Delete(id string) error {
	d, err := dir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(d, filepath.Base(id)+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
