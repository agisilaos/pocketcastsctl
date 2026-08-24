package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

type commandMemoryStore struct {
	sessions map[string]authn.Session
}

func newCommandMemoryStore() *commandMemoryStore {
	return &commandMemoryStore{sessions: map[string]authn.Session{}}
}

func (s *commandMemoryStore) Load(_ context.Context, key string) (authn.Session, error) {
	session, ok := s.sessions[key]
	if !ok {
		return authn.Session{}, authn.ErrSessionNotFound
	}
	return session, nil
}

func (s *commandMemoryStore) Save(_ context.Context, key string, session authn.Session) error {
	s.sessions[key] = session
	return nil
}

func (s *commandMemoryStore) Delete(_ context.Context, key string) error {
	delete(s.sessions, key)
	return nil
}

func useCommandMemoryStore(t *testing.T) *commandMemoryStore {
	t.Helper()
	store := newCommandMemoryStore()
	previous := credentialStoreFactory
	credentialStoreFactory = func() authn.Store { return store }
	t.Cleanup(func() { credentialStoreFactory = previous })
	return store
}

func TestAuthLoginUsesTerminalExchangeAndDoesNotLeakSecrets(t *testing.T) {
	store := useCommandMemoryStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/login_pocket_casts":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["email"] != "person@example.com" || body["password"] != "very-secret" || body["scope"] != "webplayer" {
				t.Fatalf("unexpected login body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "secret-access-token",
				"refreshToken": "secret-refresh-token",
				"expiresIn":    3600,
				"tokenType":    "Bearer",
			})
		case "/up_next/list":
			if got := r.Header.Get("Authorization"); got != "Bearer secret-access-token" {
				t.Fatalf("Authorization = %q", got)
			}
			_, _ = w.Write([]byte(`{"episodes":[]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(config.EnvAPIBaseURL, server.URL)
	code, stdout, stderr := runForTest(t, []string{"auth", "login", "--email", "person@example.com", "--password-stdin", "--json"}, "very-secret\n")
	if code != 0 {
		t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, secret := range []string{"very-secret", "secret-access-token", "secret-refresh-token"} {
		if strings.Contains(stdout, secret) || strings.Contains(stderr, secret) {
			t.Fatalf("secret %q leaked in output", secret)
		}
	}
	if len(store.sessions) != 1 {
		t.Fatalf("stored sessions = %d, want 1", len(store.sessions))
	}
	rawConfig, err := os.ReadFile(config.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawConfig), "secret-access-token") || strings.Contains(string(rawConfig), "secret-refresh-token") {
		t.Fatalf("secret leaked into config: %s", rawConfig)
	}
}

func TestAuthLoginJSONMissingInputIsStructuredUsageError(t *testing.T) {
	code, stdout, stderr := runForTest(t, []string{"auth", "login", "--json"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stdout, `"code": "auth.input.email_missing"`) {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr = %q", stderr)
	}
}

type commandBrowserReader struct {
	profiles []string
	values   map[string][]string
}

func (r commandBrowserReader) Profiles(string) ([]string, error) { return r.profiles, nil }
func (r commandBrowserReader) Read(_ context.Context, _, profile string) ([]string, []string, error) {
	values, ok := r.values[profile]
	if !ok {
		return nil, nil, errors.New("profile unavailable")
	}
	return values, nil, nil
}

func useCommandBrowserReader(t *testing.T, reader authn.BrowserReader) {
	t.Helper()
	previous := browserReaderFactory
	browserReaderFactory = func() authn.BrowserReader { return reader }
	t.Cleanup(func() { browserReaderFactory = previous })
}

func TestAuthImportBrowserIsExplicitAndDoesNotLeakCookie(t *testing.T) {
	store := useCommandMemoryStore(t)
	cookie := url.PathEscape(`{"accessToken":"cookie-secret-access","refreshToken":"cookie-secret-refresh","expiresIn":3600,"tokenType":"Bearer"}`)
	useCommandBrowserReader(t, commandBrowserReader{profiles: []string{"Default"}, values: map[string][]string{"Default": {cookie}}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/up_next/list" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"episodes":[]}`))
	}))
	defer server.Close()
	t.Setenv(config.EnvAPIBaseURL, server.URL)

	code, stdout, stderr := runForTest(t, []string{"auth", "import-browser", "--browser", "dia", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"browser": "dia"`) || !strings.Contains(stdout, `"profile": "Default"`) {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout+stderr, "cookie-secret") {
		t.Fatal("browser credential leaked in command output")
	}
	if len(store.sessions) != 1 {
		t.Fatalf("stored sessions = %d, want 1", len(store.sessions))
	}
}

func TestAuthImportBrowserRequiresProfileWhenSeveralAreValidNonInteractive(t *testing.T) {
	useCommandMemoryStore(t)
	cookie := url.PathEscape(`{"accessToken":"cookie-access","refreshToken":"cookie-refresh"}`)
	useCommandBrowserReader(t, commandBrowserReader{
		profiles: []string{"Default", "Profile 1"},
		values: map[string][]string{
			"Default":   {cookie},
			"Profile 1": {cookie},
		},
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"episodes":[]}`))
	}))
	defer server.Close()
	t.Setenv(config.EnvAPIBaseURL, server.URL)

	code, stdout, stderr := runForTest(t, []string{"auth", "import-browser", "--browser", "dia", "--json"}, "")
	if code != 2 {
		t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "multiple signed-in profiles") || !strings.Contains(stdout, "--profile") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestAuthLogoutRemovesKeychainAndLegacyCredential(t *testing.T) {
	store := useCommandMemoryStore(t)
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	store.sessions["active"] = authn.Session{AccessToken: "secret-access"}
	cfg := config.Default()
	cfg.Auth = config.AuthConfig{SessionKey: "active", Method: "password"}
	cfg.APIHeaders["Authorization"] = "Bearer legacy-secret"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runForTest(t, []string{"auth", "logout", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if len(store.sessions) != 0 {
		t.Fatalf("%d session(s) remain after logout", len(store.sessions))
	}
	updated, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if updated.Auth.SessionKey != "" {
		t.Fatalf("active session key remains: %q", updated.Auth.SessionKey)
	}
	if _, ok := updated.APIHeaders["Authorization"]; ok {
		t.Fatal("legacy Authorization header remains after logout")
	}
}

func TestAuthStatusReportsAccountMethodScopeAndExpiry(t *testing.T) {
	store := useCommandMemoryStore(t)
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	store.sessions["active"] = authn.Session{
		AccessToken: "access",
		AccountID:   "account-1",
		Email:       "person@example.com",
		Method:      "password",
		Scope:       authn.ScopeWebPlayer,
		ExpiresAt:   4102444800,
	}
	cfg := config.Default()
	cfg.Auth = config.AuthConfig{
		SessionKey: "active",
		AccountID:  "account-1",
		Email:      "person@example.com",
		Method:     "password",
		Scope:      authn.ScopeWebPlayer,
		ExpiresAt:  4102444800,
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runForTest(t, []string{"auth", "status", "--json"}, "")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	for _, field := range []string{`"account_id": "account-1"`, `"email": "person@example.com"`, `"method": "password"`, `"scope": "webplayer"`, `"token_expiry_unix": 4102444800`} {
		if !strings.Contains(stdout, field) {
			t.Fatalf("stdout missing %s: %s", field, stdout)
		}
	}
}

func TestAuthLogoutReportsEnvironmentOverride(t *testing.T) {
	useCommandMemoryStore(t)
	t.Setenv(config.EnvAccessToken, "process-only-token")
	code, stdout, stderr := runForTest(t, []string{"auth", "logout", "--json"}, "")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, config.EnvAccessToken) || strings.Contains(stdout, "process-only-token") {
		t.Fatalf("logout warning missing or leaked token: %s", stdout)
	}
}
