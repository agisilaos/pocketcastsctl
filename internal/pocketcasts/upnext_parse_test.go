package pocketcasts

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func TestParseUpNextSnapshot(t *testing.T) {
	raw := []byte(`{
  "up_next": {
    "episodes": [
      {"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":"Ep 1","podcast":"1b96d010-ed82-013c-3086-0affccc8fded","published":"2025-12-17T09:15:00Z","url":"https://example.com/a.mp3"},
      {"uuid":"826f30b0-adce-4f3b-b200-eacb1aa711eb","title":"Ep 2"}
    ]
  }
}`)

	snapshot := parseUpNextSnapshot(raw)
	if snapshot.ParseError != nil {
		t.Fatal(snapshot.ParseError)
	}
	eps := snapshot.Episodes
	if len(eps) != 2 {
		t.Fatalf("len=%d", len(eps))
	}
	if eps[0].UUID != "94c87775-4f63-42db-9684-e3b1b5fbac08" || eps[0].Title != "Ep 1" {
		t.Fatalf("unexpected first: %+v", eps[0])
	}
}

func TestParseUpNextSnapshotPreservesQueueOccurrences(t *testing.T) {
	const a = "94c87775-4f63-42db-9684-e3b1b5fbac08"
	const b = "826f30b0-adce-4f3b-b200-eacb1aa711eb"
	raw := []byte(`{
  "episodes": [
    {"uuid":"` + a + `","title":"First A"},
    {"uuid":"` + b + `","title":"B"},
    {"uuid":"` + a + `","title":"Second A"}
  ]
}`)

	snapshot := parseUpNextSnapshot(raw)
	if snapshot.ParseError != nil {
		t.Fatal(snapshot.ParseError)
	}
	eps := snapshot.Episodes
	if len(eps) != 3 {
		t.Fatalf("len = %d, want 3", len(eps))
	}
	if eps[0].Title != "First A" || eps[2].Title != "Second A" {
		t.Fatalf("queue occurrences were not preserved: %+v", eps)
	}
}

func TestParseUpNextSnapshotFallbackMergesRepeatedMetadata(t *testing.T) {
	const id = "94c87775-4f63-42db-9684-e3b1b5fbac08"
	raw := []byte(`{
  "episode": {"uuid":"` + id + `","title":"Episode"},
  "metadata": {"episodeUuid":"` + id + `","episodeTitle":"Episode","audioUrl":"https://example.test/a.mp3"},
  "details": {"episode_uuid":"` + id + `","episode_title":"Episode","podcastUuid":"podcast-a","publishedAt":"2026-08-31","playedTo":42}
}`)

	snapshot := parseUpNextSnapshot(raw)
	if snapshot.ParseError != nil {
		t.Fatal(snapshot.ParseError)
	}
	eps := snapshot.Episodes
	if len(eps) != 1 {
		t.Fatalf("len = %d, want 1", len(eps))
	}
	if eps[0].URL != "https://example.test/a.mp3" {
		t.Fatalf("merged URL = %q", eps[0].URL)
	}
	if eps[0].Podcast != "podcast-a" || eps[0].Published != "2026-08-31" || snapshot.Progress[id] != 42 {
		t.Fatalf("fallback metadata was not merged: %+v", snapshot)
	}
}

func TestParseUpNextSnapshotProgress(t *testing.T) {
	raw := []byte(`{
  "episodes":[{"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":"Ep 1"}],
  "episodeSync":[
    {"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","playedUpTo":1805,"duration":4831},
    {"uuid":"826f30b0-adce-4f3b-b200-eacb1aa711eb","duration":1200}
  ]
}`)
	snapshot := parseUpNextSnapshot(raw)
	if snapshot.ParseError != nil {
		t.Fatal(snapshot.ParseError)
	}
	progress := snapshot.Progress
	if got := progress["94c87775-4f63-42db-9684-e3b1b5fbac08"]; got != 1805 {
		t.Fatalf("progress mismatch = %d, want 1805", got)
	}
	if _, ok := progress["826f30b0-adce-4f3b-b200-eacb1aa711eb"]; ok {
		t.Fatalf("expected no progress for episode without playedUpTo")
	}
}

