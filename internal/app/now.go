package app

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/localplayback"
)

type NowOptions struct {
	VerifyAuth bool
}

type nowCollectorFuncs struct {
	web   func(context.Context) NowWebPlaybackSnapshot
	local func(context.Context) (NowLocalStatus, []string)
	api   func(context.Context) (NowAuthStatus, NowQueueStatus)
}

type NowSnapshot struct {
	GeneratedAt time.Time              `json:"generated_at"`
	Web         NowWebPlaybackSnapshot `json:"web"`
	Local       NowLocalStatus         `json:"local"`
	Queue       NowQueueStatus         `json:"queue"`
	Auth        NowAuthStatus          `json:"auth"`
	Actions     []string               `json:"actions"`
	Warnings    []string               `json:"warnings,omitempty"`
	Meta        map[string]string      `json:"meta,omitempty"`
}

type NowWebPlaybackSnapshot struct {
	State browsercontrol.PlaybackState `json:"status"`
	browsercontrol.PlaybackDetails
	Error string `json:"error,omitempty"`
}

type NowLocalStatus struct {
	Status string `json:"status"` // playing|paused|stopped|error
	Title  string `json:"title,omitempty"`
	Error  string `json:"error,omitempty"`
}

type NowQueueStatus struct {
	Status          string `json:"status"` // ready|unauthorized|empty|unavailable
	Total           int    `json:"total"`
	NextTitle       string `json:"next_title,omitempty"`
	InProgressCount int    `json:"in_progress_count"`
	Error           string `json:"error,omitempty"`
}

type NowAuthStatus struct {
	Status              string `json:"status"` // configured|missing|verified|unauthorized|unverified
	AuthorizationExists bool   `json:"authorization_present"`
	Source              string `json:"source,omitempty"`
	Method              string `json:"method,omitempty"`
	TokenExpiryKnown    bool   `json:"token_expiry_known"`
	TokenExpiryUnix     int64  `json:"token_expiry_unix,omitempty"`
	Error               string `json:"error,omitempty"`
}

func CollectNowSnapshot(ctx context.Context, cfg config.Config, opts NowOptions) NowSnapshot {
	return collectNowSnapshot(ctx, config.Path(), nowCollectorFuncs{
		web: func(ctx context.Context) NowWebPlaybackSnapshot {
			return collectWebPlaybackSnapshot(ctx, cfg)
		},
		local: func(ctx context.Context) (NowLocalStatus, []string) {
			return collectLocalStatus(ctx)
		},
		api: func(ctx context.Context) (NowAuthStatus, NowQueueStatus) {
			return collectNowAPIStatus(ctx, cfg, opts, authn.ManagerOptions{})
		},
	})
}

func collectNowSnapshot(ctx context.Context, configPath string, collectors nowCollectorFuncs) NowSnapshot {
	generatedAt := time.Now()
	var web NowWebPlaybackSnapshot
	var local NowLocalStatus
	var localWarnings []string
	var auth NowAuthStatus
	var queue NowQueueStatus
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		web = collectors.web(ctx)
	}()
	go func() {
		defer wg.Done()
		local, localWarnings = collectors.local(ctx)
	}()
	go func() {
		defer wg.Done()
		auth, queue = collectors.api(ctx)
	}()
	wg.Wait()

	s := NowSnapshot{
		GeneratedAt: generatedAt,
		Web:         web,
		Local:       local,
		Queue:       queue,
		Auth:        auth,
		Warnings:    append([]string(nil), localWarnings...),
		Meta: map[string]string{
			"config_path": configPath,
		},
	}
	s.Actions = suggestNowActions(s)
	return s
}

func collectWebPlaybackSnapshot(ctx context.Context, cfg config.Config) NowWebPlaybackSnapshot {
	if _, err := exec.LookPath("osascript"); err != nil {
		return NowWebPlaybackSnapshot{State: browsercontrol.PlaybackStateUnknown, Error: "osascript not found"}
	}
	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     cfg.Browser,
		BrowserApp:  cfg.BrowserApp,
		URLContains: cfg.URLContains,
	})
	if err != nil {
		return NowWebPlaybackSnapshot{State: browsercontrol.PlaybackStateUnknown, Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	st, err := controller.Status(ctx)
	if err != nil {
		return NowWebPlaybackSnapshot{State: browsercontrol.PlaybackStateUnknown, Error: err.Error()}
	}
	return NowWebPlaybackSnapshot{
		State:           st.State,
		PlaybackDetails: st.PlaybackDetails,
	}
}

func collectLocalStatus(ctx context.Context) (NowLocalStatus, []string) {
	controller, err := localplayback.New(localplayback.Options{
		StatePath: config.StatePath(),
		UserAgent: "pocketcastsctl",
	})
	if err != nil {
		return NowLocalStatus{Status: "error", Error: err.Error()}, nil
	}
	localCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	snapshot, err := controller.Snapshot(localCtx)
	if err != nil {
		return NowLocalStatus{Status: "error", Error: err.Error()}, snapshot.Warnings
	}
	return localStatusFromSnapshot(snapshot), snapshot.Warnings
}

func localStatusFromSnapshot(snapshot localplayback.Snapshot) NowLocalStatus {
	switch snapshot.Status {
	case localplayback.StatusPlaying:
		return NowLocalStatus{Status: "playing", Title: strings.TrimSpace(snapshot.Title)}
	case localplayback.StatusPaused:
		return NowLocalStatus{Status: "paused", Title: strings.TrimSpace(snapshot.Title)}
	default:
		return NowLocalStatus{Status: "stopped"}
	}
}

func collectNowAPIStatus(ctx context.Context, cfg config.Config, opts NowOptions, managerOpts authn.ManagerOptions) (NowAuthStatus, NowQueueStatus) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	policy := upNextRetryPolicy{attempts: 1}
	if opts.VerifyAuth {
		policy = upNextRetryPolicy{attempts: 2, baseDelay: 150 * time.Millisecond}
	}
	result := probeUpNext(ctx, cfg, managerOpts, policy)
	return result.authStatus(opts.VerifyAuth), result.queueStatus()
}

func suggestNowActions(s NowSnapshot) []string {
	actions := make([]string, 0, 6)
	add := func(v string) {
		for _, existing := range actions {
			if existing == v {
				return
			}
		}
		actions = append(actions, v)
	}

	if s.Auth.Status == "missing" || s.Auth.Status == "unauthorized" {
		add("pocketcastsctl auth login")
		add("pocketcastsctl auth import-browser --browser dia")
	}
	if s.Web.State == "paused" {
		add("pocketcastsctl web toggle")
	}
	if s.Web.State == "playing" {
		add("pocketcastsctl web next")
	}
	if s.Local.Status == "paused" {
		add("pocketcastsctl local resume")
	}
	if s.Local.Status == "stopped" && s.Queue.Total > 0 {
		add("pocketcastsctl local pick --in-progress --recent")
	}
	if s.Queue.Status == "ready" {
		add("pocketcastsctl queue api pick --recent")
	}
	if s.Queue.Status == "empty" {
		add("pocketcastsctl queue api ls")
	}
	if len(actions) == 0 {
		add("pocketcastsctl queue api ls")
	}
	return actions
}
