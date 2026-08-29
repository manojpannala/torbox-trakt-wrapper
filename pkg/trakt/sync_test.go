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

const sampleWatchedMoviesJSON = `[
  {
    "plays": 2,
    "last_watched_at": "2024-01-15T20:30:00.000Z",
    "last_updated_at": "2024-01-15T20:30:00.000Z",
    "movie": {
      "title": "Oppenheimer",
      "year": 2023,
      "ids": {
        "trakt": 615024,
        "slug": "oppenheimer-2023",
        "imdb": "tt15398776",
        "tmdb": 872585
      }
    }
  }
]`

const sampleWatchedShowsJSON = `[
  {
    "plays": 16,
    "last_watched_at": "2024-02-10T22:00:00.000Z",
    "last_updated_at": "2024-02-10T22:00:00.000Z",
    "show": {
      "title": "Breaking Bad",
      "year": 2008,
      "ids": {
        "trakt": 1388,
        "slug": "breaking-bad",
        "tvdb": 81189,
        "imdb": "tt0903747",
        "tmdb": 1396
      }
    },
    "seasons": [
      {
        "number": 1,
        "episodes": [
          {
            "number": 1,
            "plays": 2,
            "last_watched_at": "2024-01-01T12:00:00.000Z"
          },
          {
            "number": 2,
            "plays": 1,
            "last_watched_at": "2024-01-02T12:00:00.000Z"
          }
        ]
      }
    ]
  }
]`

const samplePlaybackJSON = `[
  {
    "id": 8001,
    "progress": 68.5,
    "paused_at": "2024-02-20T21:15:00.000Z",
    "type": "movie",
    "movie": {
      "title": "Dune: Part Two",
      "year": 2024,
      "ids": {
        "trakt": 644558,
        "slug": "dune-part-two-2024",
        "imdb": "tt15239678",
        "tmdb": 693134
      }
    }
  },
  {
    "id": 8002,
    "progress": 45.0,
    "paused_at": "2024-02-21T21:15:00.000Z",
    "type": "episode",
    "episode": {
      "season": 2,
      "number": 3,
      "title": "Episode 3",
      "ids": {
        "trakt": 99999
      }
    },
    "show": {
      "title": "Severance",
      "year": 2022,
      "ids": {
        "trakt": 156461
      }
    }
  }
]`

func TestSync_GetWatchedMovies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sync/watched/movies", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleWatchedMoviesJSON))
	}))
	defer server.Close()

	client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL), trakt.WithTokens(trakt.TokenResponse{
		AccessToken: "test-token",
	}))

	movies, err := client.GetWatchedMovies(context.Background())
	require.NoError(t, err)
	require.Len(t, movies, 1)
	assert.Equal(t, "Oppenheimer", movies[0].Movie.Title)
	assert.Equal(t, 2023, movies[0].Movie.Year)
	assert.Equal(t, 615024, movies[0].Movie.IDs.Trakt)
	assert.Equal(t, 2, movies[0].Plays)
}

func TestSync_GetWatchedShows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sync/watched/shows", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleWatchedShowsJSON))
	}))
	defer server.Close()

	client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL), trakt.WithTokens(trakt.TokenResponse{
		AccessToken: "test-token",
	}))

	shows, err := client.GetWatchedShows(context.Background())
	require.NoError(t, err)
	require.Len(t, shows, 1)
	assert.Equal(t, "Breaking Bad", shows[0].Show.Title)
	assert.Equal(t, 2008, shows[0].Show.Year)
	require.Len(t, shows[0].Seasons, 1)
	assert.Equal(t, 1, shows[0].Seasons[0].Number)
	require.Len(t, shows[0].Seasons[0].Episodes, 2)
	assert.Equal(t, 1, shows[0].Seasons[0].Episodes[0].Number)
	assert.Equal(t, 2, shows[0].Seasons[0].Episodes[0].Plays)
}

func TestSync_GetPlayback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sync/playback", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(samplePlaybackJSON))
	}))
	defer server.Close()

	client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL), trakt.WithTokens(trakt.TokenResponse{
		AccessToken: "test-token",
	}))

	playback, err := client.GetPlayback(context.Background())
	require.NoError(t, err)
	require.Len(t, playback, 2)

	assert.Equal(t, int64(8001), playback[0].ID)
	assert.Equal(t, 68.5, playback[0].Progress)
	assert.Equal(t, "movie", playback[0].Type)
	assert.Equal(t, "Dune: Part Two", playback[0].Movie.Title)

	assert.Equal(t, int64(8002), playback[1].ID)
	assert.Equal(t, 45.0, playback[1].Progress)
	assert.Equal(t, "episode", playback[1].Type)
	assert.Equal(t, "Severance", playback[1].Show.Title)
	assert.Equal(t, 2, playback[1].Episode.Season)
	assert.Equal(t, 3, playback[1].Episode.Number)
}

func TestSync_RemovePlayback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sync/playback/8001", r.URL.Path)
		assert.Equal(t, "DELETE", r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL), trakt.WithTokens(trakt.TokenResponse{
		AccessToken: "test-token",
	}))

	err := client.RemovePlayback(context.Background(), 8001)
	require.NoError(t, err)
}
