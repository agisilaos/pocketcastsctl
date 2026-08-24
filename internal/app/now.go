package app

import (
	"context"
	"os/exec"
	"strings"
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

type NowSnapshot struct {
	GeneratedAt time.Time         `json:"generated_at"`
	Web         NowWebStatus      `json:"web"`
	Local       NowLocalStatus    `json:"local"`
	Queue       NowQueueStatus    `json:"queue"`
	Auth        NowAuthStatus     `json:"auth"`
	Actions     []string          `json:"actions"`
	Warnings    []string          `json:"warnings,omitempty"`
	Meta        map[string]string `json:"meta,omitempty"`
}

type NowWebStatus struct {
	Status string `json:"status"` // playing|paused|unknown|unavailable
	Error  string `json:"error,omitempty"`
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
	s := NowSnapshot{
		GeneratedAt: time.Now(),
		Web:         NowWebStatus{Status: "unavailable"},
		Local:       NowLocalStatus{Status: "stopped"},
		Queue:       NowQueueStatus{Status: "unavailable"},
		Auth:        NowAuthStatus{Status: "missing", AuthorizationExists: false},
		Meta: map[string]string{
			"config_path": config.Path(),
		},
	}

	s.Web = collectWebStatus(ctx, cfg)
	s.Local = collectLocalStatus(cfg)
	s.Auth = collectAuthStatus(ctx, cfg, opts)
	s.Queue = collectQueueStatus(ctx, cfg)
	s.Actions = suggestNowActions(s)
	return s
}

func collectWebStatus(ctx context.Context, cfg config.Config) NowWebStatus {
	if _, err := exec.LookPath("osascript"); err != nil {
		return NowWebStatus{Status: "unavailable", Error: "osascript not found"}
	}
	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     cfg.Browser,
		BrowserApp:  cfg.BrowserApp,
		URLContains: cfg.URLContains,
	})
	if err != nil {
		return NowWebStatus{Status: "unavailable", Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	st, err := controller.Status(ctx)
	if err != nil {
		return NowWebStatus{Status: "unavailable", Error: err.Error()}
	}
	status := strings.TrimSpace(st.State)
	if status == "" {
		status = "unknown"
	}
	return NowWebStatus{Status: status}
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
	if s.Web.Status == "paused" {
		add("pocketcastsctl web toggle")
	}
	if s.Web.Status == "playing" {
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
