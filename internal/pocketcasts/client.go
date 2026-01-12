package pocketcasts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrNotImplemented = errors.New("not implemented (capture Pocket Casts web endpoints first)")

type Options struct {
	BaseURL string
	Headers map[string]string
	HTTP    *http.Client
}

type Client struct {
	baseURL string
	headers map[string]string
	http    *http.Client
}

func New(opts Options) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://play.pocketcasts.com"
	}
	hc := opts.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		baseURL: baseURL,
		headers: cloneHeaderMap(opts.Headers),
		http:    hc,
	}
}

func (c *Client) Queue(ctx context.Context) ([]QueueItem, error) {
	_ = ctx
	return nil, ErrNotImplemented
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	applyDefaultAPIHeaders(req.Header, c.headers)
	return req, nil
}

func cloneHeaderMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func (c *Client) debugString() string {
	var keys []string
	for k := range c.headers {
		keys = append(keys, k)
	}
	return fmt.Sprintf("baseURL=%s headers=%v", c.baseURL, keys)
}

func applyDefaultAPIHeaders(h http.Header, user map[string]string) {
	// Mimic the Pocket Casts Web Player requests closely enough to avoid CORS/authorization surprises.
	defaults := map[string]string{
		"Accept":          "*/*",
		"Accept-Language": "en-US,en;q=0.9",
		"Content-Type":    "application/json",
		"Origin":          "https://pocketcasts.com",
		"Referer":         "https://pocketcasts.com/",
		"DNT":             "1",
		"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36",
		"X-App-Language":  "en",
		"X-User-Region":   "us",
	}
	for k, v := range defaults {
		if h.Get(k) == "" {
			h.Set(k, v)
		}
	}
	for k, v := range user {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		h.Set(k, v)
	}
}
