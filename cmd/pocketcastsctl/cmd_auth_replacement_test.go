package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

func TestSessionReplacementPreflightUsesResolvedSession(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")

	t.Run("keychain session overrides stale metadata and legacy", func(t *testing.T) {
		store := useCommandMemoryStore(t)
		store.sessions["active"] = authn.Session{
			AccessToken: "keychain-token",
			AccountID:   "resolved-account",
			Email:       "resolved@example.com",
		}
		cfg := config.Default()
		cfg.Auth = config.AuthConfig{
			SessionKey: "active",
			AccountID:  "stale-account",
			Email:      "stale@example.com",
		}
		cfg.APIHeaders["Authorization"] = "Bearer legacy-token"

		current, err := sessionReplacementPreflight(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if current.AccountID != "resolved-account" || current.Email != "resolved@example.com" {
			t.Fatalf("resolved account = (%q, %q)", current.AccountID, current.Email)
		}
		if err := confirmSessionReplacement(current, authn.Session{AccountID: "resolved-account"}, false, false); err != nil {
			t.Fatalf("same resolved account requires confirmation: %v", err)
		}
		if store.loads != 1 {
			t.Fatalf("credential-store loads = %d, want 1", store.loads)
		}
	})

	t.Run("legacy JWT supplies account identity", func(t *testing.T) {
		useCommandMemoryStore(t)
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"account-1","email":"Person@Example.com"}`))
		cfg := config.Default()
		cfg.APIHeaders["Authorization"] = "Bearer x." + payload + ".y"

		current, err := sessionReplacementPreflight(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if current.AccountID != "account-1" || current.Email != "person@example.com" {
			t.Fatalf("resolved legacy account = (%q, %q)", current.AccountID, current.Email)
		}
		if err := confirmSessionReplacement(current, authn.Session{AccountID: "account-1"}, false, false); err != nil {
			t.Fatalf("same legacy account requires confirmation: %v", err)
		}
	})

	t.Run("missing configured session fails closed", func(t *testing.T) {
		store := useCommandMemoryStore(t)
		cfg := config.Default()
		cfg.Auth.SessionKey = "missing"

		_, err := sessionReplacementPreflight(cfg)
		if !errors.Is(err, authn.ErrCredentialUnavailable) {
			t.Fatalf("error = %v, want ErrCredentialUnavailable", err)
		}
		if store.loads != 1 {
			t.Fatalf("credential-store loads = %d, want 1", store.loads)
		}
	})
}

func TestConfirmSessionReplacementUsesResolvedAccount(t *testing.T) {
	tests := []struct {
		name      string
		current   authn.Session
		candidate authn.Session
		force     bool
		wantError bool
	}{
		{
			name:      "no active session",
			candidate: authn.Session{AccountID: "candidate"},
		},
		{
			name:      "same account ID",
			current:   authn.Session{AccessToken: "current", AccountID: "account-1"},
			candidate: authn.Session{AccountID: "account-1"},
		},
		{
			name:      "same normalized email",
			current:   authn.Session{AccessToken: "current", Email: "person@example.com"},
			candidate: authn.Session{Email: "Person@Example.com"},
		},
		{
			name:      "different account requires force",
			current:   authn.Session{AccessToken: "current", AccountID: "account-1"},
			candidate: authn.Session{AccountID: "account-2"},
			wantError: true,
		},
		{
			name:      "unknown current identity requires force",
			current:   authn.Session{AccessToken: "opaque-current"},
			candidate: authn.Session{AccountID: "account-2"},
			wantError: true,
		},
		{
			name:      "force permits different account",
			current:   authn.Session{AccessToken: "current", AccountID: "account-1"},
			candidate: authn.Session{AccountID: "account-2"},
			force:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := confirmSessionReplacement(tt.current, tt.candidate, tt.force, false)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, wantError = %v", err, tt.wantError)
			}
		})
	}
}

func TestAuthCommandsRefuseEnvironmentOverrideBeforeCandidateWork(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		browser bool
	}{
		{
			name: "login",
			args: []string{"auth", "login", "--email", "candidate@example.com", "--password-stdin", "--force", "--no-input", "--json"},
		},
		{
			name:    "browser import",
			args:    []string{"auth", "import-browser", "--browser", "dia", "--profile", "Default", "--force", "--no-input", "--json"},
			browser: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := useCommandMemoryStore(t)
			store.sessions["dormant"] = authn.Session{AccessToken: "dormant-token", Email: "saved@example.com"}
			configPath := filepath.Join(t.TempDir(), "config.json")
			t.Setenv(config.EnvConfigPath, configPath)

			var apiCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				apiCalls.Add(1)
			}))
			defer server.Close()
			configureAPIBaseURLForTest(t, server.URL)
			t.Setenv(config.EnvAccessToken, "environment-secret")

			cfg := config.Default()
			cfg.Auth = config.AuthConfig{SessionKey: "dormant", Email: "saved@example.com"}
			cfg.APIHeaders["Authorization"] = "Bearer dormant-legacy"
			writeEffectiveConfigForTest(t, cfg)
			before, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}

			reader := &countingBrowserReader{}
			if tt.browser {
				useCommandBrowserReader(t, reader)
			}

			code, stdout, stderr := runForTest(t, tt.args, "")
			if code != 2 || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if !strings.Contains(stdout, `"code": "auth.source.environment_override"`) || !strings.Contains(stdout, config.EnvAccessToken) {
				t.Fatalf("environment override error missing from stdout: %s", stdout)
			}
			if strings.Contains(stdout, "environment-secret") {
				t.Fatal("environment credential leaked in output")
			}
			if apiCalls.Load() != 0 || reader.calls != 0 {
				t.Fatalf("candidate work occurred: api calls=%d browser calls=%d", apiCalls.Load(), reader.calls)
			}
			if store.loads != 0 || store.saves != 0 || store.deletes != 0 {
				t.Fatalf("credential store touched: loads=%d saves=%d deletes=%d", store.loads, store.saves, store.deletes)
			}
			if got := store.sessions["dormant"].AccessToken; got != "dormant-token" {
				t.Fatalf("dormant credential = %q", got)
			}
			after, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("config changed on refusal:\n%s", after)
			}
		})
	}
}

func TestAuthLoginReplacementUsesResolvedKeychainAccount(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")

	tests := []struct {
		name         string
		currentEmail string
		force        bool
		wantCode     int
		wantError    bool
	}{
		{name: "same account", currentEmail: "candidate@example.com", wantCode: 0},
		{name: "different account no input", currentEmail: "current@example.com", wantCode: 2, wantError: true},
		{name: "different account forced", currentEmail: "current@example.com", force: true, wantCode: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := useCommandMemoryStore(t)
			store.sessions["active"] = authn.Session{AccessToken: "current-token", Email: tt.currentEmail}
			t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))

			var loginCalls, validationCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/user/login_pocket_casts":
					loginCalls.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "candidate-token", "refreshToken": "candidate-refresh"})
				case "/up_next/list":
					validationCalls.Add(1)
					_, _ = w.Write([]byte(`{"episodes":[]}`))
				default:
					t.Errorf("unexpected path %q", r.URL.Path)
				}
			}))
			defer server.Close()
			configureAPIBaseURLForTest(t, server.URL)

			cfg := config.Default()
			cfg.APIBaseURL = server.URL
			cfg.Auth = config.AuthConfig{SessionKey: "active", Email: "candidate@example.com"}
			writeEffectiveConfigForTest(t, cfg)

			args := []string{"auth", "login", "--email", "candidate@example.com", "--password-stdin", "--no-input", "--json"}
			if tt.force {
				args = append(args, "--force")
			}
			code, stdout, stderr := runForTest(t, args, "secret\n")
			if code != tt.wantCode || stderr != "" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if tt.wantError != strings.Contains(stdout, `"code": "auth.account.replace_required"`) {
				t.Fatalf("replacement error mismatch: %s", stdout)
			}
			if store.loads != 1 {
				t.Fatalf("credential-store loads = %d, want 1", store.loads)
			}
			if loginCalls.Load() != 1 {
				t.Fatalf("login calls = %d, want 1", loginCalls.Load())
			}
			if tt.wantError {
				if validationCalls.Load() != 0 || store.saves != 0 || store.deletes != 0 {
					t.Fatalf("replacement refusal mutated state: validation=%d saves=%d deletes=%d", validationCalls.Load(), store.saves, store.deletes)
				}
			} else if validationCalls.Load() != 1 || store.saves != 1 {
				t.Fatalf("successful replacement work: validation=%d saves=%d", validationCalls.Load(), store.saves)
			}
		})
	}
}

func TestAuthImportBrowserReplacementUsesResolvedKeychainAccount(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	store := useCommandMemoryStore(t)
	store.sessions["active"] = authn.Session{AccessToken: "current-token", Email: "current@example.com"}
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))

	cookie := url.PathEscape(`{"accessToken":"candidate-token","email":"candidate@example.com"}`)
	useCommandBrowserReader(t, commandBrowserReader{
		profiles: []string{"Default"},
		values:   map[string][]string{"Default": {cookie}},
	})

	var validationCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/up_next/list" {
			t.Errorf("unexpected path %q", r.URL.Path)
			return
		}
		validationCalls.Add(1)
		_, _ = w.Write([]byte(`{"episodes":[]}`))
	}))
	defer server.Close()
	configureAPIBaseURLForTest(t, server.URL)

	cfg := config.Default()
	cfg.APIBaseURL = server.URL
	cfg.Auth = config.AuthConfig{SessionKey: "active", Email: "candidate@example.com"}
	writeEffectiveConfigForTest(t, cfg)

	code, stdout, stderr := runForTest(t, []string{"auth", "import-browser", "--browser", "dia", "--profile", "Default", "--no-input", "--json"}, "")
	if code != 2 || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"code": "auth.account.replace_required"`) {
		t.Fatalf("replacement error missing from stdout: %s", stdout)
	}
	if store.loads != 1 || store.saves != 0 || store.deletes != 0 || validationCalls.Load() != 1 {
		t.Fatalf("unexpected work: loads=%d saves=%d deletes=%d validation=%d", store.loads, store.saves, store.deletes, validationCalls.Load())
	}
}

func TestAuthLoginFailsClosedWhenSavedSessionCannotResolve(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	store := useCommandMemoryStore(t)
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))

	var apiCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		apiCalls.Add(1)
	}))
	defer server.Close()
	configureAPIBaseURLForTest(t, server.URL)

	cfg := config.Default()
	cfg.Auth.SessionKey = "missing"
	writeEffectiveConfigForTest(t, cfg)

	code, stdout, stderr := runForTest(t, []string{"auth", "login", "--email", "candidate@example.com", "--password-stdin", "--json"}, "secret\n")
	if code != 1 || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"code": "auth.session.resolve_failed"`) {
		t.Fatalf("resolve error missing from stdout: %s", stdout)
	}
	if !strings.Contains(stdout, "restore Keychain access") || !strings.Contains(stdout, "pocketcastsctl auth logout") {
		t.Fatalf("recovery guidance missing from stdout: %s", stdout)
	}
	if apiCalls.Load() != 0 || store.loads != 1 || store.saves != 0 || store.deletes != 0 {
		t.Fatalf("unexpected work: api=%d loads=%d saves=%d deletes=%d", apiCalls.Load(), store.loads, store.saves, store.deletes)
	}
}

type countingBrowserReader struct {
	calls int
}

func (r *countingBrowserReader) Profiles(string) ([]string, error) {
	r.calls++
	return []string{"Default"}, nil
}

func (r *countingBrowserReader) Read(context.Context, string, string) ([]string, []string, error) {
	r.calls++
	return nil, nil, nil
}
