package authn

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"pocketcastsctl/internal/config"
)

// Install validates a candidate before making it active. The old session is
// kept addressable until both the new Keychain item and config pointer exist.
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
	validated, err := api.Validate(ctx, candidate)
	if err != nil {
		return cfg, fmt.Errorf("validate candidate API session: %w", err)
	}
	candidate = validated

	key := sessionKey(api.BaseURL, candidate)
	previousKey := strings.TrimSpace(cfg.Auth.SessionKey)
	if err := store.Save(ctx, key, candidate); err != nil {
		if previousKey != key {
			_ = store.Delete(ctx, key)
		}
		return cfg, err
	}

	updated := cfg
	updated.Auth = metadataFor(key, candidate)
	updated.APIHeaders = withoutAuthorization(cfg.APIHeaders)
	if err := config.Save(updated); err != nil {
		if previousKey != key {
			_ = store.Delete(ctx, key)
		} else {
			return updated, fmt.Errorf("API session installed, but active session metadata could not be updated: %w", err)
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
	updated := cfg
	updated.Auth = config.AuthConfig{}
	updated.APIHeaders = withoutAuthorization(cfg.APIHeaders)
	if err := config.Save(updated); err != nil {
		return cfg, fmt.Errorf("save logged-out config: %w", err)
	}
	if err := store.Delete(ctx, cfg.Auth.SessionKey); err != nil {
		return updated, fmt.Errorf("logged out, but Keychain cleanup failed: %w", err)
	}
	return updated, nil
}
