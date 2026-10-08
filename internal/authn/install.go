package authn

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"pocketcastsctl/internal/config"
)

// Install validates a candidate, then serializes credential and metadata writes
// after rechecking the saved session key. A different previous Keychain item is
// retained until the new item and config pointer are saved. Reusing the same key
// replaces its credentials first, so metadata failure can return partial success
// without restoring the previous credentials.
func Install(ctx context.Context, cfg config.Config, store Store, api *API, candidate Session) (config.Config, error) {
	if store == nil {
		store = NewKeyringStore()
	}
	candidate = candidate.normalized()
	if candidate.AccessToken == "" {
		return cfg, errors.New("candidate API session has no access token")
	}
	if api == nil {
		return cfg, errors.New("authentication API is unavailable")
	}
	if err := config.ValidateAuthUpdate(api.BaseURL); err != nil {
		return cfg, err
	}
	validated, err := api.Validate(ctx, candidate)
	if err != nil {
		return cfg, fmt.Errorf("validate candidate API session: %w", err)
	}
	candidate = validated

	unlock, err := acquirePersistenceLock(ctx)
	if err != nil {
		return cfg, err
	}
	defer unlock()
	current, err := config.Load()
	if err != nil {
		return cfg, err
	}
	if strings.TrimSpace(current.Auth.SessionKey) != strings.TrimSpace(cfg.Auth.SessionKey) {
		return current, ErrSessionChanged
	}
	cfg = current
	if err := config.ValidateAuthUpdate(api.BaseURL); err != nil {
		return cfg, err
	}

	key := sessionKey(api.BaseURL, candidate)
	previousKey := strings.TrimSpace(cfg.Auth.SessionKey)
	pending := cfg
	pending.Auth = metadataFor(key, candidate)
	pending.APIHeaders = withoutAuthorization(cfg.APIHeaders)
	if err := store.Save(ctx, key, candidate.credentials()); err != nil {
		if previousKey != key {
			_ = store.Delete(ctx, key)
		}
		return cfg, err
	}

	updated, err := config.UpdateAuth(api.BaseURL, metadataFor(key, candidate))
	if err != nil {
		if errors.Is(err, config.ErrDurabilityUncertain) {
			return updated, fmt.Errorf("API session installed, but config durability could not be confirmed: %w", err)
		}
		if previousKey != key {
			_ = store.Delete(ctx, key)
		} else {
			return pending, fmt.Errorf("API session installed, but active session metadata could not be updated: %w", err)
		}
		return cfg, fmt.Errorf("save active API session metadata: %w", err)
	}
	if previousKey != "" && previousKey != key {
		if err := store.Delete(ctx, previousKey); err != nil {
			return updated, fmt.Errorf("API session installed, but old Keychain cleanup failed: %w", err)
		}
	}
	return updated, nil
}

func Logout(ctx context.Context, cfg config.Config, store Store) (config.Config, error) {
	if store == nil {
		store = NewKeyringStore()
	}
	unlock, err := acquirePersistenceLock(ctx)
	if err != nil {
		return cfg, err
	}
	defer unlock()
	current, err := config.Load()
	if err != nil {
		return cfg, err
	}
	cfg = current
	updated, err := config.ClearAuth()
	if err != nil {
		if errors.Is(err, config.ErrDurabilityUncertain) {
			return updated, fmt.Errorf("logged out, but config durability could not be confirmed: %w", err)
		}
		return cfg, fmt.Errorf("save logged-out config: %w", err)
	}
	if err := store.Delete(ctx, cfg.Auth.SessionKey); err != nil {
		return updated, fmt.Errorf("logged out, but Keychain cleanup failed: %w", err)
	}
	return updated, nil
}
