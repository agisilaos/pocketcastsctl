package main

import (
	"strings"
	"testing"
)

func TestRunNowRejectsNonPositiveInterval(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"now", "--interval", "0s"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--interval must be > 0") {
		t.Fatalf("stderr missing interval validation: %q", stderr)
	}
}

func TestRunSetupRejectsJSONAndPlainTogether(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"setup", "--json", "--plain"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "use only one of --json or --plain") {
		t.Fatalf("stderr missing setup output-mode validation: %q", stderr)
	}
}

func TestRunHARHelpFlag(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"har", "--help"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "pocketcastsctl har summarize") {
		t.Fatalf("stdout missing har summarize usage: %q", stdout)
	}
}
