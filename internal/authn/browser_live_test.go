package authn

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveBrowserSession is an opt-in, non-persisting smoke test. It decrypts
// only Pocket Casts' auth cookie, validates it, and never prints token material.
//
// Run once per supported browser before merge:
//
//	POCKETCASTS_LIVE_BROWSER=dia go test ./internal/authn -run TestLiveBrowserSession -count=1
func TestLiveBrowserSession(t *testing.T) {
	browser := strings.ToLower(strings.TrimSpace(os.Getenv("POCKETCASTS_LIVE_BROWSER")))
	if browser == "" {
		t.Skip("set POCKETCASTS_LIVE_BROWSER to chrome, dia, or safari")
	}
	if !SupportedBrowser(browser) {
		t.Fatalf("unsupported live browser %q", browser)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	candidates, warnings, err := BrowserCandidates(ctx, NewSweetCookieReader(), browser, "", time.Now())
	if err != nil {
		t.Fatalf("read %s Pocket Casts session: %v", browser, err)
	}
	if len(candidates) == 0 {
		for _, warning := range warnings {
			t.Logf("browser warning: %s", redactLivePath(warning))
		}
		t.Fatalf("no decodable Pocket Casts session found in %s (%d warning(s))", browser, len(warnings))
	}
	valid, rejected := ValidBrowserCandidates(ctx, NewAPI("", nil), candidates)
	if len(valid) == 0 {
		t.Fatalf("no valid Pocket Casts session found in %s (%d candidate(s), %d rejected)", browser, len(candidates), len(rejected))
	}
}

// TestLiveWebPlayerScopeMatrix sends deliberately malformed JSON to mutation
// routes. A non-authentication response proves the token reached route-level
// validation, while invalid JSON guarantees the queue cannot be changed.
func TestLiveWebPlayerScopeMatrix(t *testing.T) {
	browser := strings.ToLower(strings.TrimSpace(os.Getenv("POCKETCASTS_SCOPE_LIVE_BROWSER")))
	if browser == "" {
		t.Skip("set POCKETCASTS_SCOPE_LIVE_BROWSER to a signed-in browser")
	}
	if !SupportedBrowser(browser) {
		t.Fatalf("unsupported live browser %q", browser)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	candidates, _, err := BrowserCandidates(ctx, NewSweetCookieReader(), browser, "", time.Now())
	if err != nil || len(candidates) == 0 {
		t.Fatalf("read %s Pocket Casts session: candidates=%d error=%v", browser, len(candidates), err)
	}
	valid, _ := ValidBrowserCandidates(ctx, NewAPI("", nil), candidates)
	if len(valid) == 0 {
		t.Fatalf("no valid Pocket Casts session found in %s", browser)
	}

	for _, route := range []string{"/up_next/play_next", "/up_next/remove"} {
		t.Run(route, func(t *testing.T) {
			status, err := probeAuthenticatedRoute(ctx, route, valid[0].Session.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				t.Fatalf("webplayer scope was rejected by %s (HTTP %d)", route, status)
			}
			if status == http.StatusNotFound {
				t.Fatalf("route probe was inconclusive for %s (HTTP %d)", route, status)
			}
		})
	}
}

func probeAuthenticatedRoute(ctx context.Context, route, accessToken string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.pocketcasts.com"+route, bytes.NewBufferString("{"))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return 0, fmt.Errorf("probe %s: %w", route, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, nil
}

func redactLivePath(message string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return message
	}
	return strings.ReplaceAll(message, home, "~")
}
