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
	for k, v := range c.headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
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
