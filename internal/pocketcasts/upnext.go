package pocketcasts

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// UpNextSnapshot is one observation of Up Next. Episodes retains queue order
// and repeated occurrences; Progress contains played seconds by episode UUID.
// ParseError distinguishes an unknown or invalid response from a recognized
// empty queue. Raw remains available even when the response cannot be parsed.
type UpNextSnapshot struct {
	Episodes   []UpNextEpisode
	Progress   map[string]int
	Raw        []byte
	ParseError error
}

type upNextListRequest struct {
	Model          string `json:"model"`
	ServerModified string `json:"serverModified"`
	ShowPlayStatus bool   `json:"showPlayStatus"`
	Version        int    `json:"version"`
}

// UpNextList fetches and parses the queue, keeping Web Player protocol fields
// private. The returned error reports request failures; response parsing failures
// are recorded in the snapshot so raw output and auth verification remain usable.
func (c *Client) UpNextList(ctx context.Context, serverModified string) (UpNextSnapshot, error) {
	req := upNextListRequest{
		Model:          "webplayer",
		ServerModified: serverModified,
		ShowPlayStatus: true,
		Version:        2,
	}
	b, err := json.Marshal(req)
	if err != nil {
		return UpNextSnapshot{}, err
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, "/up_next/list", bytes.NewReader(b))
	if err != nil {
		return UpNextSnapshot{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.do(httpReq)
	if err != nil {
		return UpNextSnapshot{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UpNextSnapshot{}, err
	}
	if resp.StatusCode >= 400 {
		return UpNextSnapshot{}, NewHTTPError(resp.StatusCode, string(body))
	}
	return parseUpNextSnapshot(body), nil
}
