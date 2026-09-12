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

// SearchMovie resolves a title to a Trakt movie. A release is often named
// differently from Trakt's canonical title ("Vishwanath and Sons" against
// "Vishwanath & Sons"), and scrobbling by title alone fails for those.
// A miss returns (nil, nil) so the caller can fall back.
func (c *Client) SearchMovie(ctx context.Context, title string, year int) (*Movie, error) {
	results, err := c.search(ctx, "movie", title, year)
	if err != nil || len(results) == 0 {
		return nil, err
	}
	return results[0].Movie, nil
}

// SearchShow resolves a title to a Trakt show, on the same terms as SearchMovie.
func (c *Client) SearchShow(ctx context.Context, title string, year int) (*Show, error) {
	results, err := c.search(ctx, "show", title, year)
	if err != nil || len(results) == 0 {
		return nil, err
	}
	return results[0].Show, nil
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
