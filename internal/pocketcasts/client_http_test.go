package pocketcasts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAppliesDefaultsAndClonesHeaders(t *testing.T) {
	opts := Options{Headers: map[string]string{" Authorization ": "Bearer abc", "": "x"}}
	c := New(opts)
	if c.baseURL != "https://play.pocketcasts.com" {
		t.Fatalf("baseURL = %q, want default", c.baseURL)
	}
	if c.http == nil {
		t.Fatalf("http client should not be nil")
	}
	if got := c.headers["Authorization"]; got != "Bearer abc" {
		t.Fatalf("Authorization header = %q, want Bearer abc", got)
	}
	if _, ok := c.headers[""]; ok {
		t.Fatalf("expected empty header key to be dropped")
	}
}

func TestNewRequestAddsLeadingSlashAndHeaders(t *testing.T) {
	c := New(Options{BaseURL: "https://api.example.com/", Headers: map[string]string{"Authorization": "Bearer xyz"}})
	req, err := c.newRequest(context.Background(), http.MethodPost, "up_next/list", nil)
	if err != nil {
		t.Fatalf("newRequest error: %v", err)
	}
	if req.URL.String() != "https://api.example.com/up_next/list" {
		t.Fatalf("url = %q", req.URL.String())
	}
	if got := req.Header.Get("Authorization"); got != "Bearer xyz" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestUpNextListSuccessAndError(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/up_next/list" {
				t.Fatalf("path = %q", r.URL.Path)
			}
			if r.Method != http.MethodPost {
				t.Fatalf("method = %q", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer token" {
				t.Fatalf("Authorization = %q", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		c := New(Options{BaseURL: srv.URL, Headers: map[string]string{"Authorization": "Bearer token"}, HTTP: srv.Client()})
		body, err := c.UpNextList(context.Background(), UpNextListRequest{Model: "webplayer", ServerModified: "0", ShowPlayStatus: true, Version: 2})
		if err != nil {
			t.Fatalf("UpNextList error: %v", err)
		}
		if string(body) != `{"ok":true}` {
			t.Fatalf("body = %q", string(body))
		}
	})

	t.Run("http error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
		}))
		defer srv.Close()

		c := New(Options{BaseURL: srv.URL, HTTP: srv.Client()})
		_, err := c.UpNextList(context.Background(), UpNextListRequest{Model: "webplayer", ServerModified: "0", ShowPlayStatus: true, Version: 2})
		if err == nil || !strings.Contains(err.Error(), "http 401") {
			t.Fatalf("error = %v, want http 401", err)
		}
	})
}

func TestUpNextMutationsRequests(t *testing.T) {
	seen := make([]map[string]any, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		seen = append(seen, payload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(Options{BaseURL: srv.URL, HTTP: srv.Client()})
	_, err := c.UpNextPlayNext(context.Background(), UpNextEpisode{UUID: "u-1", Title: "Ep"}, "7")
	if err != nil {
		t.Fatalf("UpNextPlayNext error: %v", err)
	}
	_, err = c.UpNextRemove(context.Background(), []string{"u-1", "u-2"}, "8")
	if err != nil {
		t.Fatalf("UpNextRemove error: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("seen payload count = %d, want 2", len(seen))
	}
	if got := seen[0]["version"]; got != float64(2) {
		t.Fatalf("play_next version = %v, want 2", got)
	}
	if got := seen[1]["version"]; got != float64(2) {
		t.Fatalf("remove version = %v, want 2", got)
	}
}
