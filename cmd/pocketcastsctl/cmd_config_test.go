package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestRunConfigSetBrowserDoesNotPersistRuntimeSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, path)
	writeSavedConfigForTest(t, map[string]any{
		"browser":      "chrome",
		"url_contains": "saved.example",
		"api_base_url": "https://saved.example",
		"future":       map[string]any{"keep": true},
	})
	t.Setenv(config.EnvBrowserApp, "Temporary App")
	t.Setenv(config.EnvURLContains, "temporary.example")
	t.Setenv(config.EnvAPIBaseURL, "https://temporary.example")
	previous := applicationAvailable
	applicationAvailable = func(appName string) bool { return appName == "Safari" }
	t.Cleanup(func() { applicationAvailable = previous })

	code, _, stderr := runForTest(t, []string{"config", "set", "browser", "safari"}, "")
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	doc := readSavedConfigForTest(t)
	if doc["browser"] != "safari" || doc["browser_app"] != "" || doc["url_contains"] != "saved.example" || doc["api_base_url"] != "https://saved.example" {
		t.Fatalf("saved config = %#v", doc)
	}
	if _, ok := doc["future"]; !ok {
		t.Fatal("config set discarded an unknown field")
	}
}

func TestRuntimeSettingCombinationsNeverPersistFromCLI(t *testing.T) {
	previousAvailable := applicationAvailable
	previousOpen := openWebLoginBrowser
	applicationAvailable = func(string) bool { return true }
	openWebLoginBrowser = func(string, string, ...string) error { return nil }
	t.Cleanup(func() {
		applicationAvailable = previousAvailable
		openWebLoginBrowser = previousOpen
	})

	for mask := 0; mask < 16; mask++ {
		for _, command := range []struct {
			name string
			args []string
		}{
			{name: "config_set", args: []string{"config", "set", "browser", "safari"}},
			{name: "web_login", args: []string{"web", "login", "--browser", "safari"}},
		} {
			t.Run(fmt.Sprintf("mask_%04b/%s", mask, command.name), func(t *testing.T) {
				t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
				writeSavedConfigForTest(t, map[string]any{
					"browser":      "chrome",
					"browser_app":  "Saved App",
					"url_contains": "saved.example",
					"api_base_url": "https://saved.example",
					"future":       true,
				})
				setRuntimeSettingMask(t, mask)
				code, _, stderr := runForTest(t, command.args, "")
				if code != 0 {
					t.Fatalf("code=%d stderr=%q", code, stderr)
				}
				doc := readSavedConfigForTest(t)
				if doc["browser"] != "safari" || doc["browser_app"] != "" || doc["url_contains"] != "saved.example" || doc["api_base_url"] != "https://saved.example" {
					t.Fatalf("saved config = %#v", doc)
				}
				if _, ok := doc["future"]; !ok {
					t.Fatal("CLI update discarded an unknown field")
				}
			})
		}
	}
}

