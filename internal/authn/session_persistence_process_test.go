package authn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pocketcastsctl/internal/config"
)

// The file-backed fixture shares synthetic credentials with a subprocess and
// can pause after writing credentials, before the auth metadata commit.
type processCredentialStore struct {
	dir       string
	pauseSave bool
}

func (s processCredentialStore) Load(_ context.Context, key string) (Credentials, error) {
	raw, err := os.ReadFile(filepath.Join(s.dir, key+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, ErrSessionNotFound
	}
	if err != nil {
		return Credentials{}, err
	}
	var credentials Credentials
	err = json.Unmarshal(raw, &credentials)
	return credentials, err
}

func (s processCredentialStore) Save(ctx context.Context, key string, credentials Credentials) error {
	raw, err := json.Marshal(credentials)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.dir, key+".json"), raw, 0o600); err != nil {
		return err
	}
	if !s.pauseSave {
		return nil
	}
	if err := os.WriteFile(filepath.Join(s.dir, "credentials-saved"), nil, 0o600); err != nil {
		return err
	}
	return awaitFixtureFile(ctx, filepath.Join(s.dir, "resume"))
}

func (s processCredentialStore) Delete(_ context.Context, key string) error {
	err := os.Remove(filepath.Join(s.dir, key+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func awaitFixtureFile(ctx context.Context, path string) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func TestLogoutCannotSplitRefreshPersistenceAcrossProcesses(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv(config.EnvAPIBaseURL, "")
	t.Setenv(config.EnvConfigPath, filepath.Join(t.TempDir(), "config.json"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"accessToken":"rotated-access","refreshToken":"rotated-refresh","expiresIn":3600}`)
	}))
	defer server.Close()
	store := processCredentialStore{dir: t.TempDir()}
	cfg := config.Default()
	cfg.APIBaseURL = server.URL
	cfg.Auth = config.AuthConfig{SessionKey: strings.Repeat("a", 64), AccountID: "synthetic-account", Scope: ScopeWebPlayer}
	writeSavedAuthConfig(t, config.Path(), server.URL, cfg.Auth, nil)
	if err := store.Save(ctx, cfg.Auth.SessionKey, Credentials{AccessToken: "original-access", RefreshToken: "original-refresh"}); err != nil {
		t.Fatal(err)
	}
	worker := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRefreshPersistenceWorker$")
	worker.Env = append(os.Environ(), "POCKETCASTS_TEST_AUTH_STORE="+store.dir)
	var output bytes.Buffer
	worker.Stdout, worker.Stderr = &output, &output
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = worker.Process.Kill() }()
	resume := func() { _ = os.WriteFile(filepath.Join(store.dir, "resume"), nil, 0o600) }
	defer resume()
	if err := awaitFixtureFile(ctx, filepath.Join(store.dir, "credentials-saved")); err != nil {
		t.Fatal(err)
	}

	logoutCtx, logoutCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, logoutErr := Logout(logoutCtx, cfg, store)
	logoutCancel()
	resume()
	if err := worker.Wait(); err != nil {
		t.Fatalf("refresh worker: %v\n%s", err, &output)
	}
	if !errors.Is(logoutErr, context.DeadlineExceeded) {
		t.Fatalf("logout entered another process's credential/metadata commit: error=%v", logoutErr)
	}
	// Cancellation must release its waiting resources. After the refresh exits,
	// a later logout can complete and remains effective for a new manager.
	if _, err := Logout(ctx, cfg, store); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(loaded, ManagerOptions{Store: store}).Snapshot(ctx); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("logout did not disable the saved session: %v", err)
	}
}

func TestRefreshPersistenceWorker(t *testing.T) {
	dir := os.Getenv("POCKETCASTS_TEST_AUTH_STORE")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(cfg, ManagerOptions{Store: processCredentialStore{dir: dir, pauseSave: true}})
	if _, err := manager.ForceRefresh(ctx); err != nil {
		t.Fatal(err)
	}
}
