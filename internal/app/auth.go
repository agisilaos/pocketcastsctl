package app

import (
	"context"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

// VerifyAuth verifies the active API session without requiring a readable queue.
func VerifyAuth(ctx context.Context, cfg config.Config) error {
	return probeUpNext(ctx, cfg, authn.ManagerOptions{}, upNextRetryPolicy{
		attempts:  3,
		baseDelay: 200 * time.Millisecond,
	}).verificationError()
}
