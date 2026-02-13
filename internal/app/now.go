package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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
	if !hasAuthorizationHeader(cfg.APIHeaders) {
		return NowQueueStatus{Status: "unauthorized", Error: "Authorization header missing"}
	}
	client := pocketcasts.New(pocketcasts.Options{BaseURL: cfg.APIBaseURL, Headers: cfg.APIHeaders})
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	body, err := client.UpNextList(ctx, pocketcasts.UpNextListRequest{
		Model:          "webplayer",
		ServerModified: "0",
		ShowPlayStatus: true,
		Version:        2,
	})
	if err != nil {
		if isUnauthorizedError(err) {
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
	auth := NowAuthStatus{Status: "missing", AuthorizationExists: hasAuthorizationHeader(cfg.APIHeaders)}
	if !auth.AuthorizationExists {
		return auth
	}
	auth.Status = "configured"
	if exp, ok := authTokenExpiry(cfg.APIHeaders); ok {
		auth.TokenExpiryKnown = true
		auth.TokenExpiryUnix = exp
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

func authTokenExpiry(headers map[string]string) (int64, bool) {
	for k, v := range headers {
		if !strings.EqualFold(strings.TrimSpace(k), "Authorization") {
			continue
		}
		raw := strings.TrimSpace(v)
		raw = strings.TrimPrefix(raw, "Bearer ")
		raw = strings.TrimPrefix(raw, "bearer ")
		parts := strings.Split(raw, ".")
		if len(parts) != 3 {
			return 0, false
		}
		payload, err := decodeJWTPart(parts[1])
		if err != nil {
			return 0, false
		}
		var m map[string]any
		if err := json.Unmarshal(payload, &m); err != nil {
			return 0, false
		}
		if f, ok := m["exp"].(float64); ok {
			return int64(f), true
		}
		return 0, false
	}
	return 0, false
}

func decodeJWTPart(s string) ([]byte, error) {
	if l := len(s) % 4; l != 0 {
		s += strings.Repeat("=", 4-l)
	}
	return base64.RawURLEncoding.DecodeString(s)
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
		add("pocketcastsctl auth refresh")
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

func formatRelativeExpiry(unix int64) string {
	if unix <= 0 {
		return ""
	}
	d := time.Until(time.Unix(unix, 0)).Round(time.Minute)
	if d <= 0 {
		return "expired"
	}
	if d < time.Hour {
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	}
	return fmt.Sprintf("in %dh", int(d.Hours()))
}

func shortTitle(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
