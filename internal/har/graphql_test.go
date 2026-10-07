package har

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRequestHostFilteringIsShared(t *testing.T) {
	urls := []string{
		" https://API.POCKETCASTS.COM:443/exact ",
		"https://edge.api.pocketcasts.com/subdomain",
		"https://example.com/api.pocketcasts.com/path",
		"https://example.com/query?host=api.pocketcasts.com",
		"https://api.pocketcasts.com.example.com/suffix",
		"https://notapi.pocketcasts.com/prefix",
		"https://api.pocketcasts.com@example.com/userinfo",
		"https://api.pocketcasts.com/escaped%20path",
		"https://%zz/invalid",
		"https://api.pocketcasts.com/bad%zz",
		"/relative",
		"file:///tmp/local",
		" ",
	}
	f := File{}
	for _, u := range urls {
		f.Log.Entries = append(f.Log.Entries, Entry{Request: Request{
			Method: " post ", URL: u,
			PostData: &PostData{MimeType: "application/json", Text: `{"operationName":"Inspect"}`},
		}})
	}
	for _, tc := range []struct {
		filter string
		paths  []string
	}{
		{" API.POCKETCASTS.COM ", []string{"/escaped%20path", "/exact", "/subdomain"}},
		{"pocketcasts.com", []string{"/escaped%20path", "/exact", "/prefix", "/subdomain"}},
		{"", []string{"/api.pocketcasts.com/path", "/escaped%20path", "/exact", "/prefix", "/query", "/subdomain", "/suffix", "/userinfo"}},
		{"missing.example", []string{}},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			s := Summarize(f, SummarizeOptions{Host: tc.filter})
			g := GraphQLOps(f, GraphQLOpsOptions{Host: tc.filter})
			if s.Total != len(urls) || g.Total != s.Total || s.Matched != len(tc.paths) || g.Matched != s.Matched {
				t.Fatalf("inconsistent counts: summary=%+v graphql=%+v", s, g)
			}
			if s.HostFilter != strings.TrimSpace(tc.filter) || g.HostFilter != s.HostFilter {
				t.Fatalf("host filters: %q, %q", s.HostFilter, g.HostFilter)
			}
			paths := make([]string, 0, len(g.Ops))
			for _, op := range g.Ops {
				paths = append(paths, op.Path)
				if op.Count != 1 || op.OperationName != "Inspect" {
					t.Fatalf("unexpected operation: %+v", op)
				}
			}
			if !reflect.DeepEqual(paths, tc.paths) {
				t.Fatalf("operation paths=%v want %v", paths, tc.paths)
			}
			for _, ep := range s.Endpoints {
				if ep.Method != "POST" || ep.Host != strings.ToLower(ep.Host) || ep.Count != 1 {
					t.Fatalf("endpoint not normalized: %+v", ep)
				}
			}
		})
	}
}

func TestGraphQLClassificationIsShared(t *testing.T) {
	for _, tc := range []struct {
		name, mime, body string
		kind             graphqlKind
	}{
		{"ordinary JSON", "application/json", `{"episode":"synthetic"}`, notGraphQL},
		{"variables only", "application/json", `{"variables":{"id":1}}`, notGraphQL},
		{"invalid JSON", "application/json", `{"query":`, notGraphQL},
		{"null", "application/json", `null`, notGraphQL},
		{"scalar", "application/json", `"query"`, notGraphQL},
		{"batch unsupported", "application/json", `[{"operationName":"Named","query":"{ viewer }"}]`, notGraphQL},
		{"wrong field types", "application/json", `{"query":42,"operationName":true,"variables":{}}`, notGraphQL},
		{"empty fields", "application/json", `{"query":" ","operationName":"","variables":{}}`, notGraphQL},
		{"unnamed query", "application/json", `{"query":"{ viewer }","variables":{"id":"synthetic"}}`, unnamedGraphQL},
		{"unnamed with invalid name", "application/json", `{"query":"{ viewer }","operationName":42}`, unnamedGraphQL},
		{"named query", "application/json", `{"operationName":"Named","query":"query Named { viewer }"}`, namedGraphQL},
		{"persisted named operation", "application/json", `{"operationName":"Named","variables":{"id":"synthetic"}}`, namedGraphQL},
		{"name only", "APPLICATION/JSON; charset=utf-8", `{"operationName":"Named"}`, namedGraphQL},
		{"non JSON MIME", "text/plain", `{"operationName":"Named"}`, notGraphQL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := File{Log: Log{Entries: []Entry{{Request: Request{
				Method: "POST", URL: "https://api.pocketcasts.com/graphql",
				PostData: &PostData{MimeType: tc.mime, Text: tc.body},
			}}}}}
			s := Summarize(f, SummarizeOptions{})
			g := GraphQLOps(f, GraphQLOpsOptions{})
			if s.Matched != 1 || g.Matched != 1 || len(s.Endpoints) != 1 {
				t.Fatalf("valid request excluded: summary=%+v graphql=%+v", s, g)
			}
			hasGraphQL := false
			for _, hint := range s.Endpoints[0].Hints {
				hasGraphQL = hasGraphQL || hint == "graphql"
			}
			if hasGraphQL != (tc.kind != notGraphQL) || (len(g.Ops) == 1) != (tc.kind == namedGraphQL) || (len(g.Unknown) == 1) != (tc.kind == unnamedGraphQL) {
				t.Fatalf("classification mismatch: summary=%+v graphql=%+v", s, g)
			}
		})
	}
}

