package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestQueueRemoveTrailingSafetyFlags(t *testing.T) {
	const uuid = "a1111111-1111-1111-1111-111111111111"
	for _, tc := range []struct {
		flag string
		code int
		want string
	}{
		{"--dry-run", 0, uuid},
		{"--help", 0, "Usage"},
		{"--unknown", 2, "flag provided but not defined"},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
			t.Setenv(config.EnvAccessToken, "synthetic-token")
			t.Setenv(config.EnvAPIBaseURL, server.URL)
			code, stdout, stderr := runForTest(t, []string{"queue", "api", "rm", "--force", uuid, tc.flag}, "")
			if requests.Load() != 0 {
				t.Errorf("%s sent %d API requests; preview/help/invalid flags must not mutate Up Next", tc.flag, requests.Load())
			}
			if code != tc.code || !strings.Contains(stdout+stderr, tc.want) {
				t.Errorf("code=%d stdout=%q stderr=%q; want code=%d containing %q", code, stdout, stderr, tc.code, tc.want)
			}
		})
	}
}

func TestQueuePlayAcceptsDocumentedTrailingFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/up_next/list" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"episodes":[{"uuid":"a1111111-1111-1111-1111-111111111111","title":"Fixture episode"}]}`)
	}))
	defer server.Close()
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	t.Setenv(config.EnvAccessToken, "synthetic-token")
	t.Setenv(config.EnvAPIBaseURL, server.URL)
	code, stdout, stderr := runForTest(t, []string{"queue", "api", "play", "1", "--search", "Fixture", "--dry-run"}, "")
	if code != 0 || !strings.Contains(stdout, "dry-run: would play in web player: Fixture episode") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestQueueFlagsRetainValuesAndExplicitOperandBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"literal help operand", []string{"queue", "api", "rm", "--dry-run", "--", "--help"}, 0, "--help\n"},
		{"help as search value", []string{"queue", "api", "play", "1", "--search", "--help", "--dry-run"}, 1, "no episodes matched"},
		{"missing value after selector", []string{"queue", "api", "play", "1", "--search"}, 2, "flag needs an argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/up_next/list" {
					t.Errorf("unexpected mutation: %s", r.URL.Path)
				}
				fmt.Fprint(w, `{"episodes":[]}`)
			}))
			defer server.Close()
			t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
			t.Setenv(config.EnvAccessToken, "synthetic-token")
			t.Setenv(config.EnvAPIBaseURL, server.URL)
			code, stdout, stderr := runForTest(t, tc.args, "")
			if code != tc.code || !strings.Contains(stdout+stderr, tc.want) {
				t.Fatalf("code=%d stdout=%q stderr=%q; want code=%d containing %q", code, stdout, stderr, tc.code, tc.want)
			}
		})
	}
}
