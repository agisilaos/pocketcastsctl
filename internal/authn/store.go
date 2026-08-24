package authn

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const (
	keychainAccessService  = "pocketcastsctl.api-session.v1.access"
	keychainRefreshService = "pocketcastsctl.api-session.v1.refresh"
	keychainValuePrefix    = "pocketcastsctl-base64-v1:"
)

var ErrSessionNotFound = errors.New("API session not found in credential store")

type Store interface {
	Load(context.Context, string) (Session, error)
	Save(context.Context, string, Session) error
	Delete(context.Context, string) error
}

// KeyringStore keeps access and refresh tokens in separate macOS Keychain
// items. Calls use CommandContext so command deadlines also bound Keychain
// prompts; secret values are supplied through stdin rather than argv.
type KeyringStore struct{}

func NewKeyringStore() Store {
	return KeyringStore{}
}

func (KeyringStore) Load(ctx context.Context, key string) (Session, error) {
	if err := validateCredentialKey(key); err != nil {
		return Session{}, err
	}
	accessToken, err := keychainRead(ctx, keychainAccessService, key)
	if err != nil {
		return Session{}, fmt.Errorf("read access token from Keychain: %w", err)
	}
	refreshToken, err := keychainRead(ctx, keychainRefreshService, key)
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		return Session{}, fmt.Errorf("read refresh token from Keychain: %w", err)
	}
	session := Session{AccessToken: accessToken, RefreshToken: refreshToken}.normalized()
	if session.AccessToken == "" {
		return Session{}, errors.New("API session in Keychain has no access token")
	}
	return session, nil
}

func (KeyringStore) Save(ctx context.Context, key string, session Session) error {
	if err := validateCredentialKey(key); err != nil {
		return err
	}
	session = session.normalized()
	if session.AccessToken == "" {
		return errors.New("cannot store an API session without an access token")
	}

	// Refresh tokens rotate. Persist the replacement before the access token so
	// an interrupted write never leaves a new access token with an invalidated
	// old refresh token.
	if session.RefreshToken == "" {
		if err := keychainDelete(ctx, keychainRefreshService, key); err != nil {
			return fmt.Errorf("clear refresh token from Keychain: %w", err)
		}
	} else if err := keychainWrite(ctx, keychainRefreshService, key, session.RefreshToken); err != nil {
		return fmt.Errorf("save refresh token to Keychain: %w", err)
	}
	if err := keychainWrite(ctx, keychainAccessService, key, session.AccessToken); err != nil {
		return fmt.Errorf("save access token to Keychain: %w", err)
	}
	return nil
}

func (KeyringStore) Delete(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if err := validateCredentialKey(key); err != nil {
		return err
	}
	var cleanupErrors []error
	for _, service := range []string{keychainAccessService, keychainRefreshService} {
		if err := keychainDelete(ctx, service, key); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if err := errors.Join(cleanupErrors...); err != nil {
		return fmt.Errorf("delete API session from Keychain: %w", err)
	}
	return nil
}

func validateCredentialKey(key string) error {
	key = strings.TrimSpace(key)
	if len(key) != 64 {
		return errors.New("credential-store key is invalid")
	}
	for _, char := range key {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return errors.New("credential-store key is invalid")
		}
	}
	return nil
}

func keychainRead(ctx context.Context, service, key string) (string, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", service, "-a", key, "-w").CombinedOutput()
	if err != nil {
		if keychainItemMissing(output) {
			return "", ErrSessionNotFound
		}
		return "", keychainCommandError(ctx, err)
	}
	encoded := strings.TrimSpace(string(output))
	if !strings.HasPrefix(encoded, keychainValuePrefix) {
		return "", errors.New("Keychain item has an unsupported format")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, keychainValuePrefix))
	if err != nil {
		return "", errors.New("Keychain item is corrupted")
	}
	return string(raw), nil
}

func keychainWrite(ctx context.Context, service, key, value string) error {
	encoded := keychainValuePrefix + base64.StdEncoding.EncodeToString([]byte(value))
	command := fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s\n", service, key, encoded)
	if len(command) > 4096 {
		return errors.New("Keychain item is too large")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(command)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = output // Never include helper output; it may contain sensitive context.
		return keychainCommandError(ctx, err)
	}
	return nil
}

func keychainDelete(ctx context.Context, service, key string) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/security", "delete-generic-password", "-s", service, "-a", key).CombinedOutput()
	if err != nil && !keychainItemMissing(output) {
		return keychainCommandError(ctx, err)
	}
	return nil
}

func keychainItemMissing(output []byte) bool {
	message := strings.ToLower(string(output))
	return strings.Contains(message, "could not be found") || strings.Contains(message, "item not found")
}

func keychainCommandError(ctx context.Context, commandErr error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("macOS Keychain command failed: %w", commandErr)
}
