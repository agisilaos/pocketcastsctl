package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	_, ok, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if ok {
		t.Fatalf("Load() ok = true, want false for missing file")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "playback.json")
	want := PlaybackState{
		PID:         1234,
		Command:     []string{"mpv", "https://example.test/a.mp3"},
		EpisodeUUID: "ep-1",
		Title:       "Episode 1",
		StartedAt:   time.Unix(1_735_689_600, 0).UTC(),
		Paused:      true,
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, ok, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatalf("Load() ok = false, want true")
	}
	if got.PID != want.PID || got.EpisodeUUID != want.EpisodeUUID || got.Title != want.Title || got.Paused != want.Paused {
		t.Fatalf("Load() mismatch: got %+v want %+v", got, want)
	}
	if got.StartedAt.Unix() != want.StartedAt.Unix() {
		t.Fatalf("StartedAt = %v, want %v", got.StartedAt, want.StartedAt)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err := Load(path)
	if err == nil {
		t.Fatalf("Load() expected error for invalid JSON")
	}
}

func TestClearExistingAndMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clear.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := Clear(path); err != nil {
		t.Fatalf("Clear(existing) error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, stat err = %v", err)
	}
	if err := Clear(path); err != nil {
		t.Fatalf("Clear(missing) error = %v", err)
	}
}
