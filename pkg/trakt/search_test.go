package trakt_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

// The real shape of GET /search/movie, trimmed to what we consume.
const vishwanathResult = `[{"type":"movie","score":1736172819,"movie":{
  "title":"Vishwanath & Sons","year":2026,
  "ids":{"trakt":1152187,"slug":"vishwanath-sons-2026","imdb":"tt99999","tmdb":123}}}]`

func searchServer(t *testing.T, body string) (*httptest.Server, *string) {
	t.Helper()
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &gotPath
}

func TestSearchMovies_ResolvesATitleTraktSpellsDifferently(t *testing.T) {
	// The release is named "Vishwanath and Sons"; Trakt calls it
	// "Vishwanath & Sons". Scrobbling by title alone 404s.
	server, gotPath := searchServer(t, vishwanathResult)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	movies, err := client.SearchMovies(context.Background(), "Vishwanath and Sons", 2026)

	require.NoError(t, err)
	require.Len(t, movies, 1)
	movie := movies[0]
	assert.Equal(t, 1152187, movie.IDs.Trakt)
	assert.Equal(t, "Vishwanath & Sons", movie.Title)
	assert.Contains(t, *gotPath, "/search/movie")
	assert.Contains(t, *gotPath, "years=2026")
}

func TestSearchMovies_ReturnsNilWhenNothingMatches(t *testing.T) {
	server, _ := searchServer(t, `[]`)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	movies, err := client.SearchMovies(context.Background(), "No Such Film", 1999)

	require.NoError(t, err)
	assert.Empty(t, movies, "an empty result is not an error; the caller falls back")
}

func TestSearchMovies_OmitsTheYearWhenUnknown(t *testing.T) {
	server, gotPath := searchServer(t, vishwanathResult)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	_, err := client.SearchMovies(context.Background(), "Vishwanath and Sons", 0)

	require.NoError(t, err)
	assert.NotContains(t, *gotPath, "years=")
}

func TestSearchShows_ResolvesAShow(t *testing.T) {
	body := `[{"type":"show","score":900,"show":{"title":"Test Crime Series","year":2021,
	  "ids":{"trakt":1388,"slug":"test-crime-series"}}}]`
	server, gotPath := searchServer(t, body)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	shows, err := client.SearchShows(context.Background(), "Test Crime Series", 0)

	require.NoError(t, err)
	require.Len(t, shows, 1)
	show := shows[0]
	assert.Equal(t, 1388, show.IDs.Trakt)
	assert.Contains(t, *gotPath, "/search/show")
}

func TestSearchTitles_MixesMoviesAndShowsInTraktOrder(t *testing.T) {
	body := `[
	  {"type":"show","score":900,"show":{"title":"Sample Show","year":2008,"ids":{"trakt":2,"imdb":"tt0000002"}}},
	  {"type":"movie","score":800,"movie":{"title":"Sample Film","year":2014,"ids":{"trakt":1,"imdb":"tt0000001"}}}]`
	server, gotPath := searchServer(t, body)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	hits, err := client.SearchTitles(context.Background(), "sampel")

	require.NoError(t, err)
	require.Len(t, hits, 2)
	assert.Equal(t, trakt.TitleHit{Kind: "show", Title: "Sample Show", Year: 2008, IDs: trakt.IDs{Trakt: 2, IMDB: "tt0000002"}}, hits[0])
	assert.Equal(t, "movie", hits[1].Kind)
	assert.Equal(t, "tt0000001", hits[1].IDs.IMDB)
	assert.Contains(t, *gotPath, "/search/movie,show?")
	assert.Contains(t, *gotPath, "query=sampel")
	assert.NotContains(t, *gotPath, "years=")
}

func TestSearchTitles_KeepsAtMostTenHits(t *testing.T) {
	var b strings.Builder
	b.WriteString("[")
	for i := range 15 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"type":"movie","movie":{"title":"Film %d","year":2000,"ids":{"trakt":%d}}}`, i, i+1)
	}
	b.WriteString("]")
	server, _ := searchServer(t, b.String())
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	hits, err := client.SearchTitles(context.Background(), "film")

	require.NoError(t, err)
	assert.Len(t, hits, trakt.MaxTitleHits)
	assert.Equal(t, "Film 0", hits[0].Title)
}

func TestLookupIMDb_ReturnsTheWorkTheIDBelongsTo(t *testing.T) {
	body := `[{"type":"show","score":1,"show":{"title":"Sample Show","year":2008,"ids":{"trakt":2,"imdb":"tt0000002"}}}]`
	server, gotPath := searchServer(t, body)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	hit, err := client.LookupIMDb(context.Background(), "tt0000002")

	require.NoError(t, err)
	require.NotNil(t, hit)
	assert.Equal(t, "show", hit.Kind)
	assert.Equal(t, "Sample Show", hit.Title)
	assert.Equal(t, "/search/imdb/tt0000002?type=movie,show", *gotPath)
}

func TestLookupIMDb_NilWhenTraktHasNoMatch(t *testing.T) {
	server, _ := searchServer(t, `[]`)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	hit, err := client.LookupIMDb(context.Background(), "tt9999999")

	require.NoError(t, err)
	assert.Nil(t, hit)
}
