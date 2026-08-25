package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestLocalStatusMigratesLegacyStateAndKeepsJSONContract(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, configPath)
	statePath := config.StatePath()
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte(`{"pid":1234,"command":["mpv","https://secret.test/audio"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runForTest(t, []string{"local", "status", "--json"}, "")
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload["status"] != "stopped" {
		t.Fatalf("payload = %v, want stopped", payload)
	}
	if !strings.Contains(stderr, "discarded legacy local playback state") || strings.Contains(stderr, "secret.test") {
		t.Fatalf("stderr warning missing or leaked state: %q", stderr)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy state still exists: %v", err)
	}
}

func TestLocalStatusRetainsFutureStateAndFails(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, configPath)
	statePath := config.StatePath()
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":2,"process":{"pid":1,"birth_unix_micros":1}}`)
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := runForTest(t, []string{"local", "status", "--json"}, "")
	if code != 1 || !strings.Contains(stderr, "incompatible local playback state") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("future state changed: %s", got)
	}
}
