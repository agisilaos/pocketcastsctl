package main

import (
	"io"
	"strings"
	"testing"

	"pocketcastsctl/internal/pocketcasts"
)

const (
	testEpisodeA = "a1111111-1111-1111-1111-111111111111"
	testEpisodeB = "b2222222-2222-2222-2222-222222222222"
)

func TestMoveEpisodeToPosition(t *testing.T) {
	eps := []pocketcasts.UpNextEpisode{
		{UUID: "a"},
		{UUID: "b"},
		{UUID: "c"},
		{UUID: "d"},
	}
	got := moveEpisodeToPosition(eps, 3, 1)
	if got[0].UUID != "a" || got[1].UUID != "d" || got[2].UUID != "b" || got[3].UUID != "c" {
		t.Fatalf("unexpected order after move: %#v", got)
	}
}

func TestDedupeEpisodes(t *testing.T) {
	eps := []pocketcasts.UpNextEpisode{
		{UUID: "a", Title: "first a"},
		{UUID: "b"},
		{UUID: "a", Title: "second a"},
		{UUID: "c"},
		{UUID: "b"},
	}
	unique, removed := dedupeEpisodes(eps)
	if len(unique) != 3 {
		t.Fatalf("unique len = %d, want 3", len(unique))
	}
	if len(removed) != 2 {
		t.Fatalf("removed len = %d, want 2", len(removed))
	}
	if unique[0].Title != "first a" {
		t.Fatalf("dedupe kept %q, want first occurrence", unique[0].Title)
	}
}

func TestReorderRepeatedEpisodeByNumericSelector(t *testing.T) {
	eps := repeatedEpisodeQueue()
	target, err := selectEpisode(queueOccurrences(eps), "3")
	if err != nil {
		t.Fatal(err)
	}
	if target.QueueIndex != 2 || target.Episode.Title != "Second A" {
		t.Fatalf("selection = %+v, want second A at index 2", target)
	}

	reordered := moveEpisodeToPosition(eps, target.QueueIndex, 0)
	if got := episodeTitles(reordered); got != "Second A,First A,B" {
		t.Fatalf("titles = %q, want second occurrence moved", got)
	}
}

func TestReorderRepeatedEpisodeByUUIDSelector(t *testing.T) {
	eps := repeatedEpisodeQueue()
	target, err := selectEpisode(queueOccurrences(eps), testEpisodeA)
	if err != nil {
		t.Fatal(err)
	}
	if target.QueueIndex != 0 || target.Episode.Title != "First A" {
		t.Fatalf("selection = %+v, want first A at index 0", target)
	}

	reordered := moveEpisodeToPosition(eps, target.QueueIndex, 2)
	if got := episodeTitles(reordered); got != "B,Second A,First A" {
		t.Fatalf("titles = %q, want first UUID occurrence moved", got)
	}
}

func TestFilteredSelectionPreservesOriginalQueueIndex(t *testing.T) {
	eps := []pocketcasts.UpNextEpisode{
		{UUID: testEpisodeA, Title: "Skip"},
		{UUID: testEpisodeB, Title: "Keep B"},
		{UUID: testEpisodeA, Title: "Keep A"},
	}
	candidates := filterQueueOccurrences(queueOccurrences(eps), "keep")
	target, err := selectEpisode(candidates, "2")
	if err != nil {
		t.Fatal(err)
	}
	if target.QueueIndex != 2 || target.Episode.Title != "Keep A" {
		t.Fatalf("selection = %+v, want original queue index 2", target)
	}
}

func TestPromptedSelectionPreservesOriginalQueueIndex(t *testing.T) {
	candidates := filterQueueOccurrences(queueOccurrences([]pocketcasts.UpNextEpisode{
		{UUID: testEpisodeA, Title: "Skip"},
		{UUID: testEpisodeB, Title: "Keep B"},
		{UUID: testEpisodeA, Title: "Keep A"},
	}), "keep")

	target, err := pickWithPromptIO(candidates, strings.NewReader("2\n"), io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if target.QueueIndex != 2 || target.Episode.Title != "Keep A" {
		t.Fatalf("selection = %+v, want original queue index 2", target)
	}
}

func TestParseOneBasedIndex(t *testing.T) {
	if _, err := parseOneBasedIndex("0", 3); err == nil {
		t.Fatalf("expected out-of-range error")
	}
	if _, err := parseOneBasedIndex("abc", 3); err == nil {
		t.Fatalf("expected parse error")
	}
	n, err := parseOneBasedIndex("2", 3)
	if err != nil {
		t.Fatalf("parseOneBasedIndex error = %v", err)
	}
	if n != 2 {
		t.Fatalf("n = %d, want 2", n)
	}
}

func repeatedEpisodeQueue() []pocketcasts.UpNextEpisode {
	return []pocketcasts.UpNextEpisode{
		{UUID: testEpisodeA, Title: "First A"},
		{UUID: testEpisodeB, Title: "B"},
		{UUID: testEpisodeA, Title: "Second A"},
	}
}

func episodeTitles(eps []pocketcasts.UpNextEpisode) string {
	titles := make([]string, len(eps))
	for i, ep := range eps {
		titles[i] = ep.Title
	}
	return strings.Join(titles, ",")
}
