package authn

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
	"time"

	"github.com/steipete/sweetcookie"

	"pocketcastsctl/internal/config"
)

type memoryStore struct {
	sessions  map[string]Session
	saves     int
	deletes   []string
	loadErr   error
	saveErr   error
	deleteErr error
}

func newMemoryStore() *memoryStore {
	return &memoryStore{sessions: map[string]Session{}}
}

func writeSavedAuthConfig(t *testing.T, path, apiBaseURL string, auth config.AuthConfig, headers map[string]string) {
	t.Helper()
	doc := map[string]any{
		"browser":      "chrome",
		"browser_app":  "",
		"url_contains": "pocketcasts.com",
		"api_base_url": apiBaseURL,
		"api_headers":  headers,
		"auth":         auth,
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (s *memoryStore) Load(_ context.Context, key string) (Session, error) {
	if s.loadErr != nil {
		return Session{}, s.loadErr
	}
	session, ok := s.sessions[key]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return session, nil
}

func (s *memoryStore) Save(_ context.Context, key string, session Session) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.sessions[key] = session
	s.saves++
	return nil
}

func (s *memoryStore) Delete(_ context.Context, key string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.sessions, key)
	s.deletes = append(s.deletes, key)
	return nil
}

func TestManagerDoesNotFallBackToPlaintextWhenKeychainSessionFails(t *testing.T) {
	store := newMemoryStore()
	store.loadErr = errors.New("Keychain is locked")
	cfg := config.Default()
	cfg.Auth.SessionKey = strings.Repeat("a", 64)
	cfg.APIHeaders["Authorization"] = "Bearer stale-legacy-token"

	manager := NewManager(cfg, ManagerOptions{Store: store})
	if token, err := manager.AccessToken(context.Background()); token != "" || !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("token=%q error=%v, want unavailable Keychain error", token, err)
	}
	if _, source, _ := manager.Snapshot(context.Background()); source != SourceNone {
		t.Fatalf("source=%q, want none", source)
	}
}

func TestManagerCredentialPrecedence(t *testing.T) {
	store := newMemoryStore()
	store.sessions["active"] = Session{AccessToken: "keychain-token"}
	cfg := config.Default()
	cfg.Auth.SessionKey = "active"
	cfg.APIHeaders["Authorization"] = "Bearer legacy-token"

	t.Setenv(config.EnvAccessToken, "environment-token")
	manager := NewManager(cfg, ManagerOptions{Store: store})
	token, err := manager.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "environment-token" {
		t.Fatalf("token = %q, want environment token", token)
	}
	_, source, _ := manager.Snapshot(context.Background())
	if source != SourceEnvironment {
		t.Fatalf("source = %q, want environment", source)
	}

	t.Setenv(config.EnvAccessToken, "")
	manager = NewManager(cfg, ManagerOptions{Store: store})
	token, err = manager.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "keychain-token" {
		t.Fatalf("token = %q, want keychain token", token)
	}
	_, source, _ = manager.Snapshot(context.Background())
	if source != SourceKeychain {
		t.Fatalf("source = %q, want keychain", source)
	}

	cfg.Auth = config.AuthConfig{}
	manager = NewManager(cfg, ManagerOptions{Store: store})
	token, err = manager.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "legacy-token" || manager.Warning() == "" {
		t.Fatalf("legacy fallback = %q, warning = %q", token, manager.Warning())
	}
	_, source, _ = manager.Snapshot(context.Background())
	if source != SourceLegacy {
		t.Fatalf("source = %q, want legacy", source)
	}
}

