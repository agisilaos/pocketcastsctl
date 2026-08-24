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
	BaseURL     string
	Headers     map[string]string
	HTTP        *http.Client
	TokenSource TokenSource
}

// TokenSource supplies API access tokens and can refresh a rejected token.
// Implementations own persistence; Client never logs or stores token material.
type TokenSource interface {
	AccessToken(context.Context) (string, error)
	ForceRefresh(context.Context) (string, error)
}

type Client struct {
	baseURL     string
	headers     map[string]string
	http        *http.Client
	tokenSource TokenSource
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
		baseURL:     baseURL,
		headers:     cloneHeaderMap(opts.Headers),
		http:        hc,
		tokenSource: opts.TokenSource,
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
	if c.tokenSource != nil {
		token, err := c.tokenSource.AccessToken(ctx)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized || c.tokenSource == nil {
		return resp, err
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	token, refreshErr := c.tokenSource.ForceRefresh(req.Context())
	if refreshErr != nil {
		if errors.Is(refreshErr, ErrTokenNotRefreshable) {
			return nil, NewHTTPError(http.StatusUnauthorized, "configured API credential was rejected and cannot be refreshed")
		}
		return nil, fmt.Errorf("refresh API session after 401: %w", refreshErr)
	}
	retry, cloneErr := cloneRequest(req)
	if cloneErr != nil {
		return nil, fmt.Errorf("replay request after token refresh: %w", cloneErr)
	}
	retry.Header.Set("Authorization", "Bearer "+token)
	return c.http.Do(retry)
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	retry := req.Clone(req.Context())
	if req.Body == nil {
		return retry, nil
	}
	if req.GetBody == nil {
		return nil, errors.New("request body cannot be replayed")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	retry.Body = body
	return retry, nil
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
