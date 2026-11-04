package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	// Backend selects how the model is reached: "cli" drives the official
	// Muse Code CLI (`muse exec`, subscription sign-in stays inside that
	// CLI); "api" calls an OpenAI-compatible endpoint with an API key.
	Backend             string   `json:"backend"`
	BaseURL             string   `json:"baseURL"`
	APIKey              string   `json:"apiKey,omitempty"`
	Model               string   `json:"model"`
	Temperature         *float64 `json:"temperature,omitempty"`
	MaxSteps            int      `json:"maxSteps"`
	ContextBudgetTokens int      `json:"contextBudgetTokens"`
	CompactionKeepGroups int     `json:"compactionKeepGroups,omitempty"`
	AutoApprove         bool     `json:"autoApprove"`
	LastWorkspace       string   `json:"lastWorkspace"`
}

func Defaults() Settings {
	return Settings{Backend: "cli", BaseURL: "https://api.meta.ai/v1", Model: "muse-spark-1.3", MaxSteps: 25, ContextBudgetTokens: 96000, CompactionKeepGroups: 4}
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "muse-harness")
	return d, os.MkdirAll(filepath.Join(d, "sessions"), 0o755)
}

func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "settings.json"), nil
}

func Load() (Settings, error) {
	s := Defaults()
	p, err := Path()
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return s, Save(s)
	}
	if err != nil {
		return s, err
	}
	_ = json.Unmarshal(b, &s)
	if s.Backend != "cli" && s.Backend != "api" {
		s.Backend = "cli"
	}
	if s.MaxSteps <= 0 {
		s.MaxSteps = 25
	}
	if s.ContextBudgetTokens <= 0 {
		s.ContextBudgetTokens = 96000
	}
	if s.BaseURL == "" {
		s.BaseURL = Defaults().BaseURL
	}
	if s.Model == "" {
		s.Model = Defaults().Model
	}
	return s, nil
}

func Save(s Settings) error {
	p, err := Path()
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ResolveAPIKey: settings > MUSE_API_KEY > META_API_KEY > MODEL_API_KEY > OPENAI_API_KEY. Never log the value.
func (s Settings) ResolveAPIKey() string {
	if s.APIKey != "" {
		return s.APIKey
	}
	for _, k := range []string{"MUSE_API_KEY", "META_API_KEY", "MODEL_API_KEY", "OPENAI_API_KEY"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
