package main

import (
	"reflect"
	"testing"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/pocketcasts"
)

func TestFilterQueueOccurrences(t *testing.T) {
	eps := []pocketcasts.UpNextEpisode{
		{Title: "Foo"},
		{Title: "Bar Baz"},
	}
	got := filterQueueOccurrences(queueOccurrences(eps), "ba")
	want := []queueOccurrence{{Episode: pocketcasts.UpNextEpisode{Title: "Bar Baz"}, QueueIndex: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterQueueOccurrences mismatch, got %+v want %+v", got, want)
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
