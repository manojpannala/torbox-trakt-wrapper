package trakt

import (
	"context"
	"fmt"
	"net/url"
)

type searchResult struct {
	Movie *Movie `json:"movie,omitempty"`
	Show  *Show  `json:"show,omitempty"`
}

// SearchMovies resolves a title to Trakt movies, best match first.
func (c *Client) SearchMovies(ctx context.Context, title string, year int) ([]Movie, error) {
	results, err := c.search(ctx, "movie", title, year)
	if err != nil {
		return nil, err
	}
	movies := make([]Movie, 0, len(results))
	for _, r := range results {
		if r.Movie != nil {
			movies = append(movies, *r.Movie)
		}
	}
	return movies, nil
}

// SearchShows resolves a title to Trakt shows, best match first.
func (c *Client) SearchShows(ctx context.Context, title string, year int) ([]Show, error) {
	results, err := c.search(ctx, "show", title, year)
	if err != nil {
		return nil, err
	}
	shows := make([]Show, 0, len(results))
	for _, r := range results {
		if r.Show != nil {
			shows = append(shows, *r.Show)
		}
	}
	return shows, nil
}

func (c *Client) search(ctx context.Context, kind, title string, year int) ([]searchResult, error) {
	q := url.Values{}
	q.Set("query", title)
	if year > 0 {
		q.Set("years", fmt.Sprintf("%d", year))
	}

	var results []searchResult
	path := fmt.Sprintf("/search/%s?%s", kind, q.Encode())
	if err := c.doRequest(ctx, "GET", path, nil, &results, false); err != nil {
		return nil, err
	}
	return results, nil
}
