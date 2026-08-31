package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

type probeTransport func(*http.Request) (*http.Response, error)

func (transport probeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func probeResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestUpNextProbeClassificationAndRetries(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	for _, test := range []struct {
		name      string
		status    int
		body      string
		err       error
		missing   bool
		wantAuth  string
		wantQueue string
		wantKind  ErrorKind
		wantCalls int
	}{
		{name: "ready", status: 200, body: `{"episodes":[{"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":" Episode "}]}`, wantAuth: "verified", wantQueue: "ready", wantCalls: 1},
		{name: "empty", status: 200, body: `{"episodes":[]}`, wantAuth: "verified", wantQueue: "empty", wantCalls: 1},
		{name: "unknown queue", status: 200, body: `{"unexpected":"shape"}`, wantAuth: "verified", wantQueue: "unavailable", wantCalls: 1},
		{name: "malformed JSON", status: 200, body: `{`, wantAuth: "verified", wantQueue: "unavailable", wantCalls: 1},
		{name: "missing", missing: true, wantAuth: "missing", wantQueue: "unauthorized", wantKind: KindUnauthorized},
		{name: "unauthorized", status: 401, body: "timeout EOF", wantAuth: "unauthorized", wantQueue: "unauthorized", wantKind: KindUnauthorized, wantCalls: 1},
		{name: "forbidden", status: 403, body: "401 Unauthorized timeout EOF", wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 1},
		{name: "bad request", status: 400, body: "timeout", wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 1},
		{name: "server error", status: 500, body: "boom", wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 3},
		{name: "unavailable", status: 503, wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 3},
		{name: "rate limit", status: 429, wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 3},
		{name: "request timeout", status: 408, wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 3},
		{name: "wrapped HTTP", err: fmt.Errorf("wrapped: %w", pocketcasts.NewHTTPError(401, "rejected")), wantAuth: "unauthorized", wantQueue: "unauthorized", wantKind: KindUnauthorized, wantCalls: 1},
		{name: "wrapped connection reset", err: fmt.Errorf("read: %w", syscall.ECONNRESET), wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 3},
		{name: "DNS timeout", err: &net.DNSError{Err: "lookup failed", IsTimeout: true}, wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 3},
		{name: "DNS name not found", err: &net.DNSError{Err: "timeout EOF", IsNotFound: true}, wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 1},
		{name: "misleading message", err: errors.New("http 401 Unauthorized connection reset timeout EOF"), wantAuth: "unverified", wantQueue: "unavailable", wantKind: KindTransient, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			cfg := config.Config{APIBaseURL: "https://api.example.test", APIHeaders: map[string]string{"Authorization": "Bearer token"}}
			if test.missing {
				cfg.APIHeaders = nil
			}
			httpClient := &http.Client{Transport: probeTransport(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.URL.Path != "/up_next/list" || request.Header.Get("Authorization") != "Bearer token" {
					t.Fatalf("unexpected request: %s", request.URL)
				}
				if test.err != nil {
					return nil, test.err
				}
				return probeResponse(test.status, test.body), nil
			})}
			result := probeUpNext(context.Background(), cfg, authn.ManagerOptions{HTTP: httpClient}, upNextRetryPolicy{attempts: 3, baseDelay: time.Millisecond})
			auth, queue := result.authStatus(true), result.queueStatus()
			if auth.Status != test.wantAuth || queue.Status != test.wantQueue || KindOf(result.err) != test.wantKind || calls != test.wantCalls {
				t.Fatalf("auth=%+v queue=%+v err=%v calls=%d; want %s/%s/%s/%d", auth, queue, result.err, calls, test.wantAuth, test.wantQueue, test.wantKind, test.wantCalls)
			}
			if test.err != nil && !errors.Is(result.err, test.err) {
				t.Fatalf("error chain lost: %v", result.err)
			}
			if !test.missing {
				if configured := result.authStatus(false); configured.Status != "configured" || configured.Error != "" {
					t.Fatalf("verification disabled changed credential status: %+v", configured)
				}
			}
		})
	}
}

func TestUpNextRetryUsesStructuredErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"cancelled", context.Canceled, false},
		{"missing credential", fmt.Errorf("load: %w", authn.ErrNotConfigured), false},
		{"unavailable credential", authn.ErrCredentialUnavailable, false},
		{"EOF", fmt.Errorf("read: %w", io.EOF), true},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, true},
		{"broken pipe", syscall.EPIPE, true},
		{"request timeout", &url.Error{Op: "Post", Err: context.DeadlineExceeded}, true},
		{"temporary DNS", &net.DNSError{IsTemporary: true}, true},
		{"text", errors.New("timeout temporarily unavailable EOF connection refused"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isRetryableUpNextError(test.err); got != test.want {
				t.Fatalf("retryable(%v)=%v, want %v", test.err, got, test.want)
			}
		})
	}
}

func TestVerifyAuthRetriesUntilSuccessOrLimit(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "token")
	for _, succeeds := range []bool{true, false} {
		t.Run(fmt.Sprint(succeeds), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				attempt := calls.Add(1)
				if succeeds && attempt == 2 {
					_, _ = io.WriteString(w, `{"episodes":[]}`)
					return
				}
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			err := VerifyAuth(context.Background(), config.Config{APIBaseURL: server.URL})
			if succeeds {
				if err != nil || calls.Load() != 2 {
					t.Fatalf("error=%v calls=%d, want success in two calls", err, calls.Load())
				}
			} else if KindOf(err) != KindTransient || calls.Load() != 3 {
				t.Fatalf("error=%v calls=%d, want transient failure in three calls", err, calls.Load())
			}
		})
	}
}

func TestUpNextProbeRetriesClientTimeoutWithLiveParent(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "token")
	var calls atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		if calls.Add(1) == 1 {
			select {
			case <-request.Context().Done():
			case <-release:
			}
			return
		}
		_, _ = io.WriteString(w, `{"episodes":[]}`)
	}))
	defer server.Close()
	defer close(release)
	client := server.Client()
	client.Timeout = 200 * time.Millisecond
	result := probeUpNext(context.Background(), config.Config{APIBaseURL: server.URL}, authn.ManagerOptions{HTTP: client}, upNextRetryPolicy{attempts: 3, baseDelay: time.Millisecond})
	if result.err != nil || calls.Load() != 2 || result.authStatus(true).Status != "verified" {
		t.Fatalf("result=%+v calls=%d", result, calls.Load())
	}
}
