package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const richWebPlaybackSnapshotJSON = `{"state":"playing","episode_title":"Episode 7","podcast_title":"The Podcast","position_seconds":754,"duration_seconds":2700,"progress_percent":27.9}`

func setupWebStatusFakeOsa(t *testing.T, output string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "osascript")
	script := "#!/bin/sh\nprintf '%s' \"$OSASCRIPT_OUT\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake osascript: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OSASCRIPT_OUT", output)
}

func TestWebStatusDetailsRendersHumanSnapshot(t *testing.T) {
	setupWebStatusFakeOsa(t, richWebPlaybackSnapshotJSON)

	code, stdout, stderr := runForTest(t, []string{"web", "status", "--details"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	want := strings.Join([]string{
		"State   : PLAYING",
		"Episode : Episode 7",
		"Podcast : The Podcast",
		"Progress: 12:34 / 45:00 (27.9%)",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("unexpected details output\n--- want ---\n%s--- got ---\n%s", want, stdout)
	}
}

func TestWebStatusDefaultOutputRemainsStateOnly(t *testing.T) {
	setupWebStatusFakeOsa(t, richWebPlaybackSnapshotJSON)

	for _, args := range [][]string{
		{"web", "status"},
		{"web", "status", "--plain"},
	} {
		code, stdout, stderr := runForTest(t, args, "")
		if code != 0 {
			t.Fatalf("%v exit code = %d, want 0; stderr=%q", args, code, stderr)
		}
		if stdout != "playing\n" {
			t.Fatalf("%v output = %q, want state token only", args, stdout)
		}
	}
}

func TestWebStatusDetailsRendersPlainSnapshot(t *testing.T) {
	setupWebStatusFakeOsa(t, richWebPlaybackSnapshotJSON)

	code, stdout, stderr := runForTest(t, []string{"web", "status", "--details", "--plain"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	want := strings.Join([]string{
		"state\tplaying",
		"episode_title\tEpisode 7",
		"podcast_title\tThe Podcast",
		"position_seconds\t754",
		"duration_seconds\t2700",
		"progress_percent\t27.9",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("unexpected plain details output\n--- want ---\n%s--- got ---\n%s", want, stdout)
	}
}

func TestWebStatusJSONIncludesPlaybackSnapshot(t *testing.T) {
	setupWebStatusFakeOsa(t, richWebPlaybackSnapshotJSON)

	code, stdout, stderr := runForTest(t, []string{"web", "status", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	want := map[string]any{
		"state":            "playing",
		"episode_title":    "Episode 7",
		"podcast_title":    "The Podcast",
		"position_seconds": float64(754),
		"duration_seconds": float64(2700),
		"progress_percent": float64(27.9),
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %#v, want %#v; payload=%#v", key, got[key], value, got)
		}
	}
}

func TestWebStatusPartialSnapshotPreservesState(t *testing.T) {
	setupWebStatusFakeOsa(t, `{"state":"paused"}`)

	code, stdout, stderr := runForTest(t, []string{"web", "status", "--json"}, "")
	if code != 0 {
		t.Fatalf("JSON exit code = %d, want 0; stderr=%q", code, stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(payload) != 1 || payload["state"] != "paused" {
		t.Fatalf("partial JSON must omit unavailable details: %#v", payload)
	}

	code, stdout, stderr = runForTest(t, []string{"web", "status", "--details"}, "")
	if code != 0 {
		t.Fatalf("details exit code = %d, want 0; stderr=%q", code, stderr)
	}
	for _, want := range []string{
		"State   : PAUSED\n",
		"Episode : unknown\n",
		"Podcast : unknown\n",
		"Progress: unknown / unknown (unknown)\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("partial details missing %q:\n%s", want, stdout)
		}
	}

	code, stdout, stderr = runForTest(t, []string{"web", "status", "--details", "--plain"}, "")
	if code != 0 {
		t.Fatalf("plain details exit code = %d, want 0; stderr=%q", code, stderr)
	}
	for _, want := range []string{
		"state\tpaused\n",
		"episode_title\tunknown\n",
		"podcast_title\tunknown\n",
		"position_seconds\tunknown\n",
		"duration_seconds\tunknown\n",
		"progress_percent\tunknown\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("partial plain details missing %q:\n%s", want, stdout)
		}
	}
}

func TestNowJSONIncludesWebPlaybackSnapshot(t *testing.T) {
	setupWebStatusFakeOsa(t, richWebPlaybackSnapshotJSON)

	code, stdout, stderr := runForTest(t, []string{"now", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	web, ok := payload["web"].(map[string]any)
	if !ok {
		t.Fatalf("missing web snapshot: %#v", payload)
	}
	if web["status"] != "playing" || web["episode_title"] != "Episode 7" || web["podcast_title"] != "The Podcast" {
		t.Fatalf("unexpected web identity: %#v", web)
	}
	if web["position_seconds"] != float64(754) || web["duration_seconds"] != float64(2700) || web["progress_percent"] != float64(27.9) {
		t.Fatalf("unexpected web timing: %#v", web)
	}
}

func TestNowHumanIncludesWebPlaybackSnapshot(t *testing.T) {
	setupWebStatusFakeOsa(t, richWebPlaybackSnapshotJSON)

	code, stdout, stderr := runForTest(t, []string{"now"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	for _, want := range []string{
		"Web    : PLAYING\n",
		"Episode : Episode 7\n",
		"Podcast : The Podcast\n",
		"Progress: 12:34 / 45:00 (27.9%)\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestNowPlainIncludesWebPlaybackSnapshot(t *testing.T) {
	setupWebStatusFakeOsa(t, richWebPlaybackSnapshotJSON)

	code, stdout, stderr := runForTest(t, []string{"now", "--plain"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	for _, want := range []string{
		"web_episode_title\tEpisode 7\n",
		"web_podcast_title\tThe Podcast\n",
		"web_position_seconds\t754\n",
		"web_duration_seconds\t2700\n",
		"web_progress_percent\t27.9\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestWebDetailsRejectsPlaybackActions(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"web", "play", "--details"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "--details is only supported by web status") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}
