package pocketcasts

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestUpNextListRecognizedQueueIsAuthoritative(t *testing.T) {
	const a = "a1111111-1111-1111-1111-111111111111"
	const metadata = `,"episodeSync":[{"uuid":"b2222222-2222-2222-2222-222222222222","title":"Previously queued B"},{"uuid":"c3333333-3333-3333-3333-333333333333","title":"Previously queued C"}]`
	const episode = `{"uuid":"` + a + `","title":"Queued A"}`
	for _, tt := range []struct {
		name, queue string
		invalid     bool
	}{
		{name: "top level", queue: `"episodes":[` + episode + `]`},
		{name: "nested", queue: `"up_next":{"episodes":[` + episode + `]}`},
		{name: "nested wins over other episodes", queue: `"up_next":{"episodes":[` + episode + `]},"episodes":[]`},
		{name: "null queue", queue: `"episodes":null`, invalid: true},
		{name: "invalid queue type", queue: `"episodes":{}`, invalid: true},
		{name: "invalid nested queue", queue: `"up_next":{"episodes":null}`, invalid: true},
		{name: "missing nested queue", queue: `"up_next":{}`, invalid: true},
		{name: "invalid nested envelope", queue: `"up_next":null`, invalid: true},
		{name: "partial queue", queue: `"episodes":[` + episode + `,{}]`, invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := `{` + tt.queue + metadata + `}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(raw))
			}))
			defer server.Close()
			snapshot, err := New(Options{BaseURL: server.URL}).UpNextList(context.Background(), "0")
			if err != nil {
				t.Fatal(err)
			}
			if string(snapshot.Raw) != raw {
				t.Fatal("raw diagnostics changed")
			}
			if tt.invalid {
				if !errors.Is(snapshot.ParseError, ErrUnknownUpNextShape) || len(snapshot.Episodes) != 0 {
					t.Fatalf("malformed recognized queue accepted unrelated metadata: %+v", snapshot)
				}
				return
			}
			want := []UpNextEpisode{{UUID: a, Title: "Queued A"}}
			if snapshot.ParseError != nil || !reflect.DeepEqual(snapshot.Episodes, want) {
				t.Fatalf("episodes=%+v parse error=%v; want only actual queued A", snapshot.Episodes, snapshot.ParseError)
			}
		})
	}
}
