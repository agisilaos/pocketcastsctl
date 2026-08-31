package pocketcasts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestUpNextListSnapshotAndRequest(t *testing.T) {
	const id = "94c87775-4f63-42db-9684-e3b1b5fbac08"
	const raw = ` {
  "episodes":[
    {"uuid":"` + id + `","title":"First"},
    {"uuid":"` + id + `","title":"Again"}
  ],
  "episodeSync":[{"uuid":"` + id + `","playedUpTo":95}]
} `
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/up_next/list" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		for _, header := range []string{"Accept", "Content-Type"} {
			if got := r.Header.Get(header); got != "application/json" {
				t.Errorf("%s = %q", header, got)
			}
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		want := map[string]any{
			"model": "webplayer", "version": float64(2), "showPlayStatus": true, "serverModified": "12345",
		}
		if !reflect.DeepEqual(payload, want) {
			t.Errorf("request payload = %v, want %v", payload, want)
		}
		_, _ = w.Write([]byte(raw))
	}))
	defer srv.Close()

	client := New(Options{BaseURL: srv.URL, HTTP: srv.Client()})
	snapshot, err := client.UpNextList(context.Background(), "12345")
	if err != nil || snapshot.ParseError != nil {
		t.Fatalf("fetch error = %v, parse error = %v", err, snapshot.ParseError)
	}
	want := []UpNextEpisode{{UUID: id, Title: "First"}, {UUID: id, Title: "Again"}}
	if !reflect.DeepEqual(snapshot.Episodes, want) || snapshot.Progress[id] != 95 || string(snapshot.Raw) != raw {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestUpNextListSeparatesParseAndRequestErrors(t *testing.T) {
	for _, tt := range []struct {
		name           string
		raw            string
		status         int
		wantUnknown    bool
		wantParseError bool
	}{
		{name: "recognized empty", raw: `{"episodes":[]}`, status: http.StatusOK},
		{name: "unknown", raw: `{"future":[]}`, status: http.StatusOK, wantUnknown: true, wantParseError: true},
		{name: "malformed", raw: `{"episodes":`, status: http.StatusOK, wantParseError: true},
		{name: "unauthorized", raw: `Unauthorized`, status: http.StatusUnauthorized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.raw))
			}))
			defer srv.Close()
			client := New(Options{BaseURL: srv.URL, HTTP: srv.Client()})
			snapshot, err := client.UpNextList(context.Background(), "0")
			if tt.status >= 400 {
				var statusErr interface{ HTTPStatusCode() int }
				if !errors.As(err, &statusErr) || statusErr.HTTPStatusCode() != tt.status {
					t.Fatalf("error = %v, want HTTP %d", err, tt.status)
				}
				if snapshot.Raw != nil || snapshot.ParseError != nil {
					t.Fatalf("request failure returned a response snapshot: %+v", snapshot)
				}
				return
			}
			if err != nil {
				t.Fatalf("response parsing returned a request error: %v", err)
			}
			if (snapshot.ParseError != nil) != tt.wantParseError || errors.Is(snapshot.ParseError, ErrUnknownUpNextShape) != tt.wantUnknown {
				t.Fatalf("parse error = %v", snapshot.ParseError)
			}
			if string(snapshot.Raw) != tt.raw {
				t.Fatalf("raw response changed: %q", snapshot.Raw)
			}
		})
	}
}
