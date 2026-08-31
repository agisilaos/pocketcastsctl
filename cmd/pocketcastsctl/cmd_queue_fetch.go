package main

import (
	"context"
	"pocketcastsctl/internal/pocketcasts"
	"time"
)

func fetchUpNextWithRetry(ctx context.Context, client *pocketcasts.Client, serverModified string) (pocketcasts.UpNextSnapshot, error) {
	var snapshot pocketcasts.UpNextSnapshot
	err := retryTransient(ctx, 3, 200*time.Millisecond, func() error {
		var fetchErr error
		snapshot, fetchErr = client.UpNextList(ctx, serverModified)
		return fetchErr
	})
	if err != nil {
		return pocketcasts.UpNextSnapshot{}, err
	}
	return snapshot, nil
}
