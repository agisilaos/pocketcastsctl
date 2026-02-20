package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestVerifyAuthMissingHeader(t *testing.T) {
	err := VerifyAuth(context.Background(), config.Config{}, VerifyOptions{})
	if KindOf(err) != KindUnauthorized {
		t.Fatalf("kind = %q, want %q", KindOf(err), KindUnauthorized)
	}
}

func TestVerifyAuthSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"up_next":{"episodes":[{"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":"Ep"}]}}`))
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		APIHeaders: map[string]string{"Authorization": "Bearer token"},
	}
	if err := VerifyAuth(context.Background(), cfg, VerifyOptions{Attempts: 1}); err != nil {
		t.Fatalf("VerifyAuth error: %v", err)
	}
}

func TestVerifyAuthUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		APIHeaders: map[string]string{"Authorization": "Bearer token"},
	}
	err := VerifyAuth(context.Background(), cfg, VerifyOptions{Attempts: 1})
	if KindOf(err) != KindUnauthorized {
		t.Fatalf("kind = %q, want %q (err=%v)", KindOf(err), KindUnauthorized, err)
	}
}

func TestVerifyAuthTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		APIHeaders: map[string]string{"Authorization": "Bearer token"},
	}
	err := VerifyAuth(context.Background(), cfg, VerifyOptions{Attempts: 1})
	if KindOf(err) != KindTransient {
		t.Fatalf("kind = %q, want %q (err=%v)", KindOf(err), KindTransient, err)
	}
}
