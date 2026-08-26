package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pocketcastsctl/internal/pocketcasts"
)

func TestPickEpisodeInteractiveSelectsWithFZF(t *testing.T) {
	installFakeFZF(t, "#!/bin/sh\nprintf ' 2  Second episode  (second-u)\\n'\n")

	chosen, err, stdout, stderr := runPickerForTest(t, pickerTestEpisodes(), "")
	if err != nil {
		t.Fatalf("pickEpisodeInteractive() error = %v", err)
	}
	if chosen.UUID != "second-uuid" {
		t.Fatalf("pickEpisodeInteractive() UUID = %q, want %q", chosen.UUID, "second-uuid")
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("unexpected prompt fallback: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestPickEpisodeInteractiveFZFCancellationDoesNotFallBack(t *testing.T) {
	installFakeFZF(t, "#!/bin/sh\nexit 130\n")

	_, err, stdout, stderr := runPickerForTest(t, pickerTestEpisodes(), "2\n")
	if !errors.Is(err, errPickerCanceled) {
		t.Fatalf("pickEpisodeInteractive() error = %v, want errPickerCanceled", err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("fzf cancellation opened prompt fallback: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestPickEpisodeInteractiveMissingFZFFallsBackToPrompt(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	chosen, err, stdout, stderr := runPickerForTest(t, pickerTestEpisodes(), "2\n")
	assertPromptSelection(t, chosen, err, stdout, stderr)
}

func TestPickEpisodeInteractiveFZFFailureFallsBackToPrompt(t *testing.T) {
	installFakeFZF(t, "#!/bin/sh\nexit 2\n")

	chosen, err, stdout, stderr := runPickerForTest(t, pickerTestEpisodes(), "2\n")
	assertPromptSelection(t, chosen, err, stdout, stderr)
}

func TestPickEpisodeInteractiveMalformedFZFOutputFallsBackToPrompt(t *testing.T) {
	installFakeFZF(t, "#!/bin/sh\nprintf 'not-an-episode\\n'\n")

	chosen, err, stdout, stderr := runPickerForTest(t, pickerTestEpisodes(), "2\n")
	assertPromptSelection(t, chosen, err, stdout, stderr)
}

func TestPickEpisodeInteractiveAcceptsNoninteractiveInputWithoutNewline(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	chosen, err, _, _ := runPickerForTest(t, pickerTestEpisodes(), "2")
	if err != nil {
		t.Fatalf("pickEpisodeInteractive() error = %v", err)
	}
	if chosen.UUID != "second-uuid" {
		t.Fatalf("pickEpisodeInteractive() UUID = %q, want %q", chosen.UUID, "second-uuid")
	}
}

func TestPickEpisodeInteractivePromptEOFCancels(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err, _, stderr := runPickerForTest(t, pickerTestEpisodes(), "")
	if !errors.Is(err, errPickerCanceled) {
		t.Fatalf("pickEpisodeInteractive() error = %v, want errPickerCanceled", err)
	}
	if !strings.Contains(stderr, "Pick number (or blank to cancel):") {
		t.Fatalf("stderr = %q, want prompt", stderr)
	}
}

func installFakeFZF(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fzf")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake fzf: %v", err)
	}
	t.Setenv("PATH", dir)
}

func runPickerForTest(t *testing.T, episodes []pocketcasts.UpNextEpisode, stdin string) (pocketcasts.UpNextEpisode, error, string, string) {
	t.Helper()
	var chosen pocketcasts.UpNextEpisode
	var pickErr error
	_, stdout, stderr := runForTestWithRunner(t, nil, stdin, func([]string) int {
		chosen, pickErr = pickEpisodeInteractive(episodes)
		return 0
	})
	return chosen, pickErr, stdout, stderr
}

func assertPromptSelection(t *testing.T, chosen pocketcasts.UpNextEpisode, err error, stdout, stderr string) {
	t.Helper()
	if err != nil {
		t.Fatalf("pickEpisodeInteractive() error = %v", err)
	}
	if chosen.UUID != "second-uuid" {
		t.Fatalf("pickEpisodeInteractive() UUID = %q, want %q", chosen.UUID, "second-uuid")
	}
	if !strings.Contains(stdout, " 2. Second episode") {
		t.Fatalf("stdout = %q, want numbered prompt options", stdout)
	}
	if !strings.Contains(stderr, "Pick number (or blank to cancel):") {
		t.Fatalf("stderr = %q, want prompt", stderr)
	}
}

func pickerTestEpisodes() []pocketcasts.UpNextEpisode {
	return []pocketcasts.UpNextEpisode{
		{UUID: "first-uuid", Title: "First episode"},
		{UUID: "second-uuid", Title: "Second episode"},
	}
}