func TestHARAnalysisAggregationAndDeterminism(t *testing.T) {
	f := File{}
	for _, tc := range []struct{ url, body string }{
		{"https://api.pocketcasts.com/z", `{"operationName":"Zed","variables":{"z":"synthetic-z","a":1}}`},
		{"https://API.POCKETCASTS.COM/z", `{"operationName":"Zed","variables":{"b":"synthetic-b"}}`},
		{"https://api.pocketcasts.com/a", `{"operationName":"Beta","variables":[]}`},
		{"https://api.pocketcasts.com/a", `{"operationName":"Alpha"}`},
		{"https://api.pocketcasts.com/unnamed?q=1", `{"query":"{ viewer }"}`},
		{"https://api.pocketcasts.com/unnamed?q=2", `{"query":"{ viewer }"}`},
	} {
		f.Log.Entries = append(f.Log.Entries, Entry{Request: Request{
			Method: "POST", URL: tc.url,
			Headers:  []Header{{Name: "Authorization"}, {Name: "X-CSRF-Token"}},
			Cookies:  []Cookie{{Name: "session"}},
			PostData: &PostData{MimeType: "application/json", Text: tc.body},
		}})
	}
	s := Summarize(f, SummarizeOptions{})
	g := GraphQLOps(f, GraphQLOpsOptions{})
	wantOps := []GraphQLOp{
		{OperationName: "Alpha", Path: "/a", Count: 1},
		{OperationName: "Beta", Path: "/a", Count: 1},
		{OperationName: "Zed", Path: "/z", Count: 2, VariableKeys: []string{"a", "b", "z"}},
	}
	if !reflect.DeepEqual(g.Ops, wantOps) || !reflect.DeepEqual(g.Unknown, []GraphQLHit{{Path: "/unnamed"}}) {
		t.Fatalf("unexpected GraphQL aggregation: %+v", g)
	}
	for _, ep := range s.Endpoints {
		if ep.Count != 2 || !reflect.DeepEqual(ep.Hints, []string{"authz", "cookie", "csrf", "graphql", "json"}) {
			t.Fatalf("unexpected endpoint aggregation: %+v", ep)
		}
	}
	before, err := json.Marshal([]any{s, g})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(before), "synthetic-") || strings.Contains(string(before), "viewer") {
		t.Fatalf("body values printed: %s", before)
	}
	for i, j := 0, len(f.Log.Entries)-1; i < j; i, j = i+1, j-1 {
		f.Log.Entries[i], f.Log.Entries[j] = f.Log.Entries[j], f.Log.Entries[i]
	}
	for i := 0; i < 10; i++ {
		after, err := json.Marshal([]any{Summarize(f, SummarizeOptions{}), GraphQLOps(f, GraphQLOpsOptions{})})
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("nondeterministic output: before=%s after=%s", before, after)
		}
	}
}

func TestFormatGraphQLOpsTextShowsUnnamedWithoutNamedOperations(t *testing.T) {
	s := GraphQLOpsSummary{Total: 1, Matched: 1, Unknown: []GraphQLHit{{Path: "/graphql"}}}
	text := FormatGraphQLOpsText(s)
	if !strings.Contains(text, "GraphQL-like JSON without operationName:") || !strings.Contains(text, "- /graphql\n") {
		t.Fatalf("unnamed query missing from text output: %s", text)
	}
}
