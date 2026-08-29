package trakt

import (
	"context"
	"fmt"
)

// GetWatchedMovies retrieves the user's watched movies history.
func (c *Client) GetWatchedMovies(ctx context.Context) ([]WatchedMovie, error) {
	var list []WatchedMovie
	if err := c.doRequest(ctx, "GET", "/sync/watched/movies", nil, &list, true); err != nil {
		return nil, err
	}
	return list, nil
}

// GetWatchedShows retrieves the user's watched TV shows and episode history.
func (c *Client) GetWatchedShows(ctx context.Context) ([]WatchedShow, error) {
	var list []WatchedShow
	if err := c.doRequest(ctx, "GET", "/sync/watched/shows", nil, &list, true); err != nil {
		return nil, err
	}
	return list, nil
}

// GetPlayback retrieves active in-progress media playback items.
func (c *Client) GetPlayback(ctx context.Context) ([]PlaybackItem, error) {
	var list []PlaybackItem
	if err := c.doRequest(ctx, "GET", "/sync/playback", nil, &list, true); err != nil {
		return nil, err
	}
	return list, nil
}

// RemovePlayback removes an in-progress playback item by ID.
func (c *Client) RemovePlayback(ctx context.Context, playbackID int64) error {
	path := fmt.Sprintf("/sync/playback/%d", playbackID)
	return c.doRequest(ctx, "DELETE", path, nil, nil, true)
}
