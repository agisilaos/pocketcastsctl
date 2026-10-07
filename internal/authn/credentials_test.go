package authn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pocketcastsctl/internal/config"
)

// Exercise the same token-only persistence contract in memory and in Keychain.
// The native invocation is opt-in, uses a unique key, and always cleans it up.
func testCredentialsRoundTrip(t *testing.T, ctx context.Context, store Store, key string) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := store.Delete(cleanupCtx, key); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	for _, input := range []Credentials{
		{AccessToken: " Bearer synthetic-access ", RefreshToken: " synthetic-refresh "},
		{AccessToken: "synthetic-access-only"},
	} {
		if err := store.Save(ctx, key, input); err != nil {
			t.Fatal(err)
		}
		got, err := store.Load(ctx, key)
		if err != nil || got != input.normalized() {
			t.Fatalf("credentials round trip changed values: error=%v", err)
		}
	}
	if err := store.Save(ctx, key, Credentials{RefreshToken: "orphan-refresh"}); err == nil {
		t.Fatal("saved credentials without access")
	}
	got, err := store.Load(ctx, key)
	if err != nil || got.AccessToken != "synthetic-access-only" || got.RefreshToken != "" {
		t.Fatal("invalid save mutated credentials")
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, key); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing credentials: %v", err)
	}
}

func TestMemoryStoreCredentialsContract(t *testing.T) {
	testCredentialsRoundTrip(t, context.Background(), newMemoryStore(), strings.Repeat("a", 64))
}

func TestManagerReconstructsSavedSession(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	cfg := config.Default()
	cfg.Auth = config.AuthConfig{SessionKey: "active", AccountID: "saved-account", Email: "Saved@Example.com", Method: "password", Scope: ScopeWebPlayer, ExpiresAt: 4102444800}
	for _, jwt := range []bool{false, true} {
		t.Run(map[bool]string{false: "opaque", true: "jwt"}[jwt], func(t *testing.T) {
			token := "opaque-access"
			wantID, wantEmail, wantExpiry := "saved-account", "saved@example.com", int64(4102444800)
			if jwt {
				token = "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"token-account","email":"token@example.com","exp":4202444800}`)) + ".y"
				wantID, wantEmail, wantExpiry = "token-account", "token@example.com", 4202444800
			}
			store := newMemoryStore()
			if err := store.Save(context.Background(), "active", Credentials{AccessToken: token, RefreshToken: "refresh"}); err != nil {
				t.Fatal(err)
			}
			session, err := NewManager(cfg, ManagerOptions{Store: store}).Snapshot(context.Background())
			if err != nil || session.Source != SourceKeychain || session.AccountID != wantID || session.Email != wantEmail || session.ExpiresAt != wantExpiry || session.Method != "password" || session.Scope != ScopeWebPlayer {
				t.Fatalf("reconstructed metadata mismatch: source=%s error=%v", session.Source, err)
			}
		})
	}
}