func TestNeedsAccountConfirmationTreatsUnknownAsDifferent(t *testing.T) {
	tests := []struct {
		name      string
		currentID string
		email     string
		candidate Session
		want      bool
	}{
		{name: "same id", currentID: "account-1", candidate: Session{AccountID: "account-1"}, want: false},
		{name: "different id", currentID: "account-1", candidate: Session{AccountID: "account-2"}, want: true},
		{name: "same email", email: "Person@Example.com", candidate: Session{Email: "person@example.com"}, want: false},
		{name: "unknown candidate", currentID: "account-1", candidate: Session{}, want: true},
		{name: "unknown current", candidate: Session{AccountID: "account-1"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsAccountConfirmation(tt.currentID, tt.email, tt.candidate); got != tt.want {
				t.Fatalf("NeedsAccountConfirmation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManagerProactivelyRefreshesAndPersistsRotation(t *testing.T) {
	var refreshCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/token" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		refreshCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken":  "new-access",
			"refreshToken": "new-refresh",
			"expiresIn":    3600,
			"tokenType":    "Bearer",
		})
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, configPath)
	store := newMemoryStore()
	store.sessions["active"] = Session{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		Scope:        ScopeWebPlayer,
		ExpiresAt:    time.Now().Add(30 * time.Second).Unix(),
	}
	cfg := config.Default()
	cfg.APIBaseURL = server.URL
	cfg.Auth.SessionKey = "active"
	writeSavedAuthConfig(t, configPath, server.URL, cfg.Auth, map[string]string{})
	manager := NewManager(cfg, ManagerOptions{Store: store, HTTP: server.Client()})

	token, err := manager.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "new-access" || refreshCalls != 1 {
		t.Fatalf("token = %q, refresh calls = %d", token, refreshCalls)
	}
	if got := store.sessions["active"].RefreshToken; got != "new-refresh" {
		t.Fatalf("stored refresh token = %q", got)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("refreshed metadata was not saved: %v", err)
	}
}

func TestManagerRefusesRefreshAgainstTemporaryAPIBase(t *testing.T) {
	var refreshCalls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		refreshCalls++
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, configPath)
	writeSavedAuthConfig(t, configPath, config.Default().APIBaseURL, config.AuthConfig{SessionKey: "active"}, map[string]string{})
	t.Setenv(config.EnvAPIBaseURL, server.URL)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	store := newMemoryStore()
	store.sessions["active"] = Session{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Minute).Unix(),
	}
	manager := NewManager(cfg, ManagerOptions{Store: store, HTTP: server.Client()})

	if _, err := manager.AccessToken(context.Background()); !errors.Is(err, config.ErrAPIBaseURLOverride) {
		t.Fatalf("AccessToken error = %v, want ErrAPIBaseURLOverride", err)
	}
	if refreshCalls != 0 || store.saves != 0 {
		t.Fatalf("refreshCalls=%d saves=%d", refreshCalls, store.saves)
	}
}

func TestAPIValidateRefreshesRejectedCandidate(t *testing.T) {
	var refreshCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/up_next/list":
			if r.Header.Get("Authorization") == "Bearer stale-access" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"episodes":[]}`))
		case "/user/token":
			refreshCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "fresh-access",
				"refreshToken": "rotated-refresh",
				"expiresIn":    3600,
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	validated, err := NewAPI(server.URL, server.Client()).Validate(context.Background(), Session{
		AccessToken:  "stale-access",
		RefreshToken: "old-refresh",
		Scope:        ScopeWebPlayer,
		Method:       "browser-dia",
	})
	if err != nil {
		t.Fatal(err)
	}
	if validated.AccessToken != "fresh-access" || validated.RefreshToken != "rotated-refresh" {
		t.Fatal("validated session was not rotated")
	}
	if refreshCalls != 1 || validated.Method != "browser-dia" {
		t.Fatalf("refresh calls=%d method=%q", refreshCalls, validated.Method)
	}
}

func TestAPIValidateDoesNotRefreshAfterServerFailure(t *testing.T) {
	var refreshCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/up_next/list":
			w.WriteHeader(http.StatusInternalServerError)
		case "/user/token":
			refreshCalls++
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	_, err := NewAPI(server.URL, server.Client()).Validate(context.Background(), Session{
		AccessToken:  "access",
		RefreshToken: "refresh",
		Scope:        ScopeWebPlayer,
	})
	if err == nil || refreshCalls != 0 {
		t.Fatalf("error=%v refresh calls=%d, want server error without refresh", err, refreshCalls)
	}
}

func TestInstallValidatesBeforeReplacingActiveSession(t *testing.T) {
	store := newMemoryStore()
	store.sessions["old"] = Session{AccessToken: "old-access"}
	cfg := config.Default()
	cfg.Auth.SessionKey = "old"
	cfg.APIHeaders["Authorization"] = "Bearer legacy"
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, configPath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer candidate" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"episodes":[]}`))
	}))
	defer server.Close()
	cfg.APIBaseURL = server.URL
	writeSavedAuthConfig(t, configPath, server.URL, cfg.Auth, cfg.APIHeaders)

	updated, err := Install(context.Background(), cfg, store, NewAPI(server.URL, server.Client()), Session{
		AccessToken:  "candidate",
		RefreshToken: "refresh",
		Scope:        ScopeWebPlayer,
		Method:       "password",
		Email:        "person@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Auth.SessionKey == "" || updated.Auth.SessionKey == "old" {
		t.Fatalf("new session key = %q", updated.Auth.SessionKey)
	}
	if _, ok := store.sessions["old"]; ok {
		t.Fatal("old session was not removed after replacement")
	}
	if _, ok := updated.APIHeaders["Authorization"]; ok {
		t.Fatal("legacy Authorization header survived successful install")
	}
}

