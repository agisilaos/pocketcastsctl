package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadResolvesSavedDefaultsAndEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(EnvConfigPath, path)
	writeRawConfig(t, `{
  "browser": "safari",
  "browser_app": "Safari",
  "url_contains": "play.pocketcasts.com",
  "api_base_url": "https://api.example.com",
  "api_headers": {"Authorization":"Bearer token"},
  "future": {"enabled":true}
}`)
	t.Setenv(EnvBrowser, "chrome")
	t.Setenv(EnvBrowserApp, "Google Chrome")
	t.Setenv(EnvURLContains, "pocketcasts.com")
	t.Setenv(EnvAPIBaseURL, "https://api.pocketcasts.com")

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Browser != "chrome" || got.BrowserApp != "Google Chrome" || got.URLContains != "pocketcasts.com" || got.APIBaseURL != "https://api.pocketcasts.com" {
		t.Fatalf("environment was not resolved: %#v", got)
	}
	if got.APIHeaders["Authorization"] != "Bearer token" {
		t.Fatalf("saved headers were not retained: %#v", got.APIHeaders)
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

func TestLoadReturnsNoRuntimeConfigAfterReadOrParseFailure(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte("{broken\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertLoadFailureReturnsNoRuntimeConfig(t, path)
	})
	t.Run("read failure", func(t *testing.T) {
		assertLoadFailureReturnsNoRuntimeConfig(t, t.TempDir())
	})
}

func assertLoadFailureReturnsNoRuntimeConfig(t *testing.T, path string) {
	t.Helper()
	t.Setenv(EnvConfigPath, path)
	t.Setenv(EnvAPIBaseURL, "https://must-not-be-selected.example")
	cfg, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if cfg.Browser != "" || cfg.BrowserApp != "" || cfg.URLContains != "" || cfg.APIBaseURL != "" || cfg.APIHeaders != nil || cfg.Auth != (AuthConfig{}) {
		t.Fatalf("Load() returned partial runtime config after failure: %#v", cfg)
	}
}

func TestInitRequiresForceAndLoadSavedPreservesAbsence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(EnvConfigPath, path)
	writeRawConfig(t, `{"browser":"safari","future":"keep"}`)

	if err := Init(false); !errors.Is(err, ErrConfigExists) {
		t.Fatalf("Init(false) error = %v, want ErrConfigExists", err)
	}
	saved, err := LoadSaved()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Browser == nil || *saved.Browser != "safari" || saved.APIBaseURL != nil {
		t.Fatalf("saved known fields = %#v", saved)
	}
	b, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "future") || strings.Contains(string(b), "api_base_url") {
		t.Fatalf("saved view exposed unknown or absent fields: %s", b)
	}

	if err := Init(true); err != nil {
		t.Fatal(err)
	}
	doc := readRawConfig(t)
	if doc["browser"] != "chrome" || doc["api_base_url"] != "https://api.pocketcasts.com" {
		t.Fatalf("forced canonical config = %#v", doc)
	}
	if _, ok := doc["future"]; ok {
		t.Fatal("forced init preserved an unknown field")
	}
}