func TestParseUpNextSnapshotEmptyAndUnknown(t *testing.T) {
	for _, tt := range []struct {
		name  string
		raw   string
		empty bool
	}{
		{name: "top level episodes", raw: `{"episodes":[]}`, empty: true},
		{name: "nested queue episodes", raw: `{"up_next":{"episodes":[]}}`, empty: true},
		{name: "top level array", raw: `[]`, empty: true},
		{name: "empty object", raw: `{}`},
		{name: "unknown object", raw: `{"unexpected":"shape"}`},
		{name: "null response", raw: `null`},
		{name: "scalar response", raw: `"episodes"`},
		{name: "null episodes", raw: `{"episodes":null}`},
		{name: "wrong type episodes", raw: `{"episodes":{}}`},
		{name: "missing nested episodes", raw: `{"up_next":{}}`},
		{name: "unrelated empty array", raw: `{"episodeSync":[]}`},
		{name: "unrelated nested empty episodes", raw: `{"metadata":{"episodes":[]}}`},
		{name: "invalid members", raw: `{"episodes":[null,{},"unexpected"]}`},
		{name: "invalid uuid", raw: `{"episodes":[{"uuid":"invalid","title":"Episode"}]}`},
		{name: "missing title", raw: `{"episodes":[{"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08"}]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := []byte(tt.raw)
			snapshot := parseUpNextSnapshot(raw)
			if tt.empty {
				if snapshot.ParseError != nil || snapshot.Episodes == nil || len(snapshot.Episodes) != 0 {
					t.Fatalf("expected recognized empty queue, got %+v", snapshot)
				}
			} else if !errors.Is(snapshot.ParseError, ErrUnknownUpNextShape) {
				t.Fatalf("parse error = %v, want unknown shape", snapshot.ParseError)
			}
			if !bytes.Equal(snapshot.Raw, raw) {
				t.Fatalf("raw response changed: %q", snapshot.Raw)
			}
		})
	}
}

func TestParseUpNextSnapshotKeepsRawOnInvalidJSON(t *testing.T) {
	for _, raw := range []string{"", `{"episodes":`, `{"episodes":[]} trailing`} {
		snapshot := parseUpNextSnapshot([]byte(raw))
		if snapshot.ParseError == nil || errors.Is(snapshot.ParseError, ErrUnknownUpNextShape) {
			t.Fatalf("raw %q: expected JSON parse error, got %v", raw, snapshot.ParseError)
		}
		if string(snapshot.Raw) != raw {
			t.Fatalf("raw response changed: %q", snapshot.Raw)
		}
	}
}

func TestParseUpNextSnapshotEmptyQueueIgnoresOtherEpisodeMetadata(t *testing.T) {
	const id = "94c87775-4f63-42db-9684-e3b1b5fbac08"
	for _, queue := range []string{`"episodes":[]`, `"up_next":{"episodes":[]}`} {
		raw := []byte(`{` + queue + `,"episodeSync":[{"uuid":"` + id + `","title":"Previously queued","playedUpTo":42}]}`)
		snapshot := parseUpNextSnapshot(raw)
		if snapshot.ParseError != nil || len(snapshot.Episodes) != 0 {
			t.Fatalf("expected empty queue despite metadata: %+v", snapshot)
		}
		if snapshot.Progress[id] != 42 {
			t.Fatalf("progress = %v, want 42 seconds", snapshot.Progress)
		}
	}
}

func TestParseUpNextSnapshotTolerantArrayAndProgress(t *testing.T) {
	const a = "94c87775-4f63-42db-9684-e3b1b5fbac08"
	const b = "826f30b0-adce-4f3b-b200-eacb1aa711eb"
	raw := []byte(`{
  "futureEnvelope":{"items":[
    {"episode_uuid":"` + a + `","episode_title":"First A","podcast":{"uuid":"podcast-a"},"published_at":"2026-08-31","audio_url":"https://example.test/a.mp3"},
    null, {},
    {"episodeUuid":"` + b + `","episodeTitle":"B"},
    {"uuid":"` + a + `","title":"Second A"}
  ]},
  "episodeSync":[
    {"episode_uuid":"` + a + `","played_up_to":95},
    {"episodeUuid":"` + b + `","position":12},
    {"uuid":"` + a + `","playedUpTo":0},
    {"uuid":"` + b + `","playedUpTo":-5}
  ]
}`)
	snapshot := parseUpNextSnapshot(raw)
	want := []UpNextEpisode{
		{UUID: a, Title: "First A", Podcast: "podcast-a", Published: "2026-08-31", URL: "https://example.test/a.mp3"},
		{UUID: b, Title: "B"},
		{UUID: a, Title: "Second A"},
	}
	if snapshot.ParseError != nil || !reflect.DeepEqual(snapshot.Episodes, want) {
		t.Fatalf("snapshot = %+v, want episodes %+v", snapshot, want)
	}
	if !reflect.DeepEqual(snapshot.Progress, map[string]int{a: 95, b: 12}) {
		t.Fatalf("progress = %v", snapshot.Progress)
	}
	if !bytes.Equal(snapshot.Raw, raw) {
		t.Fatalf("raw response changed: %q", snapshot.Raw)
	}
}
