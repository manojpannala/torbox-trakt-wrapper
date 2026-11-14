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
	results, err := c.search(ctx, "movie", title, year, false)
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
	results, err := c.search(ctx, "show", title, year, false)
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

// MaxTitleHits caps SearchTitles; past ten, Trakt's hits are rarely wanted.
const MaxTitleHits = 10

// TitleHit is one movie or show from a free-text title search.
type TitleHit struct {
	Kind     string `json:"kind"` // "movie" or "show"
	Title    string `json:"title"`
	Year     int    `json:"year,omitempty"`
	Language string `json:"language,omitempty"`
	IDs      IDs    `json:"ids"`
}

// SearchTitles looks up free text as movies and shows together, keeping
// Trakt's order, so a misspelt title still finds the work.
func (c *Client) SearchTitles(ctx context.Context, text string) ([]TitleHit, error) {
	results, err := c.search(ctx, "movie,show", text, 0, true)
	if err != nil {
		return nil, err
	}
	return titleHits(results), nil
}

// LookupIMDb finds the movie or show a typed IMDb ID belongs to, so its
// title can drive the text search; it returns nil when Trakt has no match.
func (c *Client) LookupIMDb(ctx context.Context, imdbID string) (*TitleHit, error) {
	var results []searchResult
	path := fmt.Sprintf("/search/imdb/%s?type=movie,show&extended=full", url.PathEscape(imdbID))
	if err := c.doRequest(ctx, "GET", path, nil, &results, false); err != nil {
		return nil, err
	}
	hits := titleHits(results)
	if len(hits) == 0 {
		return nil, nil
	}
	return &hits[0], nil
}

func titleHits(results []searchResult) []TitleHit {
	hits := make([]TitleHit, 0, min(len(results), MaxTitleHits))
	for _, r := range results {
		if len(hits) == MaxTitleHits {
			break
		}
		switch {
		case r.Movie != nil:
			hits = append(hits, TitleHit{Kind: "movie", Title: r.Movie.Title, Year: r.Movie.Year, Language: hitLanguage(r.Movie.Language, r.Movie.Languages), IDs: r.Movie.IDs})
		case r.Show != nil:
			hits = append(hits, TitleHit{Kind: "show", Title: r.Show.Title, Year: r.Show.Year, Language: hitLanguage(r.Show.Language, r.Show.Languages), IDs: r.Show.IDs})
		}
	}
	return hits
}

func hitLanguage(lang string, langs []string) string {
	if lang != "" {
		return lang
	}
	if len(langs) > 0 {
		return langs[0]
	}
	return ""
}

func (c *Client) search(ctx context.Context, kind, title string, year int, extended bool) ([]searchResult, error) {
	q := url.Values{}
	q.Set("query", title)
	if year > 0 {
		q.Set("years", fmt.Sprintf("%d", year))
	}
	if extended {
		q.Set("extended", "full")
	}

	var results []searchResult
	path := fmt.Sprintf("/search/%s?%s", kind, q.Encode())
	if err := c.doRequest(ctx, "GET", path, nil, &results, false); err != nil {
		return nil, err
	}
	return results, nil
}
