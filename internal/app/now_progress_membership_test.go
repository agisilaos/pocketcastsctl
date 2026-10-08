package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"pocketcastsctl/internal/config"
)

func TestNowCountsOnlyUniqueQueuedEpisodesInProgress(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "test-token")
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
"episodes":[
 {"uuid":"a1111111-1111-1111-1111-111111111111","title":"Queued A"},
 {"uuid":"a1111111-1111-1111-1111-111111111111","title":"Repeated A"}
],
"episodeSync":[
 {"uuid":"a1111111-1111-1111-1111-111111111111","playedUpTo":12},
 {"uuid":"b2222222-2222-2222-2222-222222222222","playedUpTo":42},
 {"uuid":"c3333333-3333-3333-3333-333333333333","playedUpTo":50}
]}`))
	}))
	defer server.Close()
	snapshot := CollectNowSnapshot(context.Background(), config.Config{
		APIBaseURL: server.URL, Browser: "unsupported-test-browser",
	}, NowOptions{})
	if snapshot.Queue.Status != "ready" || snapshot.Queue.Total != 2 || snapshot.Queue.InProgressCount != 1 {
		t.Fatalf("queue=%+v; want two occurrences and one queued episode in progress", snapshot.Queue)
	}
}
