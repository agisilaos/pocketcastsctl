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
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/config"
)

type commandMemoryStore struct {
	credentials map[string]authn.Credentials
	loads       int
	saves       int
	deletes     int
}

func newCommandMemoryStore() *commandMemoryStore {
	return &commandMemoryStore{credentials: map[string]authn.Credentials{}}
}

func (s *commandMemoryStore) Load(_ context.Context, key string) (authn.Credentials, error) {
	s.loads++
	credentials, ok := s.credentials[key]
	if !ok {
		return authn.Credentials{}, authn.ErrSessionNotFound
	}
	credentials.AccessToken = authutil.NormalizeToken(credentials.AccessToken)
	credentials.RefreshToken = strings.TrimSpace(credentials.RefreshToken)
	if credentials.AccessToken == "" {
		return authn.Credentials{}, errors.New("API session in Keychain has no access token")
	}
	return credentials, nil
}

func (s *commandMemoryStore) Save(_ context.Context, key string, credentials authn.Credentials) error {
	credentials.AccessToken = authutil.NormalizeToken(credentials.AccessToken)
	credentials.RefreshToken = strings.TrimSpace(credentials.RefreshToken)
	if credentials.AccessToken == "" {
		return errors.New("cannot store an API session without an access token")
	}
	s.saves++
	s.credentials[key] = credentials
	return nil
}

