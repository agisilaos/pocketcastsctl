package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	ErrConfigExists        = errors.New("config file already exists")
	ErrAPIBaseURLOverride  = errors.New("saved API sessions cannot be changed while the API base URL is overridden")
	ErrUnknownAuthFields   = errors.New("config contains unsupported auth fields")
	ErrDurabilityUncertain = errors.New("config was replaced but directory synchronization failed")
)

// SavedConfig is the known, persisted portion of a config document. Pointer
// fields distinguish an absent saved value from an explicitly saved zero value.
// Unknown document fields remain private and are not exposed by LoadSaved.
type SavedConfig struct {
	Browser     *string            `json:"browser,omitempty"`
	BrowserApp  *string            `json:"browser_app,omitempty"`
	URLContains *string            `json:"url_contains,omitempty"`
	APIBaseURL  *string            `json:"api_base_url,omitempty"`
	APIHeaders  *map[string]string `json:"api_headers,omitempty"`
	Auth        *AuthConfig        `json:"auth,omitempty"`
}

// BrowserUpdate describes the persisted browser fields a command explicitly
// chose to change. Nil fields are left untouched.
type BrowserUpdate struct {
	Browser    *string
	BrowserApp *string
}

type document map[string]json.RawMessage

var (
	renameConfigFile = os.Rename
	syncConfigDir    = func(path string) error {
		dir, err := os.Open(path)
		if err != nil {
			return err
		}
		defer dir.Close()
		return dir.Sync()
	}
)

func LoadSaved() (SavedConfig, error) {
	doc, exists, err := readDocument()
	if err != nil {
		return SavedConfig{}, err
	}
	if !exists {
		return SavedConfig{}, fmt.Errorf("read %s: %w", Path(), os.ErrNotExist)
	}

	var saved SavedConfig
	if err := decodeKnown(doc, &saved); err != nil {
		return SavedConfig{}, fmt.Errorf("parse %s: %w", Path(), err)
	}
	return saved, nil
}

// Init writes the canonical default document. Existing files are preserved
// unless force is true.
func Init(force bool) error {
	if !force {
		if _, err := os.Stat(Path()); err == nil {
			return fmt.Errorf("%w: %s", ErrConfigExists, Path())
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	doc, err := defaultDocument()
	if err != nil {
		return err
	}
	return writeDocument(doc)
}

// UpdateBrowser persists only the explicitly selected browser fields.
func UpdateBrowser(update BrowserUpdate) (Config, error) {
	return updateDocument(func(doc document) error {
		if update.Browser != nil {
			if err := setDocumentValue(doc, "browser", *update.Browser); err != nil {
				return err
			}
		}
		if update.BrowserApp != nil {
			if err := setDocumentValue(doc, "browser_app", *update.BrowserApp); err != nil {
				return err
			}
		}
		return nil
	})
}

// ValidateAuthUpdate fails before issuer-bound network or Keychain work when
// the saved document cannot safely accept auth metadata from apiBaseURL.
func ValidateAuthUpdate(apiBaseURL string) error {
	doc, err := readDocumentForUpdate()
	if err != nil {
		return err
	}
	return validateAuthUpdate(doc, apiBaseURL)
}

// UpdateAuth replaces known auth metadata and removes a legacy plaintext
// Authorization header without changing unrelated persisted settings.
func UpdateAuth(apiBaseURL string, auth AuthConfig) (Config, error) {
	return updateDocument(func(doc document) error {
		if err := validateAuthUpdate(doc, apiBaseURL); err != nil {
			return err
		}
		if err := setDocumentValue(doc, "auth", auth); err != nil {
			return err
		}
		return removeAuthorizationHeader(doc)
	})
}

// ClearAuth removes the complete auth object and any legacy plaintext
// Authorization header. Explicit logout is allowed to discard unknown auth
// fields so older binaries cannot leave a newer saved session active.
func ClearAuth() (Config, error) {
	return updateDocument(func(doc document) error {
		delete(doc, "auth")
		return removeAuthorizationHeader(doc)
	})
}

func decodeKnown(doc document, target any) error {
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

func readDocument() (document, bool, error) {
	b, err := os.ReadFile(Path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var doc document
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", Path(), err)
	}
	if doc == nil {
		return nil, false, fmt.Errorf("parse %s: top-level config must be an object", Path())
	}
	return doc, true, nil
}

func readDocumentForUpdate() (document, error) {
	doc, exists, err := readDocument()
	if err != nil {
		return nil, err
	}
	if exists {
		if _, err := resolve(doc, false); err != nil {
			return nil, err
		}
		return doc, nil
	}
	return defaultDocument()
}

func updateDocument(change func(document) error) (Config, error) {
	doc, err := readDocumentForUpdate()
	if err != nil {
		return Config{}, err
	}
	if err := change(doc); err != nil {
		return Config{}, err
	}
	if err := writeDocument(doc); err != nil {
		if errors.Is(err, ErrDurabilityUncertain) {
			cfg, loadErr := Load()
			if loadErr == nil {
				return cfg, err
			}
		}
		return Config{}, err
	}
	return Load()
}

func defaultDocument() (document, error) {
	b, err := json.Marshal(Default())
	if err != nil {
		return nil, err
	}
	var doc document
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func setDocumentValue(doc document, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	doc[key] = b
	return nil
}

func validateAuthUpdate(doc document, apiBaseURL string) error {
	saved, err := resolve(doc, false)
	if err != nil {
		return err
	}
	if NormalizeAPIBaseURL(saved.APIBaseURL) != NormalizeAPIBaseURL(apiBaseURL) {
		return fmt.Errorf("%w: effective %q, saved %q", ErrAPIBaseURLOverride, apiBaseURL, saved.APIBaseURL)
	}
	unknown, err := unknownAuthFields(doc)
	if err != nil {
		return err
	}
	if len(unknown) != 0 {
		return fmt.Errorf("%w: %s", ErrUnknownAuthFields, strings.Join(unknown, ", "))
	}
	return nil
}

func unknownAuthFields(doc document) ([]string, error) {
	raw, ok := doc["auth"]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("parse %s auth: %w", Path(), err)
	}
	known := map[string]bool{
		"session_key": true,
		"account_id":  true,
		"email":       true,
		"method":      true,
		"scope":       true,
		"expires_at":  true,
	}
	var unknown []string
	for key := range fields {
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown, nil
}

func removeAuthorizationHeader(doc document) error {
	raw, ok := doc["api_headers"]
	if !ok || string(raw) == "null" {
		return nil
	}
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return fmt.Errorf("parse %s api_headers: %w", Path(), err)
	}
	for key := range headers {
		if strings.EqualFold(strings.TrimSpace(key), "Authorization") {
			delete(headers, key)
		}
	}
	return setDocumentValue(doc, "api_headers", headers)
}

func writeDocument(doc document) error {
	p := Path()
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
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
	if err := renameConfigFile(tmpPath, p); err != nil {
		return err
	}
	if err := syncConfigDir(dir); err != nil {
		return fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
	}
	return nil
}
