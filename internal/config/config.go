package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvConfigPath  = "POCKETCASTS_CONFIG"
	EnvBrowser     = "POCKETCASTS_BROWSER"
	EnvBrowserApp  = "POCKETCASTS_BROWSER_APP"
	EnvURLContains = "POCKETCASTS_URL_CONTAINS"
	EnvAPIBaseURL  = "POCKETCASTS_API_BASE_URL"
	EnvAccessToken = "POCKETCASTS_ACCESS_TOKEN"
)

// AuthConfig contains non-secret metadata for the saved API session. Token
// material is stored in the operating system credential store under SessionKey.
type AuthConfig struct {
	SessionKey string `json:"session_key,omitempty"`
	AccountID  string `json:"account_id,omitempty"`
	Email      string `json:"email,omitempty"`
	Method     string `json:"method,omitempty"`
	Scope      string `json:"scope,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
}

// Config is the effective runtime configuration after defaults and environment
// settings have been applied. Callers must never serialize it as persisted state.
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
	doc, exists, err := readDocument()
	if err != nil {
		return Config{}, err
	}
	if !exists {
		cfg := Default()
		applyEnvironment(&cfg)
		return cfg, nil
	}
	return resolve(doc, true)
}

// NormalizeAPIBaseURL is the canonical issuer identity used by config guards
// and credential-store keys.
func NormalizeAPIBaseURL(value string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(value)), "/")
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

func resolve(doc document, environment bool) (Config, error) {
	var cfg Config
	if err := decodeKnown(doc, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", Path(), err)
	}
	defaults := Default()
	if cfg.Browser == "" {
		cfg.Browser = defaults.Browser
	}
	if cfg.BrowserApp == "" {
		cfg.BrowserApp = defaults.BrowserApp
	}
	if cfg.URLContains == "" {
		cfg.URLContains = defaults.URLContains
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = defaults.APIBaseURL
	}
	if cfg.APIHeaders == nil {
		cfg.APIHeaders = map[string]string{}
	}
	if environment {
		applyEnvironment(&cfg)
	}
	return cfg, nil
}
