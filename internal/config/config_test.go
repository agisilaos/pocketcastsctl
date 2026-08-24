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

func TestDefaultUsesBuiltInBrowser(t *testing.T) {
	if got := Default().Browser; got != "chrome" {
		t.Fatalf("Default().Browser = %q, want chrome", got)
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

func TestLoadAppliesEnvironmentWhenConfigIsMissing(t *testing.T) {
	t.Setenv(EnvConfigPath, filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv(EnvAPIBaseURL, "https://api.example.test")
	t.Setenv(EnvBrowser, "dia")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIBaseURL != "https://api.example.test" || cfg.Browser != "dia" {
		t.Fatalf("environment overrides were not applied: %+v", cfg)
	}
}

func TestSaveWritesPrivateConfigAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	t.Setenv(EnvConfigPath, path)
	cfg := Default()
	cfg.Auth = AuthConfig{SessionKey: "metadata-only", Method: "password"}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.SessionKey != "metadata-only" {
		t.Fatalf("session key = %q", loaded.Auth.SessionKey)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}
