package pocketcasts

import "testing"

func TestExtractUpNextEpisodes(t *testing.T) {
	raw := []byte(`{
  "up_next": {
    "episodes": [
      {"uuid":"94c87775-4f63-42db-9684-e3b1b5fbac08","title":"Ep 1","podcast":"1b96d010-ed82-013c-3086-0affccc8fded","published":"2025-12-17T09:15:00Z","url":"https://example.com/a.mp3"},
      {"uuid":"826f30b0-adce-4f3b-b200-eacb1aa711eb","title":"Ep 2"}
    ]
  }
}`)

	eps, err := ExtractUpNextEpisodes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 {
		t.Fatalf("len=%d", len(eps))
	}
	if eps[0].UUID != "94c87775-4f63-42db-9684-e3b1b5fbac08" || eps[0].Title != "Ep 1" {
		t.Fatalf("unexpected first: %+v", eps[0])
	}
}
