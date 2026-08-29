package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

func (c *Client) scrobbleAction(ctx context.Context, action string, req ScrobbleRequest) (*ScrobbleResponse, error) {
	if req.Movie == nil && (req.Show == nil && req.Episode == nil) {
		return nil, fmt.Errorf("either Movie or Show/Episode metadata is required for scrobbling")
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling scrobble payload: %w", err)
	}

	path := fmt.Sprintf("/scrobble/%s", action)
	var resp ScrobbleResponse
	if err := c.doRequest(ctx, "POST", path, bytes.NewReader(payload), &resp, true); err != nil {
		return nil, err
	}

	return &resp, nil
}

// StartScrobble notifies Trakt that media playback has started.
func (c *Client) StartScrobble(ctx context.Context, req ScrobbleRequest) (*ScrobbleResponse, error) {
	return c.scrobbleAction(ctx, "start", req)
}

// PauseScrobble notifies Trakt that media playback is paused.
func (c *Client) PauseScrobble(ctx context.Context, req ScrobbleRequest) (*ScrobbleResponse, error) {
	return c.scrobbleAction(ctx, "pause", req)
}

// StopScrobble notifies Trakt that media playback has stopped (or completed).
func (c *Client) StopScrobble(ctx context.Context, req ScrobbleRequest) (*ScrobbleResponse, error) {
	return c.scrobbleAction(ctx, "stop", req)
}
