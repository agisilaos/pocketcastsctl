package main

import (
	"path/filepath"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

func setupWebActionFakeOsa(t *testing.T, output string) {
	t.Helper()
	setupWebStatusFakeOsa(t, output)
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	t.Setenv(config.EnvBrowser, "chrome")
	t.Setenv(config.EnvBrowserApp, "")
	t.Setenv(config.EnvURLContains, "pocketcasts.com")
	previous := applicationAvailable
	applicationAvailable = func(string) bool { return true }
	t.Cleanup(func() { applicationAvailable = previous })
}

func TestWebActionsPreserveControlLabelOutput(t *testing.T) {
	for _, tt := range []struct {
		action string
		label  string
	}{
		{"play", "Resume"},
		{"pause", "Pause episode"},
		{"toggle", "Play episode"},
		{"next", "Skip forward"},
		{"prev", "Skip back"},
	} {
		t.Run(tt.action, func(t *testing.T) {
			setupWebActionFakeOsa(t, `{"clicked":true,"clickedLabel":"`+tt.label+`"}`)
			code, stdout, stderr := runForTest(t, []string{"web", tt.action}, "")
			if code != 0 || stdout != tt.label+"\n" || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestWebActionFailuresDoNotPrintSuccess(t *testing.T) {
	for _, tt := range []struct {
		name, output, want string
	}{
		{"missing control", `{"clicked":false,"clickedLabel":""}`, "no matching control found"},
		{"malformed result", `{"clicked":true}`, "unexpected JS result"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setupWebActionFakeOsa(t, tt.output)
			code, stdout, stderr := runForTest(t, []string{"web", "play"}, "")
			if code != 1 || stdout != "" || !strings.Contains(stderr, tt.want) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}
