package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

type upNextRetryPolicy struct {
	attempts  int
	baseDelay time.Duration
}

// One probe holds both the request outcome and the credential metadata from the
// manager that performed it. Queue parsing does not determine authentication.
type upNextProbeResult struct {
	snapshot pocketcasts.UpNextSnapshot
	auth     NowAuthStatus
	err      error
}

func probeUpNext(ctx context.Context, cfg config.Config, opts authn.ManagerOptions, policy upNextRetryPolicy) upNextProbeResult {
	result := upNextProbeResult{auth: NowAuthStatus{Status: "missing"}}
	if err := ctx.Err(); err != nil {
		result.err = classifyUpNextError(err)
		return result
	}
	client, manager := authn.NewPocketCastsClient(cfg, opts)
	session, source, loadErr := manager.Snapshot(ctx)
	if loadErr != nil {
		result.auth = authStatusFromSession(session, source, loadErr)
		if ctx.Err() != nil {
			loadErr = ctx.Err()
		}
		result.err = classifyUpNextError(loadErr)
		return result
	}

	snapshot, err := fetchUpNextWithRetry(ctx, client, policy)
	result.snapshot = snapshot
	result.err = classifyUpNextError(err)
	// AccessToken or a 401 replay may have rotated the session. Snapshot reads
	// the same manager's cached state, without another credential-store lookup.
	session, source, loadErr = manager.Snapshot(ctx)
	result.auth = authStatusFromSession(session, source, loadErr)
	return result
}

func classifyUpNextError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, authn.ErrNotConfigured) || upNextHTTPStatus(err) == http.StatusUnauthorized {
		return Wrap(KindUnauthorized, "", err)
	}
	return Wrap(KindTransient, "", err)
}

func (result upNextProbeResult) verificationError() error {
	return Wrap(KindOf(result.err), "auth verify", result.err)
}

func authStatusFromSession(session authn.Session, source authn.Source, err error) NowAuthStatus {
	auth := NowAuthStatus{Status: "missing"}
	if err != nil || session.AccessToken == "" {
		if err != nil && !errors.Is(err, authn.ErrNotConfigured) {
			auth.Error = err.Error()
		}
		return auth
	}
	auth.Status = "configured"
	auth.AuthorizationExists = true
	auth.Source = string(source)
	auth.Method = session.Method
	if session.ExpiresAt > 0 {
		auth.TokenExpiryKnown = true
		auth.TokenExpiryUnix = session.ExpiresAt
	}
	return auth
}

func (result upNextProbeResult) authStatus(verify bool) NowAuthStatus {
	auth := result.auth
	if !verify || !auth.AuthorizationExists {
		return auth
	}
	switch KindOf(result.err) {
	case "":
		auth.Status = "verified"
	case KindUnauthorized:
		auth.Status = "unauthorized"
	default:
		auth.Status = "unverified"
	}
	if result.err != nil {
		auth.Error = result.verificationError().Error()
	}
	return auth
}

func (result upNextProbeResult) queueStatus() NowQueueStatus {
	if result.err != nil {
		if errors.Is(result.err, authn.ErrNotConfigured) {
			return NowQueueStatus{Status: "unauthorized", Error: "API authentication is not configured"}
		}
		if KindOf(result.err) == KindUnauthorized {
			return NowQueueStatus{Status: "unauthorized", Error: "API returned 401 Unauthorized"}
		}
		return NowQueueStatus{Status: "unavailable", Error: result.err.Error()}
	}
	if result.snapshot.ParseError != nil {
		return NowQueueStatus{Status: "unavailable", Error: "failed to parse queue"}
	}
	if len(result.snapshot.Episodes) == 0 {
		return NowQueueStatus{Status: "empty"}
	}
	inProgress := 0
	for _, progress := range result.snapshot.Progress {
		if progress > 0 {
			inProgress++
		}
	}
	return NowQueueStatus{
		Status:          "ready",
		Total:           len(result.snapshot.Episodes),
		NextTitle:       strings.TrimSpace(result.snapshot.Episodes[0].Title),
		InProgressCount: inProgress,
	}
}

func fetchUpNextWithRetry(ctx context.Context, client *pocketcasts.Client, policy upNextRetryPolicy) (pocketcasts.UpNextSnapshot, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return pocketcasts.UpNextSnapshot{}, err
		}
		snapshot, err := client.UpNextList(ctx, "0")
		if ctx.Err() != nil {
			return pocketcasts.UpNextSnapshot{}, ctx.Err()
		}
		if err == nil || attempt >= policy.attempts || !isRetryableUpNextError(err) {
			return snapshot, err
		}
		timer := time.NewTimer(policy.baseDelay * time.Duration(1<<(attempt-1)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return pocketcasts.UpNextSnapshot{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func upNextHTTPStatus(err error) int {
	var statusErr interface{ HTTPStatusCode() int }
	if errors.As(err, &statusErr) {
		return statusErr.HTTPStatusCode()
	}
	return 0
}

func isRetryableUpNextError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, authn.ErrNotConfigured) || errors.Is(err, authn.ErrCredentialUnavailable) {
		return false
	}
	if status := upNextHTTPStatus(err); status != 0 {
		return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 && status <= 599
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && (networkErr.Timeout() || networkErr.Temporary()) {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE)
}
