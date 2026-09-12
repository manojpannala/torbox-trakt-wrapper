package trakt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestSearchMovie_ResolvesATitleTraktSpellsDifferently(t *testing.T) {
	// The release is named "Vishwanath and Sons"; Trakt calls it
	// "Vishwanath & Sons". Scrobbling by title alone 404s.
	server, gotPath := searchServer(t, vishwanathResult)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	movie, err := client.SearchMovie(context.Background(), "Vishwanath and Sons", 2026)

	require.NoError(t, err)
	require.NotNil(t, movie)
	assert.Equal(t, 1152187, movie.IDs.Trakt)
	assert.Equal(t, "Vishwanath & Sons", movie.Title)
	assert.Contains(t, *gotPath, "/search/movie")
	assert.Contains(t, *gotPath, "years=2026")
}

func TestSearchMovie_ReturnsNilWhenNothingMatches(t *testing.T) {
	server, _ := searchServer(t, `[]`)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	movie, err := client.SearchMovie(context.Background(), "No Such Film", 1999)

	require.NoError(t, err)
	assert.Nil(t, movie, "an empty result is not an error; the caller falls back")
}

func TestSearchMovie_OmitsTheYearWhenUnknown(t *testing.T) {
	server, gotPath := searchServer(t, vishwanathResult)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	_, err := client.SearchMovie(context.Background(), "Vishwanath and Sons", 0)

	require.NoError(t, err)
	assert.NotContains(t, *gotPath, "years=")
}

func TestSearchShow_ResolvesAShow(t *testing.T) {
	body := `[{"type":"show","score":900,"show":{"title":"Test Crime Series","year":2021,
	  "ids":{"trakt":1388,"slug":"test-crime-series"}}}]`
	server, gotPath := searchServer(t, body)
	client := trakt.NewClient("cid", "secret", trakt.WithBaseURL(server.URL))

	show, err := client.SearchShow(context.Background(), "Test Crime Series", 0)

	require.NoError(t, err)
	require.NotNil(t, show)
	assert.Equal(t, 1388, show.IDs.Trakt)
	assert.Contains(t, *gotPath, "/search/show")
}
