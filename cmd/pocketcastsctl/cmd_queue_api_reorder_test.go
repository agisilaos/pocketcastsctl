package main

import (
	"testing"

	"pocketcastsctl/internal/pocketcasts"
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
		{UUID: "a"},
		{UUID: "b"},
		{UUID: "a"},
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