func (s *commandMemoryStore) Delete(_ context.Context, key string) error {
	s.deletes++
	delete(s.credentials, key)
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

	configureAPIBaseURLForTest(t, server.URL)
	code, stdout, stderr := runForTest(t, []string{"auth", "login", "--email", "person@example.com", "--password-stdin", "--json"}, "very-secret\n")
	if code != 0 {
		t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, secret := range []string{"very-secret", "secret-access-token", "secret-refresh-token"} {
		if strings.Contains(stdout, secret) || strings.Contains(stderr, secret) {
			t.Fatalf("secret %q leaked in output", secret)
		}
	}
	if len(store.credentials) != 1 {
		t.Fatalf("stored credentials = %d, want 1", len(store.credentials))
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
	configureAPIBaseURLForTest(t, server.URL)

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
	if len(store.credentials) != 1 {
		t.Fatalf("stored credentials = %d, want 1", len(store.credentials))
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
	configureAPIBaseURLForTest(t, server.URL)

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
	store.credentials["active"] = authn.Credentials{AccessToken: "secret-access"}
	cfg := config.Default()
	cfg.Auth = config.AuthConfig{SessionKey: "active", Method: "password"}
	cfg.APIHeaders["Authorization"] = "Bearer legacy-secret"
	writeEffectiveConfigForTest(t, cfg)

	code, stdout, stderr := runForTest(t, []string{"auth", "logout", "--json"}, "")
	if code != 0 {
		t.Fatalf("exit code = %d; stdout=%q stderr=%q", code, stdout, stderr)
	}
	if len(store.credentials) != 0 {
		t.Fatalf("%d session(s) remain after logout", len(store.credentials))
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
	store.credentials["active"] = authn.Credentials{AccessToken: "access"}
	cfg := config.Default()
	cfg.Auth = config.AuthConfig{
		SessionKey: "active",
		AccountID:  "account-1",
		Email:      "person@example.com",
		Method:     "password",
		Scope:      authn.ScopeWebPlayer,
		ExpiresAt:  4102444800,
	}
	writeEffectiveConfigForTest(t, cfg)

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

// A malformed Store implementation must not make an empty successful load
// appear configured to any observation caller.
type emptySuccessfulCommandStore struct{ authn.Store }

func (emptySuccessfulCommandStore) Load(context.Context, string) (authn.Credentials, error) {
	return authn.Credentials{RefreshToken: "hidden-refresh"}, nil
}

func TestEmptyStoredCredentialsReportMissingAcrossAuthCallers(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	store := useCommandMemoryStore(t)
	credentialStoreFactory = func() authn.Store { return emptySuccessfulCommandStore{Store: store} }
	cfg := config.Default()
	cfg.Auth = config.AuthConfig{SessionKey: "active", Email: "saved@example.com", Method: "password"}
	cfg.APIHeaders["Authorization"] = "Bearer hidden-legacy"
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.Path(), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"--json", "--plain", ""} {
		args := []string{"auth", "status"}
		if mode != "" {
			args = append(args, mode)
		}
		code, stdout, stderr := runForTest(t, args, "")
		if code != 0 || !strings.Contains(stdout, "active Keychain session is unavailable") {
			t.Fatalf("mode=%q code=%d stdout=%q stderr=%q", mode, code, stdout, stderr)
		}
		if mode == "--json" {
			var status map[string]any
			if err := json.Unmarshal([]byte(stdout), &status); err != nil {
				t.Fatal(err)
			}
			if status["authorization_present"] != false || status["source"] != "none" || status["token_expiry_known"] != false {
				t.Fatalf("status=%v", status)
			}
		}
		for _, secret := range []string{"hidden-refresh", "hidden-legacy", "saved@example.com"} {
			if strings.Contains(stdout+stderr, secret) {
				t.Fatalf("failed observation leaked %q", secret)
			}
		}
	}
	if setupAuthConfigured(cfg) {
		t.Fatal("setup treated empty credentials as configured")
	}
	if _, err := sessionReplacementPreflight(cfg); !errors.Is(err, authn.ErrCredentialUnavailable) {
		t.Fatalf("replacement error=%v", err)
	}
	foundSession := false
	for _, check := range collectDoctorChecks(cfg, false) {
		if check.ID == "api_session" {
			foundSession = true
		}
		if check.ID == "api_session" && (check.Status != "warn" || check.Code != "doctor.auth.session_missing" || !strings.Contains(check.Message, "active Keychain session is unavailable")) {
			t.Fatalf("doctor check=%+v", check)
		}
	}
	if !foundSession {
		t.Fatal("doctor omitted API session diagnostic")
	}
}

func TestAuthRefreshOutputsUpdatedMetadataWithoutCredentials(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv(config.EnvAPIBaseURL, "")
	for _, mode := range []string{"--json", "--plain", ""} {
		t.Run(mode, func(t *testing.T) {
			store := useCommandMemoryStore(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/user/token" {
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(404)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["refresh_token"] != "old-refresh-secret" {
					t.Error("refresh credential was not used")
				}
				_, _ = w.Write([]byte(`{"accessToken":"new-access-secret","refreshToken":"new-refresh-secret","email":"refreshed@example.com","expiresIn":3600}`))
			}))
			defer server.Close()
			t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
			cfg := config.Default()
			cfg.APIBaseURL = server.URL
			cfg.Auth = config.AuthConfig{SessionKey: "active", Email: "old@example.com", Method: "password", Scope: authn.ScopeWebPlayer}
			writeEffectiveConfigForTest(t, cfg)
			store.credentials["active"] = authn.Credentials{AccessToken: "old-access-secret", RefreshToken: "old-refresh-secret"}
			args := []string{"auth", "refresh"}
			if mode != "" {
				args = append(args, mode)
			}
			code, stdout, stderr := runForTest(t, args, "")
			if code != 0 || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			for _, secret := range []string{"old-access-secret", "old-refresh-secret", "new-access-secret", "new-refresh-secret"} {
				if strings.Contains(stdout+stderr, secret) {
					t.Fatalf("rendered credentials: %q", stdout+stderr)
				}
			}
			saved, err := config.Load()
			if err != nil || saved.Auth.Email != "refreshed@example.com" || saved.Auth.ExpiresAt <= 0 || store.credentials["active"].AccessToken != "new-access-secret" {
				t.Fatalf("refresh state=%+v err=%v", saved.Auth, err)
			}
			switch mode {
			case "--json":
				var result map[string]any
				if err := json.Unmarshal([]byte(stdout), &result); err != nil {
					t.Fatal(err)
				}
				if len(result) != 6 || result["status"] != "ok" || result["command"] != "auth refresh" || result["email"] != saved.Auth.Email || result["expires_at"] != float64(saved.Auth.ExpiresAt) || result["method"] != "password" || result["scope"] != authn.ScopeWebPlayer {
					t.Fatalf("JSON=%v", result)
				}
			case "--plain":
				if stdout != "status\tok\ncommand\tauth refresh\nmethod\tpassword\nscope\twebplayer\n" {
					t.Fatalf("plain=%q", stdout)
				}
			default:
				if stdout != "auth refresh: OK\nsession: password (webplayer)\n" {
					t.Fatalf("human=%q", stdout)
				}
			}
		})
	}
}
