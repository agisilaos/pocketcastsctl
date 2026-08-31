package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

type VerifyOptions struct {
	Attempts  int
	BaseDelay time.Duration
}

func VerifyAuth(ctx context.Context, cfg config.Config, opts VerifyOptions) error {
	op := "auth verify"
	client, _ := authn.NewPocketCastsClient(cfg, authn.ManagerOptions{})
	_, err := fetchUpNextWithRetry(ctx, client, opts)
	if err != nil {
		if authutil.IsUnauthorizedError(err) || isMissingAuthError(err) {
			return Wrap(KindUnauthorized, op, err)
		}
		return Wrap(KindTransient, op, err)
	}
	return nil
}

func isMissingAuthError(err error) bool {
	return errors.Is(err, authn.ErrNotConfigured)
}

func fetchUpNextWithRetry(ctx context.Context, client *pocketcasts.Client, opts VerifyOptions) (pocketcasts.UpNextSnapshot, error) {
	attempts := opts.Attempts
	if attempts < 1 {
		attempts = 3
	}
	baseDelay := opts.BaseDelay
	if baseDelay <= 0 {
		baseDelay = 200 * time.Millisecond
	}

	var snapshot pocketcasts.UpNextSnapshot
	var lastErr error
	for i := 1; i <= attempts; i++ {
		if err := ctx.Err(); err != nil {
			return pocketcasts.UpNextSnapshot{}, err
		}
		snapshot, lastErr = client.UpNextList(ctx, "0")
		if lastErr == nil {
			return snapshot, nil
		}
		if i == attempts || !isRetryableTransientError(lastErr) {
			break
		}
		timer := time.NewTimer(baseDelay * time.Duration(1<<(i-1)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return pocketcasts.UpNextSnapshot{}, ctx.Err()
		case <-timer.C:
		}
	}
	return pocketcasts.UpNextSnapshot{}, lastErr
}

func isRetryableTransientError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	retry := []string{
		"timeout",
		"tempor",
		"connection reset",
		"connection refused",
		"broken pipe",
		"eof",
	}
	for _, token := range retry {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}
