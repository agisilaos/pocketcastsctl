package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/config"
)

func TestPrintNowWarningsFormatsDiagnostics(t *testing.T) {
	var output bytes.Buffer
	printNowWarnings(&output, []string{"discarded stale state", "cache cleanup pending"})

	want := "now: warning: discarded stale state\nnow: warning: cache cleanup pending\n"
	if output.String() != want {
		t.Fatalf("printNowWarnings() = %q, want %q", output.String(), want)
	}
}

func TestNowVerifyAuthOutputUsesOneRequest(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "token")
	for _, mode := range []string{"json", "plain", "human"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, `{"episodes":[{"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":"Next episode"}]}`)
			}))
			defer server.Close()
			// Avoid interacting with the user's browser or local playback state.
			cfg := config.Config{APIBaseURL: server.URL}
			args := []string{"--verify-auth"}
			if mode != "human" {
				args = append(args, "--"+mode)
			}
			code, stdout, stderr := runForTestWithRunner(t, args, "", func(args []string) int { return runNow(args, cfg) })
			if code != 0 || calls.Load() != 1 || stderr != "" {
				t.Fatalf("code=%d calls=%d stderr=%q", code, calls.Load(), stderr)
			}
			if mode == "json" {
				var snapshot app.NowSnapshot
				if err := json.Unmarshal([]byte(stdout), &snapshot); err != nil {
					t.Fatal(err)
				}
				if snapshot.Auth.Status != "verified" || !snapshot.Auth.AuthorizationExists || snapshot.Queue.Status != "ready" || snapshot.Queue.Total != 1 || snapshot.Queue.NextTitle != "Next episode" || len(snapshot.Actions) == 0 {
					t.Fatalf("unexpected snapshot: %+v", snapshot)
				}
				for _, key := range []string{`"generated_at"`, `"web"`, `"local"`, `"queue"`, `"auth"`, `"actions"`, `"meta"`, `"authorization_present": true`, `"token_expiry_known": false`, `"in_progress_count": 0`} {
					if !strings.Contains(stdout, key) {
						t.Fatalf("missing JSON contract %q in %s", key, stdout)
					}
				}
			} else {
				want := []string{"POCKETCASTS NOW", "Queue  : READY (1 items) | next: Next episode", "Auth   : VERIFIED | source environment", "Recommended next actions:"}
				if mode == "plain" {
					want = []string{"queue_status\tready\n", "queue_total\t1\n", "queue_next_title\tNext episode\n", "auth_status\tverified\n", "auth_present\ttrue\n", "auth_source\tenvironment\n"}
				}
				for _, text := range want {
					if !strings.Contains(stdout, text) {
						t.Fatalf("missing output %q in %s", text, stdout)
					}
				}
			}
		})
	}
}
