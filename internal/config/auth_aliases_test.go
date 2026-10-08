package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClearAuthRemovesEveryAcceptedFieldSpelling(t *testing.T) {
	t.Setenv(EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	writeRawConfig(t, `{
		"Auth":{"Session_Key":"saved-session","future_provider":"provider-state"},
		"AUTH":{"Email":"person@example.com"},
		"API_HEADERS":{"Authorization":"Bearer upper-secret","X-Upper":"keep"},
		"api_headers":{"aUtHoRiZaTiOn":"Bearer lower-secret","X-Lower":"keep"},
		"future":{"enabled":true}
	}`)

	if _, err := ClearAuth(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth != (AuthConfig{}) {
		t.Fatalf("logout retained saved authentication: %+v", loaded.Auth)
	}
	for key := range loaded.APIHeaders {
		if strings.EqualFold(key, "Authorization") {
			t.Fatalf("logout retained an Authorization header under %q", key)
		}
	}
	doc := readRawConfig(t)
	for _, key := range []string{"API_HEADERS", "api_headers"} {
		headers := doc[key].(map[string]any)
		if len(headers) != 1 {
			t.Fatalf("logout did not preserve only the unrelated header in %s: %#v", key, headers)
		}
	}
	if _, ok := doc["future"]; !ok {
		t.Fatal("logout discarded unrelated configuration")
	}
}

func TestUpdateAuthHandlesAcceptedMetadataAliases(t *testing.T) {
	t.Setenv(EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	writeRawConfig(t, `{"Auth":{"Session_Key":"old","Email":"old@example.com"},"future":true}`)
	want := AuthConfig{SessionKey: "new", Method: "password"}
	if _, err := UpdateAuth(Default().APIBaseURL, want); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil || loaded.Auth != want {
		t.Fatalf("replacement retained old metadata: auth=%+v error=%v", loaded.Auth, err)
	}

	writeRawConfig(t, `{"Auth":{"Session_Key":"old","future_provider":"keep"},"future":true}`)
	before, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateAuth(Default().APIBaseURL, want); !errors.Is(err, ErrUnknownAuthFields) {
		t.Fatalf("replacement error=%v, want unsupported auth fields", err)
	}
	after, err := os.ReadFile(Path())
	if err != nil || string(after) != string(before) {
		t.Fatal("refused replacement changed unknown authentication metadata")
	}
}
