package trakt_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleScrobbleResponseJSON = `{
  "id": 9001,
  "action": "scrobble",
  "progress": 95.0,
  "sharing": {
    "twitter": false,
    "mastodon": false
  },
  "movie": {
    "title": "Oppenheimer",
    "year": 2023,
    "ids": {
      "trakt": 615024
    }
  }
}`

func TestScrobble_Actions(t *testing.T) {
	actions := []struct {
		name     string
		call     func(c *trakt.Client) (*trakt.ScrobbleResponse, error)
		expected string
	}{
		{
			name: "Start",
			call: func(c *trakt.Client) (*trakt.ScrobbleResponse, error) {
				return c.StartScrobble(context.Background(), trakt.ScrobbleRequest{
					Movie: &trakt.Movie{
						Title: "Oppenheimer",
						Year:  2023,
					},
					Progress: 1.0,
				})
			},
			expected: "/scrobble/start",
		},
		{
			name: "Pause",
			call: func(c *trakt.Client) (*trakt.ScrobbleResponse, error) {
				return c.PauseScrobble(context.Background(), trakt.ScrobbleRequest{
					Movie: &trakt.Movie{
						Title: "Oppenheimer",
						Year:  2023,
					},
					Progress: 50.0,
				})
			},
			expected: "/scrobble/pause",
		},
		{
			name: "Stop",
			call: func(c *trakt.Client) (*trakt.ScrobbleResponse, error) {
				return c.StopScrobble(context.Background(), trakt.ScrobbleRequest{
					Movie: &trakt.Movie{
						Title: "Oppenheimer",
						Year:  2023,
					},
					Progress: 95.0,
				})
			},
			expected: "/scrobble/stop",
		},
	}

	for _, tc := range actions {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.expected, r.URL.Path)
				assert.Equal(t, "POST", r.Method)
				body, _ := io.ReadAll(r.Body)
				assert.Contains(t, string(body), "Oppenheimer")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(sampleScrobbleResponseJSON))
			}))
			defer server.Close()

			client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL), trakt.WithTokens(trakt.TokenResponse{
				AccessToken: "test-token",
			}))

			resp, err := tc.call(client)
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, int64(9001), resp.ID)
			assert.Equal(t, "scrobble", resp.Action)
			assert.Equal(t, 95.0, resp.Progress)
		})
	}
}

func TestScrobble_Episode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/scrobble/start", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "Breaking Bad")
		assert.Contains(t, string(body), `"season":1`)
		assert.Contains(t, string(body), `"number":1`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id": 9002,
			"action": "start",
			"progress": 0.5,
			"show": {"title": "Breaking Bad"},
			"episode": {"season": 1, "number": 1}
		}`))
	}))
	defer server.Close()

	client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL), trakt.WithTokens(trakt.TokenResponse{
		AccessToken: "test-token",
	}))

	resp, err := client.StartScrobble(context.Background(), trakt.ScrobbleRequest{
		Show: &trakt.Show{
			Title: "Breaking Bad",
		},
		Episode: &trakt.Episode{
			Season: 1,
			Number: 1,
		},
		Progress: 0.5,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(9002), resp.ID)
	assert.Equal(t, "start", resp.Action)
}

func TestScrobble_MissingMetadata(t *testing.T) {
	client := trakt.NewClient("cid", "csec")
	_, err := client.StartScrobble(context.Background(), trakt.ScrobbleRequest{
		Progress: 50.0,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}
