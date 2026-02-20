package main

import (
	"strings"
	"testing"
)

func TestRunQueueAPIAddMissingUUID(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"queue", "api", "add"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "missing episode uuid") {
		t.Fatalf("stderr missing missing-uuid message: %q", stderr)
	}
}

func TestRunQueueAPIAddInvalidEpisodeJSON(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"queue", "api", "add", "--episode-json", "{"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "invalid --episode-json") {
		t.Fatalf("stderr missing invalid episode json message: %q", stderr)
	}
}

func TestRunQueueAPIPlayRequiresSelector(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"queue", "api", "play"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "usage: pocketcastsctl queue api play <index|uuid>") {
		t.Fatalf("stderr missing usage for queue api play: %q", stderr)
	}
}

func TestRunQueueAPIPickRejectsConflictingFilters(t *testing.T) {
	code, _, stderr := runForTest(t, []string{"queue", "api", "pick", "--unplayed", "--in-progress"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "use only one of --unplayed or --in-progress") {
		t.Fatalf("stderr missing conflicting filter message: %q", stderr)
	}
}
