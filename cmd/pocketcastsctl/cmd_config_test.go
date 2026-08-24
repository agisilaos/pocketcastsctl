package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestRunConfigUnknownSubcommand(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"config", "nope"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown config subcommand: nope") {
		t.Fatalf("stderr missing unknown subcommand message: %q", stderr)
	}
}

func TestRunConfigShowJSONRedactsSecrets(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, cfgPath)
	err := os.WriteFile(cfgPath, []byte(`{
  "browser":"chrome",
  "url_contains":"pocketcasts.com",
  "api_base_url":"https://api.pocketcasts.com",
  "api_headers":{"Authorization":"Bearer secret-token"}
}`), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	code, stdout, stderr := runForTest(t, []string{"config", "show", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, `"Authorization": "[redacted]"`) {
		t.Fatalf("stdout missing redacted Authorization header: %q", stdout)
	}
	if strings.Contains(stdout, "secret-token") {
		t.Fatalf("stdout leaked secret token: %q", stdout)
	}
}

func TestRunConfigSetBrowserPersistsSelection(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, cfgPath)
	previous := applicationAvailable
	applicationAvailable = func(appName string) bool { return appName == "Safari" }
	t.Cleanup(func() { applicationAvailable = previous })

	code, stdout, stderr := runForTest(t, []string{"config", "set", "browser", "safari"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "browser: safari") || !strings.Contains(stdout, "next: pocketcastsctl doctor --quick") {
		t.Fatalf("stdout missing saved browser workflow: %q", stdout)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Browser != "safari" || cfg.BrowserApp != "" {
		t.Fatalf("saved browser config = %#v", cfg)
	}
}

func TestRunConfigSetBrowserRejectsMissingApplication(t *testing.T) {
	previous := applicationAvailable
	applicationAvailable = func(string) bool { return false }
	t.Cleanup(func() { applicationAvailable = previous })

	code, _, stderr := runForTest(t, []string{"config", "set", "browser", "chrome"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, `config set failed: browser application "Google Chrome" is not installed`) {
		t.Fatalf("stderr missing installed-app error: %q", stderr)
	}
}
