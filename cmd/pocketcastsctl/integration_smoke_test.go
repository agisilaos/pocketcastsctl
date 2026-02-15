package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestCLISmokeCoreCommands(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantCode    int
		allowAlt    int
		stdoutParts []string
		stderrParts []string
	}{
		{
			name:        "help",
			args:        []string{"help"},
			wantCode:    0,
			stdoutParts: []string{"Command reference:", "pocketcastsctl now"},
		},
		{
			name:        "now json",
			args:        []string{"now", "--json"},
			wantCode:    0,
			stdoutParts: []string{"\"web\"", "\"auth\"", "\"actions\""},
		},
		{
			name:        "doctor quick",
			args:        []string{"doctor", "--quick"},
			wantCode:    0,
			allowAlt:    1,
			stdoutParts: []string{"doctor status:"},
			stderrParts: []string{"running quick checks"},
		},
		{
			name:        "auth status json",
			args:        []string{"auth", "status", "--json"},
			wantCode:    0,
			stdoutParts: []string{"\"authorization_present\"", "\"config_path\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runForTest(t, tt.args, "")
			if tt.allowAlt != 0 {
				if code != tt.wantCode && code != tt.allowAlt {
					t.Fatalf("exit code = %d, want %d or %d; stderr=%q", code, tt.wantCode, tt.allowAlt, stderr)
				}
			} else if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr=%q", code, tt.wantCode, stderr)
			}
			for _, part := range tt.stdoutParts {
				if !strings.Contains(stdout, part) {
					t.Fatalf("stdout missing %q:\n%s", part, stdout)
				}
			}
			for _, part := range tt.stderrParts {
				if !strings.Contains(stderr, part) {
					t.Fatalf("stderr missing %q:\n%s", part, stderr)
				}
			}
		})
	}
}

func TestCLISmokeQueueAPILSPlainUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/up_next/list" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Unauthorized"))
	}))
	defer srv.Close()

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, cfgPath)
	err := os.WriteFile(cfgPath, []byte(`{
  "browser":"chrome",
  "url_contains":"pocketcasts.com",
  "api_base_url":"`+srv.URL+`",
  "api_headers":{"Authorization":"Bearer test-token"}
}`), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	code, stdout, stderr := runForTest(t, []string{"queue", "api", "ls", "--plain"}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout expected empty on error, got: %q", stdout)
	}
	if !strings.Contains(stderr, "queue api ls failed") {
		t.Fatalf("stderr missing failure line: %q", stderr)
	}
	if !strings.Contains(stderr, "auth refresh") {
		t.Fatalf("stderr missing auth recovery hint: %q", stderr)
	}
}