func TestSessionKeyIsStableAndScoped(t *testing.T) {
	base := Session{AccountID: "account-1", Scope: ScopeWebPlayer}
	key := sessionKey("https://api.pocketcasts.com/", base)
	if len(key) != 64 || key != sessionKey("https://api.pocketcasts.com", base) {
		t.Fatalf("key=%q is not a stable SHA-256 key", key)
	}
	if key == sessionKey("https://other.example", base) {
		t.Fatal("API base URL did not scope the credential key")
	}
	if key == sessionKey("https://api.pocketcasts.com", Session{AccountID: "account-2", Scope: ScopeWebPlayer}) {
		t.Fatal("account identity did not scope the credential key")
	}
}

func TestInstallMetadataFailureKeepsSameAccountCredentialUsable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"episodes":[]}`))
	}))
	defer server.Close()
	candidate := Session{AccessToken: "new-access", AccountID: "account-1", Scope: ScopeWebPlayer}
	key := sessionKey(server.URL, candidate)
	store := newMemoryStore()
	store.sessions[key] = Session{AccessToken: "old-access"}
	cfg := config.Default()
	cfg.APIBaseURL = server.URL
	cfg.Auth = config.AuthConfig{SessionKey: key, AccountID: "account-1", Scope: ScopeWebPlayer}
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.json")
	t.Setenv(config.EnvConfigPath, configPath)
	writeSavedAuthConfig(t, configPath, server.URL, cfg.Auth, map[string]string{})
	if err := os.Chmod(configDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o700) })

	updated, err := Install(context.Background(), cfg, store, NewAPI(server.URL, server.Client()), candidate)
	if err == nil || !strings.Contains(err.Error(), "session installed") {
		t.Fatalf("updated=%+v error=%v", updated.Auth, err)
	}
	if got := store.sessions[key].AccessToken; got != "new-access" {
		t.Fatalf("stored access token=%q, want validated replacement", got)
	}
	if len(store.deletes) != 0 {
		t.Fatalf("active credential was deleted after metadata failure: %v", store.deletes)
	}
}

func TestInstallFailurePreservesActiveSession(t *testing.T) {
	store := newMemoryStore()
	store.sessions["old"] = Session{AccessToken: "old-access"}
	cfg := config.Default()
	cfg.Auth.SessionKey = "old"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()
	cfg.APIBaseURL = server.URL
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, configPath)
	writeSavedAuthConfig(t, configPath, server.URL, cfg.Auth, map[string]string{})

	_, err := Install(context.Background(), cfg, store, NewAPI(server.URL, server.Client()), Session{AccessToken: "rejected"})
	if err == nil {
		t.Fatal("Install succeeded with a rejected token")
	}
	if got := store.sessions["old"].AccessToken; got != "old-access" {
		t.Fatalf("old token = %q", got)
	}
	if store.saves != 0 || len(store.deletes) != 0 {
		t.Fatalf("store mutated before validation: saves=%d deletes=%v", store.saves, store.deletes)
	}
}

func TestLogoutDisablesConfigBeforeDeletingKeychainSession(t *testing.T) {
	store := newMemoryStore()
	store.sessions["old"] = Session{AccessToken: "old-access"}
	cfg := config.Default()
	cfg.Auth.SessionKey = "old"
	cfg.APIHeaders["Authorization"] = "Bearer legacy"

	// Saving to an existing directory fails at the final atomic rename.
	t.Setenv(config.EnvConfigPath, t.TempDir())
	if _, err := Logout(context.Background(), cfg, store); err == nil {
		t.Fatal("Logout succeeded despite config save failure")
	}
	if _, ok := store.sessions["old"]; !ok {
		t.Fatal("Keychain session was deleted before config was safely disabled")
	}
	if len(store.deletes) != 0 {
		t.Fatalf("delete calls=%v, want none", store.deletes)
	}
}

func TestLogoutClearsSavedSessionUnderTemporaryAPIBase(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, configPath)
	writeSavedAuthConfig(t, configPath, config.Default().APIBaseURL, config.AuthConfig{SessionKey: "old"}, map[string]string{"Authorization": "Bearer legacy"})
	t.Setenv(config.EnvAPIBaseURL, "https://temporary.example")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	store := newMemoryStore()
	store.sessions["old"] = Session{AccessToken: "old-access"}

	if _, err := Logout(context.Background(), cfg, store); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.sessions["old"]; ok {
		t.Fatal("logout retained the saved Keychain session")
	}
	b, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "auth") || strings.Contains(strings.ToLower(string(b)), "authorization") {
		t.Fatalf("logout retained saved auth data: %s", b)
	}
}

type fixtureBrowserReader struct {
	profiles []string
	values   map[string][]string
}

func (r fixtureBrowserReader) Profiles(string) ([]string, error) { return r.profiles, nil }
func (r fixtureBrowserReader) Read(_ context.Context, _, profile string) ([]string, []string, error) {
	values, ok := r.values[profile]
	if !ok {
		return nil, nil, errors.New("profile unavailable")
	}
	return values, nil, nil
}

func TestBrowserCandidatesFixturesForAllSupportedBrowsers(t *testing.T) {
	cookie := url.PathEscape(`{"accessToken":"fixture-access","refreshToken":"fixture-refresh","expiresIn":3600,"tokenType":"Bearer"}`)
	for _, browser := range []string{"chrome", "dia", "safari"} {
		t.Run(browser, func(t *testing.T) {
			profile := "Default"
			if browser == "safari" {
				profile = ""
			}
			reader := fixtureBrowserReader{profiles: []string{profile}, values: map[string][]string{profile: {cookie}}}
			candidates, warnings, err := BrowserCandidates(context.Background(), reader, browser, "", time.Unix(100, 0))
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) != 0 || len(candidates) != 1 {
				t.Fatalf("candidates=%d warnings=%v", len(candidates), warnings)
			}
			if got := candidates[0].Session.Method; got != "browser-"+browser {
				t.Fatalf("method = %q", got)
			}
		})
	}
}

func TestSweetCookieReaderMapsEverySupportedBrowserExplicitly(t *testing.T) {
	want := map[string]sweetcookie.Browser{
		"chrome": sweetcookie.BrowserChrome,
		"dia":    sweetcookie.BrowserDia,
		"safari": sweetcookie.BrowserSafari,
	}
	for browser, source := range want {
		t.Run(browser, func(t *testing.T) {
			reader := &SweetCookieReader{get: func(_ context.Context, opts sweetcookie.Options) (sweetcookie.Result, error) {
				if len(opts.Browsers) != 1 || opts.Browsers[0] != source {
					t.Fatalf("browser options = %v, want %s", opts.Browsers, source)
				}
				if opts.URL != pocketCastsCookieURL || len(opts.Names) != 1 || opts.Names[0] != "auth" {
					t.Fatalf("cookie filter was not exact: %+v", opts)
				}
				return sweetcookie.Result{Cookies: []sweetcookie.Cookie{{Name: "auth", Value: "fixture"}}}, nil
			}}
			values, _, err := reader.Read(context.Background(), browser, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(values) != 1 || values[0] != "fixture" {
				t.Fatalf("values = %v", values)
			}
		})
	}
}

func TestBrowserCandidatesRetainMultipleProfiles(t *testing.T) {
	cookie := url.PathEscape(`{"accessToken":"fixture-access","refreshToken":"fixture-refresh"}`)
	reader := fixtureBrowserReader{
		profiles: []string{"Default", "Profile 1"},
		values: map[string][]string{
			"Default":   {cookie},
			"Profile 1": {cookie},
		},
	}
	candidates, _, err := BrowserCandidates(context.Background(), reader, "dia", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
}

func TestDecodeBrowserCookieRejectsMissingToken(t *testing.T) {
	_, err := DecodeBrowserCookie(url.PathEscape(`{"refreshToken":"only-refresh"}`), time.Now())
	if err == nil || !strings.Contains(err.Error(), "access token") {
		t.Fatalf("error = %v", err)
	}
}

func TestBrowserRecoveryHintIsActionableWithoutRawPaths(t *testing.T) {
	tests := []struct {
		browser  string
		warning  string
		contains string
	}{
		{browser: "safari", warning: "open /Users/person/Library/Cookies: operation not permitted", contains: "Full Disk Access"},
		{browser: "chrome", warning: `sweetcookie: chrome profile "Default" not found`, contains: "install Chrome"},
		{browser: "dia", warning: "macOS keychain read failed", contains: "allow Keychain access"},
	}
	for _, tt := range tests {
		hint := BrowserRecoveryHint(tt.browser, []string{tt.warning})
		if !strings.Contains(hint, tt.contains) {
			t.Fatalf("hint = %q, want %q", hint, tt.contains)
		}
		if strings.Contains(hint, "/Users/person") {
			t.Fatalf("raw path leaked into hint: %q", hint)
		}
	}
}
