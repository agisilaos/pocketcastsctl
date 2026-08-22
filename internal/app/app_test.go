package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestWrapAndKindOf(t *testing.T) {
	base := errors.New("boom")
	err := Wrap(KindUnauthorized, "auth verify", base)
	if KindOf(err) != KindUnauthorized {
		t.Fatalf("KindOf = %q, want %q", KindOf(err), KindUnauthorized)
	}
	if KindOf(nil) != "" {
		t.Fatalf("KindOf(nil) should be empty")
	}
	if KindOf(base) != KindInternal {
		t.Fatalf("KindOf(non-app err) = %q, want %q", KindOf(base), KindInternal)
	}
}

func TestErrorStringAndUnwrap(t *testing.T) {
	base := errors.New("boom")
	err := &Error{Kind: KindTransient, Op: "queue fetch", Err: base}
	if got := err.Error(); got != "queue fetch: boom" {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is(err, base) {
		t.Fatalf("expected errors.Is to match wrapped error")
	}
}

func TestIsRetryableTransientError(t *testing.T) {
	if isRetryableTransientError(nil) {
		t.Fatalf("nil error should not be retryable")
	}
	if !isRetryableTransientError(errors.New("connection refused by peer")) {
		t.Fatalf("expected retryable connection error")
	}
	if isRetryableTransientError(errors.New("unauthorized")) {
		t.Fatalf("unauthorized should not be retryable")
	}
}

func TestSuggestNowActions(t *testing.T) {
	actions := suggestNowActions(NowSnapshot{
		Auth:  NowAuthStatus{Status: "missing"},
		Web:   NowWebPlaybackSnapshot{Status: "paused"},
		Local: NowLocalStatus{Status: "stopped"},
		Queue: NowQueueStatus{Status: "ready", Total: 2},
	})
	want := []string{
		"pocketcastsctl auth login",
		"pocketcastsctl auth import-browser --browser dia",
		"pocketcastsctl web toggle",
		"pocketcastsctl local pick --in-progress --recent",
		"pocketcastsctl queue api pick --recent",
	}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("actions = %#v, want %#v", actions, want)
	}
}

func TestCollectAuthStatusNoVerify(t *testing.T) {
	cfg := config.Config{
		APIHeaders: map[string]string{
			"Authorization": "Bearer x.eyJleHAiOjE3MzU2ODk2MDB9.y",
		},
	}
	status := collectAuthStatus(context.Background(), cfg, NowOptions{VerifyAuth: false})
	if status.Status != "configured" {
		t.Fatalf("status = %q, want configured", status.Status)
	}
	if !status.AuthorizationExists {
		t.Fatalf("AuthorizationExists = false, want true")
	}
	if !status.TokenExpiryKnown {
		t.Fatalf("TokenExpiryKnown = false, want true")
	}
}
