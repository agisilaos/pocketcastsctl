package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestCollectLocalStatusUsesLifecycleMigrationWarning(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, configPath)
	statePath := config.StatePath()
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte(`{"pid":1234}`), 0o600); err != nil {
		t.Fatal(err)
	}

	status, warnings := collectLocalStatus(context.Background())
	if status.Status != "stopped" || len(warnings) != 1 || !strings.Contains(warnings[0], "discarded legacy") {
		t.Fatalf("collectLocalStatus() = %+v, %v", status, warnings)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy state still exists: %v", err)
	}
}
