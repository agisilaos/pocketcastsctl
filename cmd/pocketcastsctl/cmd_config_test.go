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
