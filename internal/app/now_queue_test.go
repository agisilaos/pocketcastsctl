package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

func TestCollectQueueStatusMissingAuthHeader(t *testing.T) {
	_, st := collectNowAPIStatus(context.Background(), config.Config{}, NowOptions{}, authn.ManagerOptions{})
	if st.Status != "unauthorized" {
		t.Fatalf("status = %q, want unauthorized", st.Status)
	}
}

func TestCollectQueueStatusUnauthorizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		APIHeaders: map[string]string{"Authorization": "Bearer token"},
	}
	_, st := collectNowAPIStatus(context.Background(), cfg, NowOptions{}, authn.ManagerOptions{})
	if st.Status != "unauthorized" {
		t.Fatalf("status = %q, want unauthorized", st.Status)
	}
}

func TestCollectQueueStatusParseFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"unexpected":"shape"}`))
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		APIHeaders: map[string]string{"Authorization": "Bearer token"},
	}
	_, st := collectNowAPIStatus(context.Background(), cfg, NowOptions{}, authn.ManagerOptions{})
	if st.Status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", st.Status)
	}
	if st.Error != "failed to parse queue" {
		t.Fatalf("error = %q, want parse queue error", st.Error)
	}
}

func TestCollectQueueStatusReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
  "up_next": {
    "episodes": [
      {"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":"Ep 1","published":"2025-12-17T09:15:00Z"},
      {"uuid":"826f30b0-adce-4f3b-b200-eacb1aa711eb","title":"Ep 2"}
    ]
  },
  "episodeSync":[
    {"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","playedUpTo":1805}
  ]
}`))
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		APIHeaders: map[string]string{"Authorization": "Bearer token"},
	}
	_, st := collectNowAPIStatus(context.Background(), cfg, NowOptions{}, authn.ManagerOptions{})
	if st.Status != "ready" {
		t.Fatalf("status = %q, want ready", st.Status)
	}
	if st.Total != 2 {
		t.Fatalf("total = %d, want 2", st.Total)
	}
	if st.NextTitle != "Ep 1" {
		t.Fatalf("next title = %q, want Ep 1", st.NextTitle)
	}
	if st.InProgressCount != 1 {
		t.Fatalf("inProgress = %d, want 1", st.InProgressCount)
	}
}

func TestCollectQueueStatusEmpty(t *testing.T) {
	for _, raw := range []string{`{"episodes":[]}`, `{"up_next":{"episodes":[]}}`} {
		t.Run(raw, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(raw))
			}))
			defer srv.Close()
			cfg := config.Config{
				APIBaseURL: srv.URL,
				APIHeaders: map[string]string{"Authorization": "Bearer token"},
			}
			_, status := collectNowAPIStatus(context.Background(), cfg, NowOptions{}, authn.ManagerOptions{})
			if status.Status != "empty" || status.Total != 0 || status.Error != "" {
				t.Fatalf("expected empty queue status, got %+v", status)
			}
		})
	}
}