func TestManagerRefreshPersistenceRecovery(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv(config.EnvAPIBaseURL, "")
	for _, metadataFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "credential save fails", true: "metadata save fails"}[metadataFailure], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "new-access", "refreshToken": "new-refresh", "expiresIn": 3600})
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "config.json")
			t.Setenv(config.EnvConfigPath, path)
			cfg := config.Default()
			cfg.APIBaseURL = server.URL
			cfg.Auth = config.AuthConfig{SessionKey: "active", Email: "saved@example.com", Method: "password", Scope: ScopeWebPlayer}
			writeSavedAuthConfig(t, path, server.URL, cfg.Auth, map[string]string{})
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			store := newMemoryStore()
			old := Credentials{AccessToken: "old-access", RefreshToken: "old-refresh"}
			store.credentials["active"] = old
			if metadataFailure {
				// Force UpdateAuth to fail after issuer validation and credential saving.
				dir := filepath.Dir(path)
				if err := os.Chmod(dir, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			} else {
				store.saveErr = errors.New("credential write failed")
			}
			manager := NewManager(cfg, ManagerOptions{Store: store, HTTP: server.Client(), Now: func() time.Time { return time.Unix(100, 0) }})
			if _, err := manager.ForceRefresh(context.Background()); err == nil {
				t.Fatal("refresh succeeded despite persistence failure")
			}
			snapshot, err := manager.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := old
			if metadataFailure {
				want = Credentials{AccessToken: "new-access", RefreshToken: "new-refresh"}
			}
			if snapshot.Source != SourceKeychain || manager.session.credentials() != want || store.credentials["active"] != want {
				t.Fatal("refresh persistence failure lost recoverable credentials")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Fatal("failed metadata update changed saved config")
			}
			// A new process reconstructs account/method/scope from the old configuration
			// even if credentials were rotated before metadata could be saved.
			reloadedManager := NewManager(cfg, ManagerOptions{Store: store})
			reloaded, err := reloadedManager.Snapshot(context.Background())
			if err != nil || reloadedManager.session.credentials() != want || reloaded.Email != cfg.Auth.Email || reloaded.Method != cfg.Auth.Method || reloaded.Scope != cfg.Auth.Scope {
				t.Fatal("new process could not reconstruct recoverable session")
			}
		})
	}
}

func TestInstallStoreFailureRollsBackOnlyNewKey(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv(config.EnvAPIBaseURL, "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"episodes":[]}`)) }))
	defer server.Close()
	candidate := Session{AccessToken: "candidate", RefreshToken: "refresh", AccountID: "account", Scope: ScopeWebPlayer}
	for _, sameKey := range []bool{false, true} {
		t.Run(map[bool]string{false: "new account", true: "same account"}[sameKey], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			t.Setenv(config.EnvConfigPath, path)
			key := sessionKey(server.URL, candidate)
			oldKey := "old"
			if sameKey {
				oldKey = key
			}
			cfg := config.Default()
			cfg.APIBaseURL = server.URL
			cfg.Auth.SessionKey = oldKey
			writeSavedAuthConfig(t, path, server.URL, cfg.Auth, map[string]string{})
			store := newMemoryStore()
			store.credentials[oldKey] = Credentials{AccessToken: "old-access"}
			store.saveErr = errors.New("write failed")
			updated, err := Install(context.Background(), cfg, store, NewAPI(server.URL, server.Client()), candidate)
			if err == nil || updated.Auth.SessionKey != oldKey || store.credentials[oldKey].AccessToken != "old-access" {
				t.Fatal("store failure lost active session")
			}
			if sameKey && len(store.deletes) != 0 {
				t.Fatal("deleted same-account credentials after save failure")
			}
			if !sameKey && (len(store.deletes) != 1 || store.deletes[0] != key) {
				t.Fatal("new credential key was not cleaned up")
			}
		})
	}
}

func TestLogoutCleanupFailureLeavesConfigDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(config.EnvConfigPath, path)
	cfg := config.Default()
	cfg.Auth.SessionKey = "active"
	writeSavedAuthConfig(t, path, cfg.APIBaseURL, cfg.Auth, map[string]string{"Authorization": "Bearer legacy"})
	store := newMemoryStore()
	store.credentials["active"] = Credentials{AccessToken: "access"}
	store.deleteErr = errors.New("Keychain locked")
	updated, err := Logout(context.Background(), cfg, store)
	if err == nil || updated.Auth.SessionKey != "" || len(updated.APIHeaders) != 0 || store.credentials["active"].AccessToken != "access" {
		t.Fatal("logout cleanup failure did not leave config disabled")
	}
	loaded, err := config.Load()
	if err != nil || loaded.Auth.SessionKey != "" || legacyAuthorization(loaded.APIHeaders) != "" {
		t.Fatal("logout disabled only in memory")
	}
}
