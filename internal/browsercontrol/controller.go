package browsercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Action string

const (
	ActionPlay   Action = "play"
	ActionPause  Action = "pause"
	ActionToggle Action = "toggle"
	ActionNext   Action = "next"
	ActionPrev   Action = "prev"
)

type Options struct {
	Browser     string
	BrowserApp  string
	URLContains string
}

type Controller struct {
	browser     browser
	urlContains string
}

func New(opts Options) (*Controller, error) {
	b, err := parseBrowser(opts.Browser, opts.BrowserApp)
	if err != nil {
		return nil, err
	}
	urlContains := strings.TrimSpace(opts.URLContains)
	if urlContains == "" {
		return nil, errors.New("url-contains cannot be empty")
	}
	return &Controller{browser: b, urlContains: urlContains}, nil
}

type ActionResult struct {
	Clicked      bool   `json:"clicked"`
	ClickedLabel string `json:"clickedLabel"`
}

type PlaybackDetails struct {
	EpisodeTitle    *string  `json:"episode_title,omitempty"`
	PodcastTitle    *string  `json:"podcast_title,omitempty"`
	PositionSeconds *int64   `json:"position_seconds,omitempty"`
	DurationSeconds *int64   `json:"duration_seconds,omitempty"`
	ProgressPercent *float64 `json:"progress_percent,omitempty"`
}

type PlaybackState string

const (
	PlaybackStatePlaying    PlaybackState = "playing"
	PlaybackStatePaused     PlaybackState = "paused"
	PlaybackStateLoading    PlaybackState = "loading"
	PlaybackStateTransition PlaybackState = "transition"
	PlaybackStateNoEpisode  PlaybackState = "no_episode"
	PlaybackStateUnknown    PlaybackState = "unknown"
)

type PlaybackSnapshot struct {
	State PlaybackState `json:"state"`
	PlaybackDetails
}

type QueueItem struct {
	Title string `json:"title"`
	Href  string `json:"href"`
}

func (c *Controller) Do(ctx context.Context, action Action) (ActionResult, error) {
	before := PlaybackStateUnknown
	verify := c.browser.kind == kindDia && isPlaybackStateAction(action)
	if verify {
		if snapshot, err := c.Status(ctx); err == nil {
			before = snapshot.State
		}
	}

	js := jsForAction(action)
	out, err := c.runJS(ctx, js)
	if err != nil {
		return ActionResult{}, err
	}

	var res ActionResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return ActionResult{}, fmt.Errorf("unexpected JS result: %q", out)
	}
	if !res.Clicked {
		return res, fmt.Errorf("no matching control found in page (action=%s)", action)
	}
	if verify {
		if err := c.verifyPlaybackAction(ctx, action, before, res.ClickedLabel); err != nil {
			return res, err
		}
	}
	return res, nil
}

func isPlaybackStateAction(action Action) bool {
	return action == ActionPlay || action == ActionPause || action == ActionToggle
}

func (c *Controller) verifyPlaybackAction(ctx context.Context, action Action, before PlaybackState, label string) error {
	timer := time.NewTimer(500 * time.Millisecond)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
	}

	snapshot, err := c.Status(ctx)
	if err != nil {
		return fmt.Errorf("%s could not verify the playback action: %w", c.browser.appName, err)
	}
	after := snapshot.State
	if playbackActionApplied(action, before, after) {
		return nil
	}
	if strings.TrimSpace(label) == "" {
		label = string(action)
	}
	return fmt.Errorf("%s reported %s but playback state remained %s", c.browser.appName, label, after)
}

func playbackActionApplied(action Action, before, after PlaybackState) bool {
	switch action {
	case ActionPlay:
		return after == PlaybackStatePlaying || after == PlaybackStateLoading || after == PlaybackStateTransition
	case ActionPause:
		return after == PlaybackStatePaused
	case ActionToggle:
		switch before {
		case PlaybackStatePaused:
			return after == PlaybackStatePlaying || after == PlaybackStateLoading || after == PlaybackStateTransition
		case PlaybackStatePlaying, PlaybackStateLoading:
			return after == PlaybackStatePaused
		default:
			return after != before && after != PlaybackStateUnknown
		}
	default:
		return true
	}
}

func (c *Controller) Status(ctx context.Context) (PlaybackSnapshot, error) {
	out, err := c.runJS(ctx, jsStatus())
	if err != nil {
		return PlaybackSnapshot{}, err
	}
	var st PlaybackSnapshot
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		return PlaybackSnapshot{}, fmt.Errorf("unexpected JS result: %q", out)
	}
	switch st.State {
	case PlaybackStatePlaying,
		PlaybackStatePaused,
		PlaybackStateLoading,
		PlaybackStateTransition,
		PlaybackStateNoEpisode,
		PlaybackStateUnknown:
	default:
		st.State = PlaybackStateUnknown
	}
	return st, nil
}

func (c *Controller) QueueList(ctx context.Context) ([]QueueItem, error) {
	out, err := c.runJS(ctx, jsQueueList())
	if err != nil {
		return nil, err
	}
	var items []QueueItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return nil, fmt.Errorf("unexpected JS result: %q", out)
	}
	return items, nil
}

func (c *Controller) runJS(ctx context.Context, js string) (string, error) {
	script := c.browser.appleScript()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script, c.browser.appName, c.urlContains, js)
	b, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return unwrapJavaScriptResult(strings.TrimSpace(string(b))), nil
}

func unwrapJavaScriptResult(result string) string {
	var unwrapped string
	if err := json.Unmarshal([]byte(result), &unwrapped); err == nil && json.Valid([]byte(unwrapped)) {
		return unwrapped
	}
	return result
}

func (c *Controller) SetTabURL(ctx context.Context, newURL string) error {
	newURL = strings.TrimSpace(newURL)
	if newURL == "" {
		return errors.New("new URL cannot be empty")
	}

	script := c.browser.appleScriptSetURL()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script, c.browser.appName, c.urlContains, newURL)
	b, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func (c *Controller) TabURLs(ctx context.Context) ([]string, error) {
	script := c.browser.appleScriptListURLs()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script, c.browser.appName)
	b, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	var urls []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &urls); err != nil {
		return nil, fmt.Errorf("unexpected JS result: %q", strings.TrimSpace(string(b)))
	}
	return urls, nil
}
