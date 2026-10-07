package authn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"pocketcastsctl/internal/config"
)

func TestSnapshotResolvesOnlyNonSecretMetadata(t *testing.T) {
	token := "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"token-account","email":"Token@Example.com","exp":100}`)) + ".y"
	for _, test := range []struct {
		name        string
		environment string
		saved       bool
		legacy      bool
		want        Source
		method      string
	}{
		{"environment", token, true, true, SourceEnvironment, "environment"},
		{"saved", "", true, true, SourceKeychain, "password"},
		{"legacy", "", false, true, SourceLegacy, "legacy-config"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(config.EnvAccessToken, test.environment)
			cfg := config.Default()
			if test.saved {
				cfg.Auth = config.AuthConfig{SessionKey: "active", AccountID: "stale-account", Email: "stale@example.com", Method: "password", Scope: ScopeWebPlayer, ExpiresAt: 200}
			}
			if test.legacy {
				cfg.APIHeaders["Authorization"] = "Bearer " + token
			}
			store := newMemoryStore()
			store.credentials["active"] = Credentials{AccessToken: token, RefreshToken: "refresh-secret"}
			manager := NewManager(cfg, ManagerOptions{Store: store, Now: func() time.Time { return time.Unix(1000, 0) }, HTTP: &http.Client{Transport: snapshotNoNetwork{t}}})
			snapshot, err := manager.Snapshot(context.Background())
			if err != nil || snapshot.Source != test.want || snapshot.AccountID != "token-account" || snapshot.Email != "token@example.com" || snapshot.ExpiresAt != 100 || snapshot.Method != test.method {
				t.Fatalf("snapshot=%+v error=%v", snapshot, err)
			}
			wantScope := ""
			if test.want == SourceKeychain {
				wantScope = ScopeWebPlayer
			}
			if snapshot.Scope != wantScope {
				t.Fatalf("scope=%q, want %q", snapshot.Scope, wantScope)
			}
			second, err := manager.Snapshot(context.Background())
			if err != nil || second != snapshot || store.saves != 0 {
				t.Fatalf("local snapshot changed state: %+v, %v", second, err)
			}
			raw, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			for _, output := range []string{string(raw), fmt.Sprintf("%+v", snapshot)} {
				for _, secret := range []string{token, "refresh-secret", "AccessToken", "RefreshToken"} {
					if strings.Contains(output, secret) {
						t.Fatalf("snapshot exposed secret material: %s", output)
					}
				}
			}
		})
	}
}

type snapshotNoNetwork struct{ t *testing.T }

func (transport snapshotNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	transport.t.Error("Snapshot performed network I/O")
	return nil, errors.New("unexpected network operation")
}

// Return raw credentials to exercise Manager's invariant even when a Store
// violates its contract by successfully loading an empty access token.
type snapshotCredentialStore struct {
	Store
	credentials Credentials
	loadErr     error
	loads       int
}

func (store *snapshotCredentialStore) Load(context.Context, string) (Credentials, error) {
	store.loads++
	return store.credentials, store.loadErr
}

func TestSnapshotFailsClosedWithoutCredentials(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	for _, test := range []struct {
		name        string
		saved       bool
		credentials Credentials
		loadErr     error
		want        error
	}{
		{name: "not configured", want: ErrNotConfigured},
		{name: "inaccessible", saved: true, loadErr: errors.New("Keychain locked"), want: ErrCredentialUnavailable},
		{name: "empty successful load", saved: true, want: ErrCredentialUnavailable},
		{name: "refresh only", saved: true, credentials: Credentials{RefreshToken: "refresh-secret"}, want: ErrCredentialUnavailable},
		{name: "whitespace access", saved: true, credentials: Credentials{AccessToken: "  \t "}, want: ErrCredentialUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.Default()
			store := &snapshotCredentialStore{Store: newMemoryStore(), credentials: test.credentials, loadErr: test.loadErr}
			if test.saved {
				cfg.Auth = config.AuthConfig{SessionKey: "active", Email: "saved@example.com"}
				cfg.APIHeaders["Authorization"] = "Bearer dormant-legacy-secret"
			}
			manager := NewManager(cfg, ManagerOptions{Store: store})
			for attempt := 0; attempt < 2; attempt++ {
				snapshot, err := manager.Snapshot(context.Background())
				if !errors.Is(err, test.want) || snapshot != (ResolvedSession{Source: SourceNone}) {
					t.Fatalf("snapshot=%+v error=%v", snapshot, err)
				}
				token, err := manager.AccessToken(context.Background())
				if token != "" || !errors.Is(err, test.want) {
					t.Fatalf("token source returned credentials after snapshot failure: %v", err)
				}
			}
			wantLoads := 0
			if test.saved {
				wantLoads = 1
			}
			if store.loads != wantLoads {
				t.Fatalf("credential loads=%d, want %d", store.loads, wantLoads)
			}
		})
	}
}
