package main

import (
	"strings"
	"testing"
)

func TestRunDoctorApplyRequiresFix(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"doctor", "--apply"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--apply requires --fix") {
		t.Fatalf("stderr missing apply/fix requirement: %q", stderr)
	}
}

func TestRunDoctorQuickFixJSONIncludesSuggestedFixes(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"doctor", "--quick", "--fix", "--json"}, "")
	if code != 0 && code != 1 {
		t.Fatalf("exit code = %d, want 0 or 1; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "\"suggested_fixes\"") {
		t.Fatalf("stdout missing suggested_fixes: %q", stdout)
	}
}
