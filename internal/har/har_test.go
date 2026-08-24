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
	secrets := []string{
		"url-user",
		"url-password",
		"url-access-token",
		"url-fragment-secret",
		"authorization-secret",
		"request-cookie-secret",
		"query-secret",
		"request-body-secret",
		"request-param-secret",
		"set-cookie-secret",
		"response-cookie-secret",
		"response-body-secret",
		"redirect-secret",
		"websocket-extension-secret",
		"initiator-extension-secret",
	}

	input := map[string]any{
		"log": map[string]any{
			"entries": []any{
				map[string]any{
					"_webSocketMessages": []any{
						map[string]any{"data": "websocket-extension-secret"},
					},
					"_initiator": map[string]any{
						"url": "https://example.com/?token=initiator-extension-secret",
					},
					"request": map[string]any{
						"method": "POST",
						"url":    "https://url-user:url-password@play.pocketcasts.com/graphql?access_token=url-access-token#url-fragment-secret",
						"headers": []any{
							map[string]any{"name": "Authorization", "value": "Bearer authorization-secret"},
							map[string]any{"name": "Content-Type", "value": "application/json"},
						},
						"cookies": []any{
							map[string]any{"name": "session", "value": "request-cookie-secret"},
						},
						"queryString": []any{
							map[string]any{"name": "access_token", "value": "query-secret"},
						},
						"postData": map[string]any{
							"mimeType": "application/json",
							"text":     `{"operationName":"Refresh","variables":{"refresh_token":"request-body-secret"}}`,
							"params": []any{
								map[string]any{"name": "refresh_token", "value": "request-param-secret"},
							},
						},
					},
					"response": map[string]any{
						"status": 200,
						"headers": []any{
							map[string]any{"name": "Set-Cookie", "value": "session=set-cookie-secret"},
							map[string]any{"name": "Content-Type", "value": "application/json"},
						},
						"cookies": []any{
							map[string]any{"name": "session", "value": "response-cookie-secret"},
						},
						"content": map[string]any{
							"mimeType": "application/json",
							"text":     `{"access_token":"response-body-secret"}`,
						},
						"redirectURL": "https://play.pocketcasts.com/callback?token=redirect-secret",
					},
					"time": 42,
				},
			},
		},
	}
	writeHARFixture(t, inPath, input)
	if err := os.WriteFile(outPath, []byte("old"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(outPath, 0o666); err != nil {
		t.Fatal(err)
	}

	if err := RedactFile(inPath, outPath, DefaultRedactOptions()); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	assertSecretsAbsent(t, out, secrets)
	s := string(out)
	if strings.Contains(s, "_webSocketMessages") || strings.Contains(s, "_initiator") {
		t.Fatalf("browser extension fields were retained: %s", s)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("output mode = %o, want 600", got)
	}

	redacted, err := ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(redacted.Log.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(redacted.Log.Entries))
	}
	entry := redacted.Log.Entries[0]
	if entry.Request.Method != "POST" || entry.Request.URL != "https://play.pocketcasts.com/graphql" {
		t.Fatalf("request metadata not preserved: %+v", entry.Request)
	}
	if entry.Response.Status != 200 {
		t.Fatalf("response status = %d, want 200", entry.Response.Status)
	}

	summary := Summarize(redacted, SummarizeOptions{Host: "play.pocketcasts.com"})
	if summary.Matched != 1 || len(summary.Endpoints) != 1 || summary.Endpoints[0].Path != "/graphql" {
		t.Fatalf("redacted HAR is not useful for endpoint summaries: %+v", summary)
	}
}

func TestRedactFileFailsClosedForOpaqueBodiesAndInvalidURLs(t *testing.T) {
	tmp := t.TempDir()
	inPath := filepath.Join(tmp, "in.har")
	outPath := filepath.Join(tmp, "out.har")
	input := map[string]any{
		"log": map[string]any{
			"entries": []any{
				map[string]any{
					"request": map[string]any{
						"method": "POST",
						"url":    "file:///tmp/invalid-url-secret",
						"postData": map[string]any{
							"mimeType": "multipart/form-data; boundary=secret",
							"text":     "opaque-request-secret",
						},
					},
					"response": map[string]any{
						"status":      302,
						"redirectURL": "https://%zz/invalid-redirect-secret",
						"content": map[string]any{
							"encoding": "base64",
							"text":     "opaque-response-secret",
						},
					},
				},
			},
		},
	}
	writeHARFixture(t, inPath, input)
	if err := RedactFile(inPath, outPath, RedactOptions{}); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	assertSecretsAbsent(t, out, []string{
		"invalid-url-secret",
		"opaque-request-secret",
		"invalid-redirect-secret",
		"opaque-response-secret",
	})
	redacted, err := ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := redacted.Log.Entries[0].Request.URL; got != defaultReplacement {
		t.Fatalf("non-HTTP URL = %q, want %q", got, defaultReplacement)
	}
}

func TestRedactFileRejectsMalformedHARWithoutReplacingOutput(t *testing.T) {
	tmp := t.TempDir()
	inPath := filepath.Join(tmp, "in.har")
	outPath := filepath.Join(tmp, "out.har")
	if err := os.WriteFile(inPath, []byte(`{"log":{"entries":"not-an-array-with-secret"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, []byte("existing-output"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := RedactFile(inPath, outPath, DefaultRedactOptions())
	if err == nil || !strings.Contains(err.Error(), "log.entries") {
		t.Fatalf("error = %v, want malformed entries error", err)
	}
	out, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(out) != "existing-output" {
		t.Fatalf("output changed after failed redaction: %q", out)
	}
}

func writeHARFixture(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertSecretsAbsent(t *testing.T, output []byte, secrets []string) {
	t.Helper()
	s := string(output)
	for _, secret := range secrets {
		if strings.Contains(s, secret) {
			t.Errorf("secret %q was not redacted: %s", secret, s)
		}
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
