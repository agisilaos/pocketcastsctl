package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestLocalLifecycleArgumentsNeverActOnHelpOrInvalidInput(t *testing.T) {
	for _, action := range []string{"pause", "resume", "stop"} {
		for _, argument := range []string{"--help", "-h", "--unknown", "extra"} {
			t.Run(action+"/"+argument, func(t *testing.T) {
				t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
				original := []byte(`{"pid":1234}`)
				if err := os.WriteFile(config.StatePath(), original, 0o600); err != nil {
					t.Fatal(err)
				}
				code, stdout, stderr := runForTest(t, []string{"local", action, argument}, "")
				wantCode := 2
				if argument == "--help" || argument == "-h" {
					wantCode = 0
				}
				if code != wantCode || !strings.Contains(strings.ToLower(stdout+stderr), "usage") {
					t.Errorf("code=%d stdout=%q stderr=%q, want usage with exit %d", code, stdout, stderr, wantCode)
				}
				got, err := os.ReadFile(config.StatePath())
				if err != nil || string(got) != string(original) {
					t.Errorf("help/invalid arguments changed playback state: got=%s err=%v", got, err)
				}
			})
		}
	}
}
