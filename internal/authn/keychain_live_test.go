package authn

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// This opt-in test writes only synthetic values and removes both Keychain
// items before returning. It validates the real macOS `security` integration.
func TestLiveKeychainStoreRoundTrip(t *testing.T) {
	if os.Getenv("POCKETCASTS_KEYCHAIN_LIVE") != "1" {
		t.Skip("set POCKETCASTS_KEYCHAIN_LIVE=1 to test macOS Keychain")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store := KeyringStore{}
	key := sessionKey("https://keychain-live.invalid", Session{AccountID: time.Now().UTC().Format(time.RFC3339Nano), Scope: "test"})
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = store.Delete(cleanupCtx, key)
	})

	want := Session{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh"}
	if err := store.Save(ctx, key, want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken {
		t.Fatal("Keychain round trip changed synthetic token values")
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, key); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("load after delete error = %v, want ErrSessionNotFound", err)
	}
}
