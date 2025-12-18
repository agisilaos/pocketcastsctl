package har

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RedactOptions struct {
	RedactHeaders    map[string]bool
	RedactQueryParms map[string]bool
	RedactJSONKeys   map[string]bool
	Replacement      string
}

func DefaultRedactOptions() RedactOptions {
	return RedactOptions{
		RedactHeaders: map[string]bool{
			"authorization":  true,
			"cookie":         true,
			"x-csrf-token":   true,
			"x-xsrf-token":   true,
			"x-api-key":      true,
			"x-auth-token":   true,
			"x-access-token": true,
		},
		RedactQueryParms: map[string]bool{
			"token":        true,
			"access_token": true,
			"auth":         true,
		},
		RedactJSONKeys: map[string]bool{
			"email":        true,
			"password":     true,
			"token":        true,
			"accessToken":  true,
			"refreshToken": true,
			"session":      true,
			"cookie":       true,
		},
		Replacement: "<redacted>",
	}
}

func RedactFile(inPath, outPath string, opts RedactOptions) error {
	b, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}

	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		return fmt.Errorf("parse HAR: %w", err)
	}

	redactHAR(root, opts)

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, out, 0o600)
}

func redactHAR(root any, opts RedactOptions) {
	m, ok := root.(map[string]any)
	if !ok {
		return
	}
	log, ok := m["log"].(map[string]any)
	if !ok {
		return
	}
	entries, ok := log["entries"].([]any)
	if !ok {
		return
	}

	for _, entry := range entries {
		em, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		req, ok := em["request"].(map[string]any)
		if !ok {
			continue
		}

		redactHeaders(req["headers"], opts)
		redactCookies(req["cookies"], opts)
		redactQuery(req["queryString"], opts)
		redactPostData(req["postData"], opts)
	}
}

func redactHeaders(headers any, opts RedactOptions) {
	hs, ok := headers.([]any)
	if !ok {
		return
	}
	for _, h := range hs {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		name, _ := hm["name"].(string)
		if name == "" {
			continue
		}
		if opts.RedactHeaders[strings.ToLower(strings.TrimSpace(name))] {
			hm["value"] = opts.Replacement
		}
	}
}

func redactCookies(cookies any, opts RedactOptions) {
	cs, ok := cookies.([]any)
	if !ok {
		return
	}
	for _, c := range cs {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := cm["value"]; ok {
			cm["value"] = opts.Replacement
		}
	}
}

func redactQuery(query any, opts RedactOptions) {
	qs, ok := query.([]any)
	if !ok {
		return
	}
	for _, q := range qs {
		qm, ok := q.(map[string]any)
		if !ok {
			continue
		}
		name, _ := qm["name"].(string)
		if name == "" {
			continue
		}
		if opts.RedactQueryParms[strings.ToLower(strings.TrimSpace(name))] {
			qm["value"] = opts.Replacement
		}
	}
}

func redactPostData(postData any, opts RedactOptions) {
	pm, ok := postData.(map[string]any)
	if !ok {
		return
	}
	mime, _ := pm["mimeType"].(string)
	text, _ := pm["text"].(string)
	if text == "" {
		return
	}

	if !strings.Contains(strings.ToLower(mime), "json") {
		return
	}

	var body any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return
	}
	redactJSON(body, opts)
	b, err := json.Marshal(body)
	if err != nil {
		return
	}
	pm["text"] = string(b)
}

func redactJSON(v any, opts RedactOptions) {
	switch vv := v.(type) {
	case map[string]any:
		for k, child := range vv {
			if opts.RedactJSONKeys[k] {
				vv[k] = opts.Replacement
				continue
			}
			redactJSON(child, opts)
		}
	case []any:
		for _, child := range vv {
			redactJSON(child, opts)
		}
	default:
		return
	}
}