func TestFocusedUpdatesAcrossEveryRuntimeSettingCombination(t *testing.T) {
	overrides := []struct {
		env   string
		value string
	}{
		{EnvBrowser, "override-browser"},
		{EnvBrowserApp, "Override Browser App"},
		{EnvURLContains, "override.example"},
		{EnvAPIBaseURL, "https://override.example"},
	}

	for mask := 0; mask < 1<<len(overrides); mask++ {
		t.Run(fmt.Sprintf("mask_%04b", mask), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			t.Setenv(EnvConfigPath, path)
			writeRawConfig(t, `{
  "browser":"safari",
  "url_contains":"saved.example",
  "api_base_url":"https://saved.example",
  "api_headers":{"Authorization":"Bearer legacy","X-Future":"keep"},
  "future":{"nested":[1,2,3]}
}`)
			for index, override := range overrides {
				value := ""
				if mask&(1<<index) != 0 {
					value = override.value
				}
				t.Setenv(override.env, value)
			}

			browser := "dia"
			emptyApp := ""
			if _, err := UpdateBrowser(BrowserUpdate{Browser: &browser, BrowserApp: &emptyApp}); err != nil {
				t.Fatal(err)
			}
			doc := readRawConfig(t)
			if doc["browser"] != "dia" || doc["browser_app"] != "" || doc["url_contains"] != "saved.example" || doc["api_base_url"] != "https://saved.example" {
				t.Fatalf("browser update persisted an effective setting: %#v", doc)
			}
			if _, ok := doc["future"]; !ok {
				t.Fatal("browser update discarded an unknown field")
			}

			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			effective, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			_, err = UpdateAuth(effective.APIBaseURL, AuthConfig{SessionKey: "new-session", Method: "password"})
			if mask&(1<<3) != 0 {
				if !errors.Is(err, ErrAPIBaseURLOverride) {
					t.Fatalf("UpdateAuth error = %v, want ErrAPIBaseURLOverride", err)
				}
				after, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if string(after) != string(before) {
					t.Fatal("issuer refusal changed the config file")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			doc = readRawConfig(t)
			if doc["url_contains"] != "saved.example" || doc["api_base_url"] != "https://saved.example" {
				t.Fatalf("auth update persisted runtime settings: %#v", doc)
			}
			headers := doc["api_headers"].(map[string]any)
			if headers["X-Future"] != "keep" {
				t.Fatalf("auth update discarded headers: %#v", headers)
			}
			if _, ok := headers["Authorization"]; ok {
				t.Fatal("auth update retained legacy Authorization")
			}
			if _, ok := doc["future"]; !ok {
				t.Fatal("auth update discarded an unknown field")
			}
		})
	}
}

func TestAuthUpdatePreservesExistingOmissionsAndInitializesMissingFile(t *testing.T) {
	t.Run("existing omissions", func(t *testing.T) {
		t.Setenv(EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
		writeRawConfig(t, `{"future":true}`)
		if _, err := UpdateAuth(Default().APIBaseURL, AuthConfig{SessionKey: "saved"}); err != nil {
			t.Fatal(err)
		}
		doc := readRawConfig(t)
		if _, ok := doc["browser"]; ok {
			t.Fatal("auth update materialized an omitted browser")
		}
		if doc["future"] != true {
			t.Fatal("auth update changed an unknown value")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "config.json")
		t.Setenv(EnvConfigPath, path)
		if _, err := UpdateAuth(Default().APIBaseURL, AuthConfig{SessionKey: "saved"}); err != nil {
			t.Fatal(err)
		}
		doc := readRawConfig(t)
		if doc["browser"] != "chrome" || doc["api_base_url"] != Default().APIBaseURL {
			t.Fatalf("missing-file update did not create canonical defaults: %#v", doc)
		}
	})
}

func TestUnknownAuthFieldsFailClosedExceptLogout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(EnvConfigPath, path)
	writeRawConfig(t, `{
  "api_base_url":"https://api.pocketcasts.com",
  "auth":{"session_key":"old","issuer":"future"},
  "future":"keep"
}`)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAuthUpdate(Default().APIBaseURL); !errors.Is(err, ErrUnknownAuthFields) {
		t.Fatalf("ValidateAuthUpdate error = %v, want ErrUnknownAuthFields", err)
	}
	if _, err := UpdateAuth(Default().APIBaseURL, AuthConfig{SessionKey: "new"}); !errors.Is(err, ErrUnknownAuthFields) {
		t.Fatalf("UpdateAuth error = %v, want ErrUnknownAuthFields", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("failed auth update changed the file")
	}
	if _, err := ClearAuth(); err != nil {
		t.Fatal(err)
	}
	doc := readRawConfig(t)
	if _, ok := doc["auth"]; ok {
		t.Fatal("logout retained the unfamiliar auth object")
	}
	if doc["future"] != "keep" {
		t.Fatal("logout discarded an unrelated unknown field")
	}
}

func TestMalformedConfigIsNeverOverwrittenByFocusedUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(EnvConfigPath, path)
	writeRawConfig(t, `{not-json`)
	browser := "safari"
	if _, err := UpdateBrowser(BrowserUpdate{Browser: &browser}); err == nil {
		t.Fatal("UpdateBrowser accepted malformed config")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{not-json\n" {
		t.Fatalf("malformed config was replaced: %q", b)
	}
}

func TestAtomicWritePermissionsFailureAndDirectorySync(t *testing.T) {
	t.Run("private and synced", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "config.json")
		t.Setenv(EnvConfigPath, path)
		originalSync := syncConfigDir
		syncCalls := 0
		syncConfigDir = func(dir string) error {
			syncCalls++
			return originalSync(dir)
		}
		t.Cleanup(func() { syncConfigDir = originalSync })
		if err := Init(false); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 || syncCalls != 1 {
			t.Fatalf("mode=%o syncCalls=%d", info.Mode().Perm(), syncCalls)
		}
		assertNoConfigTemps(t, path)
	})

	t.Run("rename failure preserves old file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		t.Setenv(EnvConfigPath, path)
		writeRawConfig(t, `{"browser":"safari"}`)
		before, _ := os.ReadFile(path)
		originalRename := renameConfigFile
		renameConfigFile = func(string, string) error { return errors.New("rename failed") }
		t.Cleanup(func() { renameConfigFile = originalRename })
		browser := "dia"
		if _, err := UpdateBrowser(BrowserUpdate{Browser: &browser}); err == nil {
			t.Fatal("UpdateBrowser succeeded despite rename failure")
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(before) {
			t.Fatal("rename failure replaced the old file")
		}
		assertNoConfigTemps(t, path)
	})

	t.Run("sync failure reports applied update", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		t.Setenv(EnvConfigPath, path)
		writeRawConfig(t, `{"browser":"safari"}`)
		originalSync := syncConfigDir
		syncConfigDir = func(string) error { return errors.New("sync failed") }
		t.Cleanup(func() { syncConfigDir = originalSync })
		browser := "dia"
		cfg, err := UpdateBrowser(BrowserUpdate{Browser: &browser})
		if !errors.Is(err, ErrDurabilityUncertain) || cfg.Browser != "dia" {
			t.Fatalf("cfg=%#v error=%v", cfg, err)
		}
		if readRawConfig(t)["browser"] != "dia" {
			t.Fatal("sync failure did not leave the renamed file visible")
		}
		assertNoConfigTemps(t, path)
	})
}

func writeRawConfig(t *testing.T, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(contents+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readRawConfig(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func assertNoConfigTemps(t *testing.T, path string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}
