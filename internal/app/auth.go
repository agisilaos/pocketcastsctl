package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/pocketcasts"
)

type VerifyOptions struct {
	Attempts  int
	BaseDelay time.Duration
}

type CandidateFailure struct {
	SourceKey string `json:"source_key"`
	Reason    string `json:"reason"`
}

type SyncVerifyOptions struct {
	Browser         string
	BrowserApp      string
	URLContains     string
	KeyContains     string
	CandidatePasses int
	VerifyOptions   VerifyOptions
}

type SyncVerifyResult struct {
	SourceKey string             `json:"source_key"`
	Failures  []CandidateFailure `json:"failures,omitempty"`
}

func VerifyAuth(ctx context.Context, cfg config.Config, opts VerifyOptions) error {
	op := "auth verify"
	if !authutil.HasAuthorizationHeader(cfg.APIHeaders) {
		return Wrap(KindUnauthorized, op, fmt.Errorf("Authorization header missing"))
	}

	client := pocketcasts.New(pocketcasts.Options{BaseURL: cfg.APIBaseURL, Headers: cfg.APIHeaders})
	_, err := fetchUpNextWithRetry(ctx, client, opts)
	if err != nil {
		if authutil.IsUnauthorizedError(err) {
			return Wrap(KindUnauthorized, op, err)
		}
		return Wrap(KindTransient, op, err)
	}
	return nil
}

func SyncAndVerifyAuth(ctx context.Context, cfg config.Config, opts SyncVerifyOptions) (config.Config, SyncVerifyResult, error) {
	op := "auth refresh"
	if strings.TrimSpace(opts.Browser) == "" {
		opts.Browser = cfg.Browser
	}
	if strings.TrimSpace(opts.BrowserApp) == "" {
		opts.BrowserApp = cfg.BrowserApp
	}
	if strings.TrimSpace(opts.URLContains) == "" {
		opts.URLContains = cfg.URLContains
	}
	if opts.CandidatePasses < 1 {
		opts.CandidatePasses = 1
	}

	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     opts.Browser,
		BrowserApp:  opts.BrowserApp,
		URLContains: opts.URLContains,
	})
	if err != nil {
		return cfg, SyncVerifyResult{}, Wrap(KindUsage, op, fmt.Errorf("invalid browser options: %w", err))
	}

	updated := cfg
	if updated.APIHeaders == nil {
		updated.APIHeaders = map[string]string{}
	}
	updated.Browser = opts.Browser
	updated.BrowserApp = strings.TrimSpace(opts.BrowserApp)
	updated.URLContains = opts.URLContains

	var failures []CandidateFailure
	for pass := 1; pass <= opts.CandidatePasses; pass++ {
		cands, err := controller.TokenCandidates(ctx)
		if err != nil {
			return cfg, SyncVerifyResult{Failures: failures}, Wrap(KindTransient, op, fmt.Errorf("token discovery failed: %w", err))
		}
		ranked := rankedTokenCandidates(cands, opts.KeyContains)
		if len(ranked) == 0 {
			return cfg, SyncVerifyResult{Failures: failures}, Wrap(KindUnauthorized, op, fmt.Errorf("no token candidates found"))
		}

		for _, c := range ranked {
			token := normalizeToken(c.Token)
			if token == "" {
				continue
			}
			candidateCfg := updated
			headersCopy := make(map[string]string, len(updated.APIHeaders))
			for k, v := range updated.APIHeaders {
				headersCopy[k] = v
			}
			candidateCfg.APIHeaders = headersCopy
			candidateCfg.APIHeaders["Authorization"] = "Bearer " + token

			if verifyErr := VerifyAuth(ctx, candidateCfg, opts.VerifyOptions); verifyErr == nil {
				return candidateCfg, SyncVerifyResult{SourceKey: c.SourceKey, Failures: failures}, nil
			} else {
				reason := "verification failed"
				switch KindOf(verifyErr) {
				case KindUnauthorized:
					reason = "401 unauthorized"
				case KindTransient:
					reason = "verification unavailable"
				}
				failures = append(failures, CandidateFailure{SourceKey: c.SourceKey, Reason: reason})
			}
		}
	}

	if len(failures) == 0 {
		return cfg, SyncVerifyResult{}, Wrap(KindUnauthorized, op, fmt.Errorf("all token candidates were empty"))
	}
	allUnauthorized := true
	for _, f := range failures {
		if f.Reason != "401 unauthorized" {
			allUnauthorized = false
			break
		}
	}
	if allUnauthorized {
		return cfg, SyncVerifyResult{Failures: failures}, Wrap(KindUnauthorized, op, fmt.Errorf("all token candidates were rejected by API"))
	}
	return cfg, SyncVerifyResult{Failures: failures}, Wrap(KindTransient, op, fmt.Errorf("could not verify token due to transient/API errors"))
}

func normalizeToken(token string) string {
	return authutil.NormalizeToken(token)
}

func fetchUpNextWithRetry(ctx context.Context, client *pocketcasts.Client, opts VerifyOptions) ([]byte, error) {
	attempts := opts.Attempts
	if attempts < 1 {
		attempts = 3
	}
	baseDelay := opts.BaseDelay
	if baseDelay <= 0 {
		baseDelay = 200 * time.Millisecond
	}

	var body []byte
	var lastErr error
	for i := 1; i <= attempts; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, lastErr = client.UpNextList(ctx, pocketcasts.UpNextListRequest{
			Model:          "webplayer",
			ServerModified: "0",
			ShowPlayStatus: true,
			Version:        2,
		})
		if lastErr == nil {
			return body, nil
		}
		if i == attempts || !isRetryableTransientError(lastErr) {
			break
		}
		timer := time.NewTimer(baseDelay * time.Duration(1<<(i-1)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func rankedTokenCandidates(cands []browsercontrol.TokenCandidate, keyContains string) []browsercontrol.TokenCandidate {
	out := make([]browsercontrol.TokenCandidate, 0, len(cands))
	for _, c := range cands {
		if strings.TrimSpace(c.Token) == "" {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return tokenCandidateScore(out[i], keyContains) > tokenCandidateScore(out[j], keyContains)
	})
	return out
}

func tokenCandidateScore(c browsercontrol.TokenCandidate, keyContains string) int {
	keyContains = strings.ToLower(strings.TrimSpace(keyContains))
	score := 0
	k := strings.ToLower(c.SourceKey)
	if keyContains != "" {
		if strings.Contains(k, keyContains) {
			score += 1000
		} else {
			score -= 1000
		}
	}
	if strings.Contains(k, "access") {
		score += 30
	}
	if strings.Contains(k, "auth") {
		score += 20
	}
	if strings.Contains(k, "token") {
		score += 10
	}
	if strings.Contains(k, "session") {
		score += 5
	}
	if len(strings.TrimSpace(c.Token)) >= 40 {
		score += 5
	}
	return score
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
