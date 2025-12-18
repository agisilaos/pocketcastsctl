package har

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	f := File{
		Log: Log{
			Entries: []Entry{
				{Request: Request{Method: "POST", URL: "https://play.pocketcasts.com/graphql"}},
				{Request: Request{Method: "POST", URL: "https://play.pocketcasts.com/graphql"}},
				{Request: Request{Method: "GET", URL: "https://example.com/other"}},
			},
		},
	}
	s := Summarize(f, SummarizeOptions{Host: "play.pocketcasts.com"})
	if s.Total != 3 {
		t.Fatalf("Total=%d", s.Total)
	}
	if s.Matched != 2 {
		t.Fatalf("Matched=%d", s.Matched)
	}
	if len(s.Endpoints) != 1 {
		t.Fatalf("Endpoints=%d", len(s.Endpoints))
	}
	if s.Endpoints[0].Path != "/graphql" || s.Endpoints[0].Count != 2 {
		t.Fatalf("unexpected endpoint: %+v", s.Endpoints[0])
	}
}

func TestRedactFile(t *testing.T) {
	tmp := t.TempDir()
	inPath := filepath.Join(tmp, "in.har")
	outPath := filepath.Join(tmp, "out.har")

	input := map[string]any{
		"log": map[string]any{
			"entries": []any{
				map[string]any{
					"request": map[string]any{
						"method": "POST",
						"url":    "https://play.pocketcasts.com/graphql",
						"headers": []any{
							map[string]any{"name": "Authorization", "value": "Bearer secret"},
							map[string]any{"name": "Content-Type", "value": "application/json"},
						},
						"cookies": []any{
							map[string]any{"name": "session", "value": "cookie-secret"},
						},
						"postData": map[string]any{
							"mimeType": "application/json",
							"text":     `{"operationName":"Login","variables":{"email":"a@b.com","password":"p"}}`,
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(input)
	if err := os.WriteFile(inPath, b, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RedactFile(inPath, outPath, DefaultRedactOptions()); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "Bearer secret") {
		t.Fatalf("authorization not redacted: %s", s)
	}
	if strings.Contains(s, "cookie-secret") {
		t.Fatalf("cookie not redacted: %s", s)
	}
	if strings.Contains(s, "a@b.com") || strings.Contains(s, `"password":"p"`) {
		t.Fatalf("postData json keys not redacted: %s", s)
	}
}

func TestGraphQLOps(t *testing.T) {
	f := File{
		Log: Log{
			Entries: []Entry{
				{
					Request: Request{
						Method: "POST",
						URL:    "https://play.pocketcasts.com/graphql",
						PostData: &PostData{
							MimeType: "application/json",
							Text:     `{"operationName":"UpNextAdd","variables":{"episodeId":"123","position":1}}`,
						},
					},
				},
				{
					Request: Request{
						Method: "POST",
						URL:    "https://play.pocketcasts.com/graphql",
						PostData: &PostData{
							MimeType: "application/json",
							Text:     `{"operationName":"UpNextAdd","variables":{"episodeId":"456"}}`,
						},
					},
				},
			},
		},
	}
	s := GraphQLOps(f, GraphQLOpsOptions{Host: "play.pocketcasts.com"})
	if len(s.Ops) != 1 {
		t.Fatalf("Ops=%d", len(s.Ops))
	}
	if s.Ops[0].OperationName != "UpNextAdd" || s.Ops[0].Count != 2 {
		t.Fatalf("unexpected op: %+v", s.Ops[0])
	}
}
