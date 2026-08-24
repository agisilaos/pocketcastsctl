package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	EnvConfigPath  = "POCKETCASTS_CONFIG"
	EnvBrowser     = "POCKETCASTS_BROWSER"
	EnvBrowserApp  = "POCKETCASTS_BROWSER_APP"
	EnvURLContains = "POCKETCASTS_URL_CONTAINS"
	EnvAPIBaseURL  = "POCKETCASTS_API_BASE_URL"
	EnvAccessToken = "POCKETCASTS_ACCESS_TOKEN"
)

// AuthConfig contains non-secret metadata for the active API session. Token
// material is stored in the operating system credential store under SessionKey.
type AuthConfig struct {
	SessionKey string `json:"session_key,omitempty"`
	AccountID  string `json:"account_id,omitempty"`
	Email      string `json:"email,omitempty"`
	Method     string `json:"method,omitempty"`
	Scope      string `json:"scope,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
}

type Config struct {
	Browser     string            `json:"browser"`
	BrowserApp  string            `json:"browser_app"`
	URLContains string            `json:"url_contains"`
	APIBaseURL  string            `json:"api_base_url"`
	APIHeaders  map[string]string `json:"api_headers"`
	Auth        AuthConfig        `json:"auth,omitempty"`
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
	if p := os.Getenv(EnvConfigPath); p != "" {
		return p
	}
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
			cfg := Default()
			applyEnvironment(&cfg)
			return cfg, nil
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

	applyEnvironment(&cfg)

	return cfg, nil
}

func applyEnvironment(cfg *Config) {
	if v := os.Getenv(EnvBrowser); v != "" {
		cfg.Browser = v
	}
	if v := os.Getenv(EnvBrowserApp); v != "" {
		cfg.BrowserApp = v
	}
	if v := os.Getenv(EnvURLContains); v != "" {
		cfg.URLContains = v
	}
	if v := os.Getenv(EnvAPIBaseURL); v != "" {
		cfg.APIBaseURL = v
	}
}

func Save(cfg Config) error {
	p := Path()
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, p)
}
