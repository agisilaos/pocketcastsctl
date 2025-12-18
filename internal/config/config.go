package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Browser     string            `json:"browser"`
	BrowserApp  string            `json:"browser_app"`
	URLContains string            `json:"url_contains"`
	APIBaseURL  string            `json:"api_base_url"`
	APIHeaders  map[string]string `json:"api_headers"`
}

func Default() Config {
	return Config{
		Browser:     "chrome",
		BrowserApp:  "",
		URLContains: "pocketcasts.com",
		APIBaseURL:  "https://api.pocketcasts.com",
		APIHeaders:  map[string]string{},
	}
}

func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "pocketcastsctl-config.json"
	}
	return filepath.Join(dir, "pocketcastsctl", "config.json")
}

func Dir() string {
	return filepath.Dir(Path())
}

func StatePath() string {
	return filepath.Join(Dir(), "state.json")
}

func Load() (Config, error) {
	p := Path()
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", p, err)
	}
	if cfg.Browser == "" {
		cfg.Browser = Default().Browser
	}
	if cfg.BrowserApp == "" {
		cfg.BrowserApp = Default().BrowserApp
	}
	if cfg.URLContains == "" {
		cfg.URLContains = Default().URLContains
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = Default().APIBaseURL
	}
	if cfg.APIHeaders == nil {
		cfg.APIHeaders = map[string]string{}
	}
	return cfg, nil
}

func Save(cfg Config) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(p, b, 0o600)
}
