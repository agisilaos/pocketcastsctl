package browsercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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

type StatusResult struct {
	State string `json:"state"` // playing|paused|unknown
}

type QueueItem struct {
	Title string `json:"title"`
	Href  string `json:"href"`
}

func (c *Controller) Do(ctx context.Context, action Action) (ActionResult, error) {
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
	return res, nil
}

func (c *Controller) Status(ctx context.Context) (StatusResult, error) {
	out, err := c.runJS(ctx, jsStatus())
	if err != nil {
		return StatusResult{}, err
	}
	var st StatusResult
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		return StatusResult{}, fmt.Errorf("unexpected JS result: %q", out)
	}
	if st.State == "" {
		st.State = "unknown"
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
	return strings.TrimSpace(string(b)), nil
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
