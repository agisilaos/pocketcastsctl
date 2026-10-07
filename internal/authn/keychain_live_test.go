package authn

import (
	"context"
	"errors"
	"os"
	"strings"
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

	testCredentialsRoundTrip(t, ctx, store, key)

	want := Credentials{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh"}
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
	// Oversized values fail before invoking security. A refresh-write failure
	// preserves both old items; an access-write failure retains the rotated
	// refresh token alongside the old access token for the next process.
	if err := store.Save(ctx, key, Credentials{AccessToken: "new-access", RefreshToken: strings.Repeat("x", 4096)}); err == nil {
		t.Fatal("oversized refresh token was saved")
	}
	if got, err := store.Load(ctx, key); err != nil || got != want {
		t.Fatal("refresh-write failure changed credentials")
	}
	if err := store.Save(ctx, key, Credentials{AccessToken: strings.Repeat("x", 4096), RefreshToken: "rotated-refresh"}); err == nil {
		t.Fatal("oversized access token was saved")
	}
	if got, err := store.Load(ctx, key); err != nil || got.AccessToken != want.AccessToken || got.RefreshToken != "rotated-refresh" {
		t.Fatal("access-write failure lost rotated refresh token")
	}
	// An orphan refresh item cannot resolve a saved API session.
	if err := keychainDelete(ctx, keychainAccessService, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, key); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("orphan refresh: %v", err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, key); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("load after delete error = %v, want ErrSessionNotFound", err)
	}
}
