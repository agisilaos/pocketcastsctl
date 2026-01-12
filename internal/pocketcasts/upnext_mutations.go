package pocketcasts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// UpNextEpisode matches the shape observed in the Web Player HAR for /up_next/play_next.
// Fields are kept as strings to avoid surprising parsing differences.
type UpNextEpisode struct {
	Podcast   string `json:"podcast"`
	Published string `json:"published"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	UUID      string `json:"uuid"`
}

type upNextPlayNextRequest struct {
	Episode        UpNextEpisode `json:"episode"`
	Version        int           `json:"version"`
	ServerModified string        `json:"serverModified,omitempty"`
}

func (c *Client) UpNextPlayNext(ctx context.Context, episode UpNextEpisode, serverModified string) ([]byte, error) {
	reqBody := upNextPlayNextRequest{
		Episode:        episode,
		Version:        2,
		ServerModified: serverModified,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/up_next/play_next", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

type upNextRemoveRequest struct {
	UUIDs          []string `json:"uuids"`
	Version        int      `json:"version"`
	ServerModified string   `json:"serverModified,omitempty"`
}

func (c *Client) UpNextRemove(ctx context.Context, uuids []string, serverModified string) ([]byte, error) {
	reqBody := upNextRemoveRequest{
		UUIDs:          uuids,
		Version:        2,
		ServerModified: serverModified,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, "/up_next/remove", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}
