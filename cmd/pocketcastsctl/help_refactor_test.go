package main

import (
	"strings"
	"testing"
)

func TestUsageRegistryContainsCriticalTopics(t *testing.T) {
	critical := []string{
		"config init",
		"auth refresh",
		"web status",
		"queue api rm",
		"doctor explain",
	}
	for _, k := range critical {
		if _, ok := usageText[k]; !ok {
			t.Fatalf("usageText missing key %q", k)
		}
	}
}

func TestRunHelpQueueAPIRmIncludesUsage(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"help", "queue", "api", "rm"}, "")
	if code != 0 {
		t.Fatalf("code = %d, stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Fatalf("stdout missing Usage header: %q", stdout)
	}
	if !strings.Contains(stdout, usageText["queue api rm"]) {
		t.Fatalf("stdout missing usage line for queue api rm: %q", stdout)
	}
}

func TestWebStatusHelpIncludesDetails(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"help", "web", "status"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "--details") {
		t.Fatalf("stdout missing --details: %q", stdout)
	}
}
