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

const smokeEpisodeUUID = "11111111-1111-1111-1111-111111111111"

func writeSmokeConfig(t *testing.T, baseURL string) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, cfgPath)
	err := os.WriteFile(cfgPath, []byte(`{
  "browser":"chrome",
  "url_contains":"pocketcasts.com",
  "api_base_url":"`+baseURL+`",
  "api_headers":{"Authorization":"Bearer test-token"}
}`), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}
}

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

	writeSmokeConfig(t, srv.URL)

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
	if !strings.Contains(stderr, "auth login") {
		t.Fatalf("stderr missing auth recovery hint: %q", stderr)
	}
}

func TestCLISmokeQueueAPIPlayDryRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/up_next/list" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"episodes":[{"uuid":"` + smokeEpisodeUUID + `","title":"Smoke Episode","url":"https://example.test/audio.mp3"}]}`))
	}))
	defer srv.Close()
	writeSmokeConfig(t, srv.URL)

	code, stdout, stderr := runForTest(t, []string{"queue", "api", "play", "--dry-run", "1"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "dry-run: would play in web player: Smoke Episode") {
		t.Fatalf("stdout missing dry-run summary: %q", stdout)
	}
}

func TestCLISmokeQueueAPIMovePreservesRepeatedEpisodeOccurrence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/up_next/list" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"episodes":[
  {"uuid":"` + testEpisodeA + `","title":"First A"},
  {"uuid":"` + testEpisodeB + `","title":"B"},
  {"uuid":"` + testEpisodeA + `","title":"Second A"}
]}`))
	}))
	defer srv.Close()
	writeSmokeConfig(t, srv.URL)

	code, stdout, stderr := runForTest(t, []string{"queue", "api", "move", "--dry-run", "--json", "3", "1"}, "")
	if code != 0 {
		t.Fatalf("numeric move exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, `"from_index": 3`) || !strings.Contains(stdout, `"title": "Second A"`) {
		t.Fatalf("numeric move selected wrong occurrence: %s", stdout)
	}

	code, stdout, stderr = runForTest(t, []string{"queue", "api", "move", "--dry-run", "--json", testEpisodeA, "3"}, "")
	if code != 0 {
		t.Fatalf("UUID move exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, `"from_index": 1`) || !strings.Contains(stdout, `"title": "First A"`) {
		t.Fatalf("UUID move selected wrong occurrence: %s", stdout)
	}
}

func TestCLISmokeLocalPlayDryRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/up_next/list" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"episodes":[{"uuid":"` + smokeEpisodeUUID + `","title":"Local Smoke","url":"https://example.test/audio.mp3","playedUpTo":95}]}`))
	}))
	defer srv.Close()
	writeSmokeConfig(t, srv.URL)

	code, stdout, stderr := runForTest(t, []string{"local", "play", "--dry-run", "1"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "dry-run: would play local audio: Local Smoke") {
		t.Fatalf("stdout missing local dry-run summary: %q", stdout)
	}
}
