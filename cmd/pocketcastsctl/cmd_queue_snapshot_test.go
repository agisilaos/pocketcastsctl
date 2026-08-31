package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestQueueListSnapshotOutput(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "test-token")
	for _, tt := range []struct {
		name string
		raw  string
		mode string
		want string
	}{
		{name: "empty JSON", raw: `{"episodes":[]}`, mode: "--json", want: "[]\n"},
		{name: "empty plain", raw: `{"up_next":{"episodes":[]}}`, mode: "--plain", want: ""},
		{name: "raw empty", raw: " {\"episodes\":[]} ", mode: "--raw", want: " {\"episodes\":[]} \n"},
		{name: "raw unknown", raw: " {\"unexpected\": []} ", mode: "--raw", want: " {\"unexpected\": []} \n"},
		{name: "raw malformed", raw: `{"episodes":`, mode: "--raw", want: "{\"episodes\":\n"},
		{name: "unknown fallback", raw: `{"unexpected":[]}`, mode: "--json", want: "{\n  \"unexpected\": []\n}\n"},
		{name: "malformed fallback", raw: `not JSON`, mode: "--plain", want: "not JSON\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/up_next/list" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				_, _ = w.Write([]byte(tt.raw))
			}))
			defer srv.Close()
			writeSmokeConfig(t, srv.URL)
			code, stdout, stderr := runForTest(t, []string{"queue", "api", "ls", tt.mode}, "")
			if code != 0 || stdout != tt.want || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q, want stdout=%q", code, stdout, stderr, tt.want)
			}
			if requests != 1 {
				t.Fatalf("requests=%d, response parsing should not retry", requests)
			}
		})
	}
}

func TestQueueCommandsDistinguishEmptyAndUnknownSnapshot(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "test-token")
	for _, command := range []struct {
		args         []string
		emptyMessage string
		emptyCode    int
	}{
		{args: []string{"queue", "api", "bump", "--dry-run", "1"}, emptyMessage: "queue is empty", emptyCode: 1},
		{args: []string{"queue", "api", "move", "--dry-run", "1", "2"}, emptyMessage: "queue is empty", emptyCode: 1},
		{args: []string{"queue", "api", "dedupe", "--dry-run"}, emptyMessage: "queue is empty", emptyCode: 1},
		{args: []string{"queue", "api", "play", "--dry-run", "1"}, emptyMessage: "no episodes matched", emptyCode: 1},
		{args: []string{"queue", "api", "pick", "--no-play"}, emptyMessage: "no episodes matched", emptyCode: 1},
		{args: []string{"local", "play", "--dry-run", "1"}, emptyMessage: "index out of range", emptyCode: 2},
		{args: []string{"local", "pick"}, emptyMessage: "no episodes matched", emptyCode: 1},
	} {
		for _, empty := range []bool{true, false} {
			raw := `{"unexpected":[]}`
			wantMessage, wantCode := "failed to parse queue: unknown Up Next response shape", 1
			if empty {
				raw = `{"episodes":[]}`
				wantMessage, wantCode = command.emptyMessage, command.emptyCode
			}
			t.Run(strings.Join(command.args, " ")+"/"+raw, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/up_next/list" {
						t.Errorf("unexpected mutation: %s", r.URL.Path)
					}
					_, _ = w.Write([]byte(raw))
				}))
				defer srv.Close()
				writeSmokeConfig(t, srv.URL)
				code, stdout, stderr := runForTest(t, command.args, "")
				if code != wantCode || stdout != "" || !strings.Contains(stderr, wantMessage) {
					t.Fatalf("code=%d stdout=%q stderr=%q, want code=%d and %q", code, stdout, stderr, wantCode, wantMessage)
				}
			})
		}
	}
}
