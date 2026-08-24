package har

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const defaultReplacement = "<redacted>"

type RedactOptions struct {
	Replacement string
}

func DefaultRedactOptions() RedactOptions {
	return RedactOptions{Replacement: defaultReplacement}
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

	replacement := opts.Replacement
	if replacement == "" {
		replacement = defaultReplacement
	}
	dropExtensionFields(root)
	if err := redactHAR(root, replacement); err != nil {
		return fmt.Errorf("redact HAR: %w", err)
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return writeRedactedFile(outPath, out)
}

func dropExtensionFields(v any) {
	switch value := v.(type) {
	case map[string]any:
		for key, child := range value {
			// HAR reserves underscore-prefixed keys for browser extensions. Their
			// schemas are not stable enough to redact safely, so omit them.
			if strings.HasPrefix(key, "_") {
				delete(value, key)
				continue
			}
			dropExtensionFields(child)
		}
	case []any:
		for _, child := range value {
			dropExtensionFields(child)
		}
	}
}

func writeRedactedFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".pocketcastsctl-har-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func redactHAR(root any, replacement string) error {
	m, ok := root.(map[string]any)
	if !ok {
		return fmt.Errorf("root is %T, want object", root)
	}
	log, ok := m["log"].(map[string]any)
	if !ok {
		return fmt.Errorf("log is %T, want object", m["log"])
	}
	entries, ok := log["entries"].([]any)
	if !ok {
		return fmt.Errorf("log.entries is %T, want array", log["entries"])
	}

	for i, entry := range entries {
		em, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf("log.entries[%d] is %T, want object", i, entry)
		}
		request, ok := em["request"].(map[string]any)
		if !ok {
			return fmt.Errorf("log.entries[%d].request is %T, want object", i, em["request"])
		}
		if err := redactRequest(request, replacement); err != nil {
			return fmt.Errorf("log.entries[%d].request: %w", i, err)
		}
		response, ok := em["response"].(map[string]any)
		if !ok {
			return fmt.Errorf("log.entries[%d].response is %T, want object", i, em["response"])
		}
		if err := redactResponse(response, replacement); err != nil {
			return fmt.Errorf("log.entries[%d].response: %w", i, err)
		}
	}
	return nil
}

func redactRequest(request map[string]any, replacement string) error {
	// HARs duplicate sensitive values across raw URLs, structured fields, and
	// bodies. Redact every value at those boundaries instead of classifying
	// token names, which is easy to bypass with aliases such as refresh_token.
	if err := redactURLField(request, "url", replacement); err != nil {
		return err
	}
	for _, key := range []string{"headers", "cookies", "queryString"} {
		if err := redactValues(request, key, replacement); err != nil {
			return err
		}
	}

	postDataValue, exists := request["postData"]
	if !exists || postDataValue == nil {
		return nil
	}
	postData, ok := postDataValue.(map[string]any)
	if !ok {
		return fmt.Errorf("postData is %T, want object", postDataValue)
	}
	redactText(postData, replacement)
	return redactValues(postData, "params", replacement)
}

func redactResponse(response map[string]any, replacement string) error {
	if err := redactURLField(response, "redirectURL", replacement); err != nil {
		return err
	}
	for _, key := range []string{"headers", "cookies"} {
		if err := redactValues(response, key, replacement); err != nil {
			return err
		}
	}
	contentValue, exists := response["content"]
	if !exists || contentValue == nil {
		return nil
	}
	content, ok := contentValue.(map[string]any)
	if !ok {
		return fmt.Errorf("content is %T, want object", contentValue)
	}
	redactText(content, replacement)
	return nil
}

func redactURLField(m map[string]any, key, replacement string) error {
	value, exists := m[key]
	if !exists || value == nil {
		return nil
	}
	raw, ok := value.(string)
	if !ok {
		return fmt.Errorf("%s is %T, want string", key, value)
	}
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) || u.Host == "" {
		m[key] = replacement
		return nil
	}
	// Parameter names remain available in queryString; the raw URL only needs
	// the origin and path for safe endpoint diagnostics.
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	m[key] = u.String()
	return nil
}

func redactValues(m map[string]any, key, replacement string) error {
	value, exists := m[key]
	if !exists || value == nil {
		return nil
	}
	values, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s is %T, want array", key, value)
	}
	for i, item := range values {
		entry, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("%s[%d] is %T, want object", key, i, item)
		}
		if _, ok := entry["value"]; ok {
			entry["value"] = replacement
		}
	}
	return nil
}

func redactText(m map[string]any, replacement string) {
	if _, ok := m["text"]; ok {
		m["text"] = replacement
	}
}
