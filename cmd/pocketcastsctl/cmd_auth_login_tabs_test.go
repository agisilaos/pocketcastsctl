package main

import (
	"strings"
	"testing"
)

func TestRunWebLoginRejectsEmptyURL(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"web", "login", "--url", ""}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "web login failed: url cannot be empty") {
		t.Fatalf("stderr missing empty-url error: %q", stderr)
	}
}

func TestRunAuthTabsInvalidBrowserOptions(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"auth", "tabs", "--browser", "chromium"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "invalid browser options") {
		t.Fatalf("stderr missing invalid browser options: %q", stderr)
	}
}
