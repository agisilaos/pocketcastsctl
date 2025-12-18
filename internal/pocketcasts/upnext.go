package pocketcasts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type UpNextListRequest struct {
	Model          string `json:"model"`
	ServerModified string `json:"serverModified"`
	ShowPlayStatus bool   `json:"showPlayStatus"`
	Version        int    `json:"version"`
}

func (c *Client) UpNextList(ctx context.Context, req UpNextListRequest) ([]byte, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := c.newRequest(ctx, http.MethodPost, "/up_next/list", bytes.NewReader(b))
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