func TestRunConfigShowSavedDistinguishesAbsentValues(t *testing.T) {
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	writeSavedConfigForTest(t, map[string]any{
		"browser": "safari",
		"future":  "private-shape",
		"api_headers": map[string]string{
			"Authorization": "Bearer secret",
		},
	})
	t.Setenv(config.EnvBrowser, "chrome")

	code, stdout, stderr := runForTest(t, []string{"config", "show", "--saved", "--json"}, "")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, `"browser": "safari"`) || !strings.Contains(stdout, `"Authorization": "[redacted]"`) {
		t.Fatalf("saved output = %s", stdout)
	}
	for _, forbidden := range []string{"future", "private-shape", "api_base_url", "Bearer secret"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("saved output contains %q: %s", forbidden, stdout)
		}
	}

	code, stdout, stderr = runForTest(t, []string{"config", "show", "--saved"}, "")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "api_base_url: (absent)") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestRunConfigShowSavedRequiresAFile(t *testing.T) {
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "missing.json"))
	code, _, stderr := runForTest(t, []string{"config", "show", "--saved"}, "")
	if code != 1 || !strings.Contains(stderr, "config init`") || strings.Contains(stderr, "config init --force") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestRunConfigInitIsExplicitRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, path)
	if err := os.WriteFile(path, []byte("{broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runForTest(t, []string{"config", "init"}, "")
	if code != 1 || !strings.Contains(stderr, "already exists") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "{broken\n" {
		t.Fatal("ordinary init replaced malformed config")
	}

	code, _, stderr = runForTest(t, []string{"config", "init", "--force"}, "")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if _, err := config.Load(); err != nil {
		t.Fatalf("forced init did not recover config: %v", err)
	}
}

func TestConfigBootstrapErrorsFailClosedButRecoveryCommandsWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, path)
	if err := os.WriteFile(path, []byte("{broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runForTest(t, []string{"now", "--json"}, "")
	if code != 1 || !strings.Contains(stderr, "failed to load config") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	code, stdout, stderr := runForTest(t, []string{"config", "path"}, "")
	if code != 0 || strings.TrimSpace(stdout) != path || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, _, stderr = runForTest(t, []string{"config", "show", "--saved"}, "")
	if code != 1 || !strings.Contains(stderr, "config init --force") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestConfigConsumingCommandsRejectInvalidConfig(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte("{broken\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertConfigConsumingCommandsRejectPath(t, path)
	})
	t.Run("read failure", func(t *testing.T) {
		assertConfigConsumingCommandsRejectPath(t, t.TempDir())
	})
}

func assertConfigConsumingCommandsRejectPath(t *testing.T, path string) {
	t.Helper()
	t.Setenv(config.EnvConfigPath, path)
	for _, args := range [][]string{
		{"config", "show"},
		{"config", "set", "browser", "safari"},
	} {
		code, stdout, stderr := runForTest(t, args, "")
		if code != 1 || stdout != "" || !strings.Contains(stderr, "failed to load config") {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
}

func TestRunWebLoginPersistsOnlyExplicitBrowserFlags(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantBrowser    string
		wantBrowserApp string
	}{
		{name: "no flags", args: nil, wantBrowser: "chrome", wantBrowserApp: "Saved App"},
		{name: "browser clears app", args: []string{"--browser", "safari"}, wantBrowser: "safari", wantBrowserApp: ""},
		{name: "browser and app", args: []string{"--browser", "custom", "--browser-app", "Custom App"}, wantBrowser: "custom", wantBrowserApp: "Custom App"},
		{name: "app only", args: []string{"--browser-app", "Other App"}, wantBrowser: "chrome", wantBrowserApp: "Other App"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
			writeSavedConfigForTest(t, map[string]any{
				"browser":      "chrome",
				"browser_app":  "Saved App",
				"url_contains": "saved.example",
				"api_base_url": "https://saved.example",
				"future":       true,
			})
			t.Setenv(config.EnvBrowser, "temporary")
			t.Setenv(config.EnvBrowserApp, "Temporary App")
			t.Setenv(config.EnvURLContains, "temporary.example")
			t.Setenv(config.EnvAPIBaseURL, "https://temporary.example")
			previousAvailable := applicationAvailable
			previousOpen := openWebLoginBrowser
			applicationAvailable = func(string) bool { return true }
			openWebLoginBrowser = func(string, string, ...string) error { return nil }
			t.Cleanup(func() {
				applicationAvailable = previousAvailable
				openWebLoginBrowser = previousOpen
			})

			code, _, stderr := runForTest(t, append([]string{"web", "login"}, tt.args...), "")
			if code != 0 || stderr != "" {
				t.Fatalf("code=%d stderr=%q", code, stderr)
			}
			doc := readSavedConfigForTest(t)
			if doc["browser"] != tt.wantBrowser || doc["browser_app"] != tt.wantBrowserApp || doc["url_contains"] != "saved.example" || doc["api_base_url"] != "https://saved.example" {
				t.Fatalf("saved config = %#v", doc)
			}
			if _, ok := doc["future"]; !ok {
				t.Fatal("web login discarded an unknown field")
			}
		})
	}
}

func TestRunAuthLoginRejectsTemporaryAPIBaseBeforeNetwork(t *testing.T) {
	store := useCommandMemoryStore(t)
	var apiCalls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		apiCalls++
	}))
	defer server.Close()
	configureAPIBaseURLForTest(t, "https://saved.example")
	t.Setenv(config.EnvAPIBaseURL, server.URL)

	code, stdout, stderr := runForTest(t, []string{"auth", "login", "--email", "person@example.com", "--password-stdin", "--json"}, "secret\n")
	if code != 1 || stderr != "" || !strings.Contains(stdout, "API base URL is overridden") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if apiCalls != 0 || store.saves != 0 {
		t.Fatalf("apiCalls=%d saves=%d", apiCalls, store.saves)
	}
}

func TestRunAuthImportRejectsTemporaryAPIBaseBeforeBrowserRead(t *testing.T) {
	reader := &countingBrowserReader{}
	useCommandBrowserReader(t, reader)
	configureAPIBaseURLForTest(t, "https://saved.example")
	t.Setenv(config.EnvAPIBaseURL, "https://temporary.example")

	code, stdout, stderr := runForTest(t, []string{"auth", "import-browser", "--browser", "dia", "--no-input", "--json"}, "")
	if code != 1 || stderr != "" || !strings.Contains(stdout, "API base URL is overridden") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if reader.calls != 0 {
		t.Fatalf("browser reader calls=%d, want 0", reader.calls)
	}
}

func readSavedConfigForTest(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func setRuntimeSettingMask(t *testing.T, mask int) {
	t.Helper()
	overrides := []struct {
		name  string
		value string
	}{
		{config.EnvBrowser, "temporary-browser"},
		{config.EnvBrowserApp, "Temporary App"},
		{config.EnvURLContains, "temporary.example"},
		{config.EnvAPIBaseURL, "https://temporary.example"},
	}
	for index, override := range overrides {
		value := ""
		if mask&(1<<index) != 0 {
			value = override.value
		}
		t.Setenv(override.name, value)
	}
}
