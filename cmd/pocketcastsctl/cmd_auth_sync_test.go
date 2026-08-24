package main

import (
	"strings"
	"testing"
)

func TestRunAuthSyncDryRunCannotPersistPlaintextCredential(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"auth", "sync", "--dry-run", "--json"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "auth sync` is deprecated") {
		t.Fatalf("stderr missing deprecation warning: %q", stderr)
	}
	if !strings.Contains(stdout, `"code": "auth.sync.dry_run_removed"`) {
		t.Fatalf("stdout missing structured migration error: %q", stdout)
	}
}
