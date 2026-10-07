package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestCockpitQueuePreservesOccurrencesAndProgress(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
  "up_next":{"episodes":[
    {"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":"First","published":"2026-08-31T10:00:00Z"},
    {"uuid":"826f30b0-adce-4f3b-b200-eacb1aa711eb","title":"Second"},
    {"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":"First again"}
  ]},
  "episodeSync":[{"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","playedUpTo":125}]
}`)
	}))
	defer server.Close()

	collector := NewCockpitCollector(config.Config{APIBaseURL: server.URL})
	snapshot := collector.Queue(context.Background())
	if snapshot.Status.Status != "ready" || snapshot.Status.Total != 3 {
		t.Fatalf("unexpected queue status: %+v", snapshot.Status)
	}
	if len(snapshot.Occurrences) != 3 {
		t.Fatalf("occurrence count = %d, want 3", len(snapshot.Occurrences))
	}
	if snapshot.Occurrences[0].Position != 1 || snapshot.Occurrences[0].Title != "First" || snapshot.Occurrences[0].Published != "2026-08-31T10:00:00Z" || snapshot.Occurrences[0].PlayedUpTo != 125 || !snapshot.Occurrences[0].HasProgress {
		t.Fatalf("unexpected first occurrence: %+v", snapshot.Occurrences[0])
	}
	if snapshot.Occurrences[2].Position != 3 || snapshot.Occurrences[2].Title != "First again" || snapshot.Occurrences[2].UUID != snapshot.Occurrences[0].UUID || !snapshot.Occurrences[2].HasProgress {
		t.Fatalf("repeated occurrence was not preserved: %+v", snapshot.Occurrences[2])
	}
}

func TestCockpitQueueReportsUnavailableWithoutOccurrences(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"unexpected":"shape"}`)
	}))
	defer server.Close()

	collector := NewCockpitCollector(config.Config{APIBaseURL: server.URL})
	snapshot := collector.Queue(context.Background())
	if snapshot.Status.Status != "unavailable" || len(snapshot.Occurrences) != 0 {
		t.Fatalf("unexpected queue snapshot: %+v", snapshot)
	}
}
