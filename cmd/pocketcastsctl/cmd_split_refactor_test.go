package main

import "testing"

func TestRunQueueUnknownSubcommand(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"queue", "nope"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stderr == "" {
		t.Fatalf("stderr should not be empty")
	}
}

func TestRunLocalUnknownSubcommand(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"local", "nope"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stderr == "" {
		t.Fatalf("stderr should not be empty")
	}
}
