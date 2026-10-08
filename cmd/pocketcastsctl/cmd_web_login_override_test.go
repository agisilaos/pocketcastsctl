package main

import (
	"path/filepath"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestWebLoginLaunchesExplicitBrowserOverInheritedApp(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		app  string
	}{
		{"browser replaces inherited app", []string{"--browser", "chrome"}, "Google Chrome"},
		{"explicit app wins", []string{"--browser", "chrome", "--browser-app", "Custom Chrome"}, "Custom Chrome"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
			t.Setenv(config.EnvBrowser, "")
			t.Setenv(config.EnvBrowserApp, "")
			writeSavedConfigForTest(t, map[string]any{"browser": "safari", "browser_app": "Safari"})
			previousAvailable, previousOpen := applicationAvailable, openWebLoginBrowser
			applicationAvailable = func(string) bool { return true }
			opened := ""
			openWebLoginBrowser = func(app, url string, args ...string) error {
				opened = app
				return nil
			}
			t.Cleanup(func() { applicationAvailable, openWebLoginBrowser = previousAvailable, previousOpen })
			code, stdout, stderr := runForTest(t, append([]string{"web", "login"}, tc.args...), "")
			if code != 0 || opened != tc.app {
				t.Fatalf("code=%d opened=%q want=%q stdout=%q stderr=%q", code, opened, tc.app, stdout, stderr)
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			if target := newBrowserTarget(cfg.Browser, cfg.BrowserApp, cfg.URLContains); target.applicationName() != opened {
				t.Fatalf("saved application %q differs from launched %q", target.applicationName(), opened)
			}
		})
	}
}
