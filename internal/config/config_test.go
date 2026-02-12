package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathFromEnv(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom.json")
	t.Setenv(EnvConfigPath, want)
	if got := Path(); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(EnvConfigPath, cfgPath)

	err := os.WriteFile(cfgPath, []byte(`{
  "browser": "safari",
  "browser_app": "Safari",
  "url_contains": "play.pocketcasts.com",
  "api_base_url": "https://api.example.com",
  "api_headers": {"Authorization":"Bearer token"}
}
`), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv(EnvBrowser, "chrome")
	t.Setenv(EnvBrowserApp, "Google Chrome")
	t.Setenv(EnvURLContains, "pocketcasts.com")
	t.Setenv(EnvAPIBaseURL, "https://api.pocketcasts.com")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Browser != "chrome" {
		t.Fatalf("Browser = %q, want chrome", got.Browser)
	}
	if got.BrowserApp != "Google Chrome" {
		t.Fatalf("BrowserApp = %q, want Google Chrome", got.BrowserApp)
	}
	if got.URLContains != "pocketcasts.com" {
		t.Fatalf("URLContains = %q, want pocketcasts.com", got.URLContains)
	}
	if got.APIBaseURL != "https://api.pocketcasts.com" {
		t.Fatalf("APIBaseURL = %q, want https://api.pocketcasts.com", got.APIBaseURL)
	}
	if got.APIHeaders["Authorization"] != "Bearer token" {
		t.Fatalf("APIHeaders[Authorization] = %q, want preserved token", got.APIHeaders["Authorization"])
	}
}
