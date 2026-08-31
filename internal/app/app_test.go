package app

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/localplayback"
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

func TestLocalStatusFromLifecycleSnapshot(t *testing.T) {
	for _, test := range []struct {
		name     string
		snapshot localplayback.Snapshot
		want     NowLocalStatus
	}{
		{name: "playing", snapshot: localplayback.Snapshot{Status: localplayback.StatusPlaying, Title: " Episode "}, want: NowLocalStatus{Status: "playing", Title: "Episode"}},
		{name: "paused", snapshot: localplayback.Snapshot{Status: localplayback.StatusPaused, Title: " Episode "}, want: NowLocalStatus{Status: "paused", Title: "Episode"}},
		{name: "stopped", snapshot: localplayback.Snapshot{Status: localplayback.StatusStopped, Title: "ignored"}, want: NowLocalStatus{Status: "stopped"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := localStatusFromSnapshot(test.snapshot); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("localStatusFromSnapshot() = %+v, want %+v", got, test.want)
			}
		})
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

func TestSuggestNowActions(t *testing.T) {
	actions := suggestNowActions(NowSnapshot{
		Auth:  NowAuthStatus{Status: "missing"},
		Web:   NowWebPlaybackSnapshot{State: "paused"},
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
	client := &http.Client{Transport: probeTransport(func(*http.Request) (*http.Response, error) {
		return probeResponse(http.StatusOK, `{"episodes":[]}`), nil
	})}
	status, _ := collectNowAPIStatus(context.Background(), cfg, NowOptions{}, authn.ManagerOptions{HTTP: client})
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
