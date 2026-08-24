package app

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/player"
	"pocketcastsctl/internal/pocketcasts"
	"pocketcastsctl/internal/state"
)

type NowOptions struct {
	VerifyAuth bool
}

type nowCollectorFuncs struct {
	web   func(context.Context) NowWebPlaybackSnapshot
	local func() NowLocalStatus
	auth  func(context.Context) NowAuthStatus
	queue func(context.Context) NowQueueStatus
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
	State string `json:"status"` // JSON name retained for compatibility
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
		local: func() NowLocalStatus {
			return collectLocalStatus(cfg)
		},
		auth: func(ctx context.Context) NowAuthStatus {
			return collectAuthStatus(ctx, cfg, opts)
		},
		queue: func(ctx context.Context) NowQueueStatus {
			return collectQueueStatus(ctx, cfg)
		},
	})
}

func collectNowSnapshot(ctx context.Context, configPath string, collectors nowCollectorFuncs) NowSnapshot {
	generatedAt := time.Now()
	var web NowWebPlaybackSnapshot
	var local NowLocalStatus
	var auth NowAuthStatus
	var queue NowQueueStatus
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		web = collectors.web(ctx)
	}()
	go func() {
		defer wg.Done()
		local = collectors.local()
	}()
	go func() {
		defer wg.Done()
		auth = collectors.auth(ctx)
	}()
	go func() {
		defer wg.Done()
		queue = collectors.queue(ctx)
	}()
	wg.Wait()

	s := NowSnapshot{
		GeneratedAt: generatedAt,
		Web:         web,
		Local:       local,
		Queue:       queue,
		Auth:        auth,
		Meta: map[string]string{
			"config_path": configPath,
		},
	}
	s.Actions = suggestNowActions(s)
	return s
}

func collectWebPlaybackSnapshot(ctx context.Context, cfg config.Config) NowWebPlaybackSnapshot {
	if _, err := exec.LookPath("osascript"); err != nil {
		return NowWebPlaybackSnapshot{State: "unavailable", Error: "osascript not found"}
	}
	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     cfg.Browser,
		BrowserApp:  cfg.BrowserApp,
		URLContains: cfg.URLContains,
	})
	if err != nil {
		return NowWebPlaybackSnapshot{State: "unavailable", Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	st, err := controller.Status(ctx)
	if err != nil {
		return NowWebPlaybackSnapshot{State: "unavailable", Error: err.Error()}
	}
	status := strings.TrimSpace(string(st.State))
	if status == "" {
		status = "unknown"
	}
	return NowWebPlaybackSnapshot{
		State:           status,
		PlaybackDetails: st.PlaybackDetails,
	}
}

func collectLocalStatus(cfg config.Config) NowLocalStatus {
	st, ok, err := state.Load(config.StatePath())
	if err != nil {
		return NowLocalStatus{Status: "error", Error: err.Error()}
	}
	if !ok {
		return NowLocalStatus{Status: "stopped"}
	}
	if !player.Alive(st.PID) {
		_ = state.Clear(config.StatePath())
		return NowLocalStatus{Status: "stopped"}
	}
	if st.Paused {
		return NowLocalStatus{Status: "paused", Title: strings.TrimSpace(st.Title)}
	}
	return NowLocalStatus{Status: "playing", Title: strings.TrimSpace(st.Title)}
}

func collectQueueStatus(ctx context.Context, cfg config.Config) NowQueueStatus {
	client, _ := authn.NewPocketCastsClient(cfg, authn.ManagerOptions{})
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	body, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          "webplayer",
		ServerModified: "0",
		ShowPlayStatus: true,
		Version:        2,
	})
	if err != nil {
		if isMissingAuthError(err) {
			return NowQueueStatus{Status: "unauthorized", Error: "API authentication is not configured"}
		}
		if authutil.IsUnauthorizedError(err) {
			return NowQueueStatus{Status: "unauthorized", Error: "API returned 401 Unauthorized"}
		}
		return NowQueueStatus{Status: "unavailable", Error: err.Error()}
	}
	eps, err := pocketcasts.ExtractUpNextEpisodes(body)
	if err != nil {
		return NowQueueStatus{Status: "unavailable", Error: "failed to parse queue"}
	}
	if len(eps) == 0 {
		return NowQueueStatus{Status: "empty", Total: 0}
	}
	progress, _ := pocketcasts.ExtractEpisodeProgress(body)
	inProgress := 0
	for _, p := range progress {
		if p > 0 {
			inProgress++
		}
	}
	return NowQueueStatus{
		Status:          "ready",
		Total:           len(eps),
		NextTitle:       strings.TrimSpace(eps[0].Title),
		InProgressCount: inProgress,
	}
}

func collectAuthStatus(ctx context.Context, cfg config.Config, opts NowOptions) NowAuthStatus {
	manager := authn.NewManager(cfg, authn.ManagerOptions{})
	session, source, loadErr := manager.Snapshot(ctx)
	auth := NowAuthStatus{Status: "missing"}
	if loadErr != nil || session.AccessToken == "" {
		if loadErr != nil && !isMissingAuthError(loadErr) {
			auth.Error = loadErr.Error()
		}
		return auth
	}
	auth.AuthorizationExists = true
	auth.Source = string(source)
	auth.Method = session.Method
	auth.Status = "configured"
	if session.ExpiresAt > 0 {
		auth.TokenExpiryKnown = true
		auth.TokenExpiryUnix = session.ExpiresAt
	}
	if !opts.VerifyAuth {
		return auth
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	err := VerifyAuth(ctx, cfg, VerifyOptions{Attempts: 2, BaseDelay: 150 * time.Millisecond})
	if err == nil {
		auth.Status = "verified"
		return auth
	}
	switch KindOf(err) {
	case KindUnauthorized:
		auth.Status = "unauthorized"
	default:
		auth.Status = "unverified"
	}
	auth.Error = err.Error()
	return auth
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
