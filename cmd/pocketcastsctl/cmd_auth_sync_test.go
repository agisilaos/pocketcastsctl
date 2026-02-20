package main

import (
	"strings"
	"testing"
)

func TestRunAuthSyncRejectsEmptyURLContains(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"auth", "sync", "--url-contains", ""}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "invalid browser options") {
		t.Fatalf("stderr missing invalid browser options: %q", stderr)
	}
}
