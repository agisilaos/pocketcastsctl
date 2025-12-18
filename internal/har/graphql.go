package har

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type GraphQLOpsOptions struct {
	Host string
}

type GraphQLOpsSummary struct {
	HostFilter string       `json:"hostFilter"`
	Total      int          `json:"total"`
	Matched    int          `json:"matched"`
	Ops        []GraphQLOp  `json:"ops"`
	Unknown    []GraphQLHit `json:"unknown,omitempty"`
}

type GraphQLOp struct {
	OperationName string   `json:"operationName"`
	Path          string   `json:"path"`
	Count         int      `json:"count"`
	VariableKeys  []string `json:"variableKeys,omitempty"`
}

type GraphQLHit struct {
	Path string `json:"path"`
}

func GraphQLOpsFile(path string, opts GraphQLOpsOptions) (GraphQLOpsSummary, error) {
	f, err := ReadFile(path)
	if err != nil {
		return GraphQLOpsSummary{}, err
	}
	return GraphQLOps(f, opts), nil
}

func GraphQLOps(f File, opts GraphQLOpsOptions) GraphQLOpsSummary {
	endpoints := Summarize(f, SummarizeOptions{Host: opts.Host})

	type key struct {
		op   string
		path string
	}
	counts := map[key]*GraphQLOp{}
	unknown := map[string]bool{}

	for _, e := range f.Log.Entries {
		raw := strings.TrimSpace(e.Request.URL)
		if raw == "" {
			continue
		}

		if opts.Host != "" && !strings.Contains(strings.ToLower(raw), strings.ToLower(opts.Host)) {
			continue
		}

		if e.Request.PostData == nil {
			continue
		}
		if !strings.Contains(strings.ToLower(e.Request.PostData.MimeType), "json") {
			continue
		}

		var body map[string]any
		if err := json.Unmarshal([]byte(e.Request.PostData.Text), &body); err != nil {
			continue
		}

		opName, _ := body["operationName"].(string)
		if opName == "" {
			unknown[raw] = true
			continue
		}

		varKeys := extractTopLevelKeys(body["variables"])
		u, err := parseURL(raw)
		if err != nil {
			continue
		}
		k := key{op: opName, path: u.EscapedPath()}
		item := counts[k]
		if item == nil {
			item = &GraphQLOp{
				OperationName: opName,
				Path:          k.path,
				VariableKeys:  varKeys,
			}
			counts[k] = item
		}
		item.Count++
		item.VariableKeys = unionKeys(item.VariableKeys, varKeys)
	}

	out := make([]GraphQLOp, 0, len(counts))
	for _, v := range counts {
		sort.Strings(v.VariableKeys)
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].OperationName < out[j].OperationName
	})

	var unknownOut []GraphQLHit
	for raw := range unknown {
		u, err := parseURL(raw)
		if err != nil {
			continue
		}
		unknownOut = append(unknownOut, GraphQLHit{Path: u.EscapedPath()})
	}
	sort.Slice(unknownOut, func(i, j int) bool { return unknownOut[i].Path < unknownOut[j].Path })

	return GraphQLOpsSummary{
		HostFilter: endpoints.HostFilter,
		Total:      endpoints.Total,
		Matched:    endpoints.Matched,
		Ops:        out,
		Unknown:    unknownOut,
	}
}

func FormatGraphQLOpsText(s GraphQLOpsSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Total entries: %d\n", s.Total)
	if s.HostFilter != "" {
		fmt.Fprintf(&b, "Matched host %q: %d\n", s.HostFilter, s.Matched)
	} else {
		fmt.Fprintf(&b, "Matched: %d\n", s.Matched)
	}
	if len(s.Ops) == 0 {
		b.WriteString("\nNo GraphQL operations found (by operationName).\n")
		return b.String()
	}
	b.WriteString("\nGraphQL operations:\n")
	for _, op := range s.Ops {
		varKeys := ""
		if len(op.VariableKeys) > 0 {
			varKeys = " vars=" + strings.Join(op.VariableKeys, ",")
		}
		fmt.Fprintf(&b, "- %s %s (%d)%s\n", op.Path, op.OperationName, op.Count, varKeys)
	}
	if len(s.Unknown) > 0 {
		b.WriteString("\nGraphQL-like JSON without operationName:\n")
		seen := map[string]bool{}
		for _, u := range s.Unknown {
			if seen[u.Path] {
				continue
			}
			seen[u.Path] = true
			fmt.Fprintf(&b, "- %s\n", u.Path)
		}
	}
	return b.String()
}

func extractTopLevelKeys(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func unionKeys(a, b []string) []string {
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]bool, len(a)+len(b))
	for _, k := range a {
		seen[k] = true
	}
	for _, k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func parseURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}
