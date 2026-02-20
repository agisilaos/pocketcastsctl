package main

import (
	"context"
	"pocketcastsctl/internal/pocketcasts"
	"time"
)

func fetchUpNextWithRetry(ctx context.Context, client *pocketcasts.Client, serverModified string) ([]byte, error) {
	var body []byte
	err := retryTransient(ctx, 3, 200*time.Millisecond, func() error {
		var fetchErr error
		body, fetchErr = client.UpNextList(ctx, pocketcasts.UpNextListRequest{
			Model:          "webplayer",
			ServerModified: serverModified,
			ShowPlayStatus: true,
			Version:        2,
		})
		return fetchErr
	})
	if err != nil {
		return nil, err
	}
	return body, nil
}
