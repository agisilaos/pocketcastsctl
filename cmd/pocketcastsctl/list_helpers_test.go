package main

import (
	"reflect"
	"testing"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/pocketcasts"
)

func TestFilterEpisodes(t *testing.T) {
	eps := []pocketcasts.UpNextEpisode{
		{Title: "Foo"},
		{Title: "Bar Baz"},
	}
	got := filterEpisodes(eps, "ba")
	want := []pocketcasts.UpNextEpisode{{Title: "Bar Baz"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterEpisodes mismatch, got %+v want %+v", got, want)
	}
}

func TestFilterQueueItems(t *testing.T) {
	items := []browsercontrol.QueueItem{
		{Title: "Hello World"},
		{Title: "Goodbye"},
	}
	got := filterQueueItems(items, "world")
	want := []browsercontrol.QueueItem{{Title: "Hello World"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterQueueItems mismatch, got %+v want %+v", got, want)
	}
}

