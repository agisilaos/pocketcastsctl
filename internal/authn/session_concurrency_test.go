package authn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"pocketcastsctl/internal/config"
)

type concurrentCredentialStore struct {
	mu     sync.Mutex
	values map[string]Credentials
}

func (s *concurrentCredentialStore) Load(_ context.Context, key string) (Credentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return Credentials{}, ErrSessionNotFound
	}
	return value, nil
}

func (s *concurrentCredentialStore) Save(_ context.Context, key string, value Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *concurrentCredentialStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

func TestRefreshDoesNotRestoreAChangedSession(t *testing.T) {
	for _, action := range []string{"logout", "different account", "same account"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv(config.EnvAccessToken, "")
			t.Setenv(config.EnvAPIBaseURL, "")
			t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/user/token" {
					close(started)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					fmt.Fprint(w, `{"accessToken":"rotated-access","refreshToken":"rotated-refresh","expiresIn":3600}`)
					return
				}
				fmt.Fprint(w, `{"episodes":[]}`)
			}))
			defer server.Close()
			store := &concurrentCredentialStore{values: make(map[string]Credentials)}
			cfg := config.Default()
			cfg.APIBaseURL = server.URL
			writeSavedAuthConfig(t, config.Path(), server.URL, config.AuthConfig{}, nil)
			api := NewAPI(server.URL, server.Client())
			candidate := Session{AccountID: "original-account", AccessToken: "original-access", RefreshToken: "original-refresh", Scope: ScopeWebPlayer}
			cfg, err := Install(ctx, cfg, store, api, candidate)
			if err != nil {
				t.Fatal(err)
			}
			manager := NewManager(cfg, ManagerOptions{Store: store, HTTP: server.Client()})
			finished := make(chan error, 1)
			go func() { _, err := manager.ForceRefresh(ctx); finished <- err }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}

			if action == "logout" {
				_, err = Logout(ctx, cfg, store)
			} else {
				candidate.AccessToken, candidate.RefreshToken = "replacement-access", "replacement-refresh"
				if action == "different account" {
					candidate.AccountID = "replacement-account"
				}
				_, err = Install(ctx, cfg, store, api, candidate)
			}
			if err != nil {
				t.Fatal(err)
			}
			unblock()
			if err := <-finished; err == nil {
				t.Fatal("stale refresh succeeded after the active session changed")
			}
			loaded, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			active := NewManager(loaded, ManagerOptions{Store: store})
			token, err := active.AccessToken(ctx)
			if action == "logout" {
				if !errors.Is(err, ErrNotConfigured) {
					t.Fatalf("refresh restored a logged-out session: error=%v", err)
				}
			} else if err != nil || token != "replacement-access" {
				t.Fatalf("refresh replaced the newly installed session: token=%q error=%v", token, err)
			}
		})
	}
}

func TestInstallDoesNotRestoreSessionLoggedOutDuringValidation(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv(config.EnvAPIBaseURL, "")
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer replacement-access" {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprint(w, `{"episodes":[]}`)
	}))
	defer server.Close()
	defer unblock()
	store := &concurrentCredentialStore{values: make(map[string]Credentials)}
	cfg := config.Default()
	cfg.APIBaseURL = server.URL
	writeSavedAuthConfig(t, config.Path(), server.URL, config.AuthConfig{}, nil)
	api := NewAPI(server.URL, server.Client())
	candidate := Session{AccountID: "original-account", AccessToken: "original-access", RefreshToken: "original-refresh", Scope: ScopeWebPlayer}
	cfg, err := Install(ctx, cfg, store, api, candidate)
	if err != nil {
		t.Fatal(err)
	}
	candidate.AccessToken, candidate.RefreshToken = "replacement-access", "replacement-refresh"
	finished := make(chan error, 1)
	go func() { _, err := Install(ctx, cfg, store, api, candidate); finished <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := Logout(ctx, cfg, store); err != nil {
		t.Fatal(err)
	}
	unblock()
	if err := <-finished; !errors.Is(err, ErrSessionChanged) {
		t.Fatalf("pending installation error=%v, want session changed", err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth != (config.AuthConfig{}) {
		t.Fatalf("pending installation restored authentication metadata: %+v", loaded.Auth)
	}
	if _, err := store.Load(ctx, cfg.Auth.SessionKey); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("pending installation restored logged-out credentials: %v", err)
	}
	if _, err := NewManager(loaded, ManagerOptions{Store: store}).Snapshot(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("pending installation restored the logged-out API session: %v", err)
	}
}
