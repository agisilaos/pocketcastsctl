package har

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

type File struct {
	Log Log `json:"log"`
}

type Log struct {
	Entries []Entry `json:"entries"`
}

type Entry struct {
	Request  Request  `json:"request"`
	Response Response `json:"response"`
}

type Request struct {
	Method   string       `json:"method"`
	URL      string       `json:"url"`
	Headers  []Header     `json:"headers"`
	Cookies  []Cookie     `json:"cookies"`
	PostData *PostData    `json:"postData,omitempty"`
	Query    []QueryParam `json:"queryString,omitempty"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type QueryParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type PostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type Response struct {
	Status  int      `json:"status"`
	Headers []Header `json:"headers"`
	Body    []byte   `json:"-"`
}

func ReadFile(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, fmt.Errorf("parse HAR: %w", err)
	}
	return f, nil
}

type SummarizeOptions struct {
	Host string
}

type Summary struct {
	HostFilter string          `json:"hostFilter"`
	Total      int             `json:"total"`
	Matched    int             `json:"matched"`
	Endpoints  []EndpointCount `json:"endpoints"`
}

type EndpointCount struct {
	Method string   `json:"method"`
	Host   string   `json:"host"`
	Path   string   `json:"path"`
	Count  int      `json:"count"`
	Hints  []string `json:"hints,omitempty"`
}

func SummarizeFile(path string, opts SummarizeOptions) (Summary, error) {
	f, err := ReadFile(path)
	if err != nil {
		return Summary{}, err
	}
	return Summarize(f, opts), nil
}

func Summarize(f File, opts SummarizeOptions) Summary {
	hostNeedle := strings.TrimSpace(opts.Host)

	type key struct {
		method string
		host   string
		path   string
	}
	counts := map[key]*EndpointCount{}

	total := len(f.Log.Entries)
	matched := 0

	for _, e := range f.Log.Entries {
		r, ok := analyzeRequest(e.Request, hostNeedle)
		if !ok {
			continue
		}
		matched++

		k := key{method: r.method, host: r.host, path: r.path}
		ec := counts[k]
		if ec == nil {
			ec = &EndpointCount{Method: k.method, Host: k.host, Path: k.path}
			counts[k] = ec
		}
		ec.Count++

		if hasHeader(e.Request.Headers, "authorization") {
			ec.Hints = addHint(ec.Hints, "authz")
		}
		if hasHeader(e.Request.Headers, "cookie") || len(e.Request.Cookies) > 0 {
			ec.Hints = addHint(ec.Hints, "cookie")
		}
		if hasHeader(e.Request.Headers, "x-csrf-token") || hasHeader(e.Request.Headers, "x-xsrf-token") {
			ec.Hints = addHint(ec.Hints, "csrf")
		}
		if r.jsonBody {
			ec.Hints = addHint(ec.Hints, "json")
			if r.graphql != notGraphQL {
				ec.Hints = addHint(ec.Hints, "graphql")
			}
		}
	}

	out := make([]EndpointCount, 0, len(counts))
	for _, v := range counts {
		sort.Strings(v.Hints)
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})

	return Summary{
		HostFilter: hostNeedle,
		Total:      total,
		Matched:    matched,
		Endpoints:  out,
	}
}

func FormatSummaryText(s Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Total entries: %d\n", s.Total)
	if s.HostFilter != "" {
		fmt.Fprintf(&b, "Matched host %q: %d\n", s.HostFilter, s.Matched)
	} else {
		fmt.Fprintf(&b, "Matched: %d\n", s.Matched)
	}
	if len(s.Endpoints) == 0 {
		return b.String()
	}
	b.WriteString("\nEndpoints:\n")
	for _, e := range s.Endpoints {
		hints := ""
		if len(e.Hints) > 0 {
			hints = " [" + strings.Join(e.Hints, ",") + "]"
		}
		fmt.Fprintf(&b, "- %s %s %s%s (%d)\n", e.Host, e.Method, e.Path, hints, e.Count)
	}
	return b.String()
}

func hasHeader(hs []Header, nameLower string) bool {
	for _, h := range hs {
		if strings.ToLower(strings.TrimSpace(h.Name)) == nameLower {
			return true
		}
	}
	return false
}

func addHint(hints []string, h string) []string {
	for _, existing := range hints {
		if existing == h {
			return hints
		}
	}
	return append(hints, h)
}

// Request analysis is deliberately separate from redaction's fail-closed traversal.
type graphqlKind uint8

const (
	notGraphQL graphqlKind = iota
	unnamedGraphQL
	namedGraphQL
)

type analyzedRequest struct {
	method        string
	host          string
	path          string
	jsonBody      bool
	graphql       graphqlKind
	operationName string
	variableKeys  []string
}

func analyzeRequest(req Request, hostFilter string) (analyzedRequest, bool) {
	raw := strings.TrimSpace(req.URL)
	if raw == "" {
		return analyzedRequest{}, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return analyzedRequest{}, false
	}
	host := strings.ToLower(u.Hostname())
	filter := strings.ToLower(strings.TrimSpace(hostFilter))
	if filter != "" && host != filter && !strings.HasSuffix(host, "."+filter) {
		return analyzedRequest{}, false
	}
	r := analyzedRequest{
		method: strings.ToUpper(strings.TrimSpace(req.Method)),
		host:   host,
		path:   u.EscapedPath(),
	}
	if req.PostData == nil || !strings.Contains(strings.ToLower(req.PostData.MimeType), "json") {
		return r, true
	}
	r.jsonBody = true
	// Only single JSON objects are supported; batch arrays are not operations.
	var body map[string]any
	if err := json.Unmarshal([]byte(req.PostData.Text), &body); err != nil {
		return r, true
	}
	opName, _ := body["operationName"].(string)
	query, _ := body["query"].(string)
	switch {
	case strings.TrimSpace(opName) != "":
		r.graphql = namedGraphQL
		r.operationName = opName
		r.variableKeys = extractTopLevelKeys(body["variables"])
	case strings.TrimSpace(query) != "":
		r.graphql = unnamedGraphQL
	}
	return r, true
}
