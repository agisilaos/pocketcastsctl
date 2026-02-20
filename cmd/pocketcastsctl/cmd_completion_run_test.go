package main

import (
	"strings"
	"testing"
)

func TestRunCompletionUnknownShell(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"completion", "powershell"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown shell: powershell") {
		t.Fatalf("stderr missing unknown shell message: %q", stderr)
	}
}

func TestRunCompletionBashWritesScript(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"completion", "bash"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "_pocketcastsctl_completions") {
		t.Fatalf("stdout missing bash completion function: %q", stdout)
	}
}
