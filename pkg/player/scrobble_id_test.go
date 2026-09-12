package player_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type traktStub struct {
	mu           sync.Mutex
	searchBody   string
	searches     int
	scrobbleBody map[string]any
}

func newTraktStub(t *testing.T, searchBody string) (*traktStub, *trakt.Client) {
	t.Helper()
	s := &traktStub{searchBody: searchBody}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		s.mu.Lock()
		defer s.mu.Unlock()

		if r.URL.Path == "/search/movie" || r.URL.Path == "/search/show" {
			s.searches++
			_, _ = w.Write([]byte(s.searchBody))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &s.scrobbleBody)
		_, _ = w.Write([]byte(`{"action":"start","progress":1}`))
	}))
	t.Cleanup(server.Close)

	client := trakt.NewClient("cid", "secret",
		trakt.WithBaseURL(server.URL),
		trakt.WithTokens(trakt.TokenResponse{AccessToken: "tok", ExpiresIn: 99999}),
	)
	return s, client
}

func (s *traktStub) movieIDs(t *testing.T) map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.NotNil(t, s.scrobbleBody, "no scrobble was sent")
	movie, ok := s.scrobbleBody["movie"].(map[string]any)
	require.True(t, ok, "scrobble payload had no movie: %v", s.scrobbleBody)
	ids, _ := movie["ids"].(map[string]any)
	return ids
}

// Trakt calls this movie "Vishwanath & Sons"; the release is named
// "Vishwanath and Sons". Scrobbling by title alone returned 404.
const searchHit = `[{"type":"movie","movie":{"title":"Vishwanath & Sons","year":2026,
  "ids":{"trakt":1152187,"slug":"vishwanath-sons-2026"}}}]`

func vishwanath() matcher.ParsedMedia {
	return matcher.ParsedMedia{
		CleanTitle: "Vishwanath and Sons",
		Year:       2026,
		Type:       matcher.MediaTypeMovie,
	}
}

func TestScrobbler_SendsTheResolvedTraktID(t *testing.T) {
	stub, client := newTraktStub(t, searchHit)

	require.NoError(t, player.NewTraktScrobbler(client).Start(context.Background(), vishwanath(), 1))

	assert.Equal(t, float64(1152187), stub.movieIDs(t)["trakt"],
		"without the id Trakt cannot resolve a differently-spelled title")
}

func TestScrobbler_ResolvesOncePerTitle(t *testing.T) {
	stub, client := newTraktStub(t, searchHit)
	s := player.NewTraktScrobbler(client)
	ctx := context.Background()

	require.NoError(t, s.Start(ctx, vishwanath(), 1))
	require.NoError(t, s.Pause(ctx, vishwanath(), 40))
	_, err := s.Stop(ctx, vishwanath(), 90)
	require.NoError(t, err)

	stub.mu.Lock()
	defer stub.mu.Unlock()
	assert.Equal(t, 1, stub.searches, "one playback must not cost three searches")
}

func TestScrobbler_IgnoresAResultThatIsADifferentTitle(t *testing.T) {
	wrong := `[{"type":"movie","movie":{"title":"Something Else Entirely","year":2026,
	  "ids":{"trakt":999999}}}]`
	stub, client := newTraktStub(t, wrong)

	require.NoError(t, player.NewTraktScrobbler(client).Start(context.Background(), vishwanath(), 1))

	assert.Equal(t, float64(0), stub.movieIDs(t)["trakt"],
		"scrobbling the wrong film is worse than not scrobbling")
}

func TestScrobbler_FallsBackToTitleWhenSearchFindsNothing(t *testing.T) {
	stub, client := newTraktStub(t, `[]`)

	require.NoError(t, player.NewTraktScrobbler(client).Start(context.Background(), vishwanath(), 1))

	stub.mu.Lock()
	defer stub.mu.Unlock()
	movie := stub.scrobbleBody["movie"].(map[string]any)
	assert.Equal(t, "Vishwanath and Sons", movie["title"], "behaviour must be no worse than before")
}

// A fresh install has no Trakt client id, so NewAppModel builds the scrobbler
// around a nil client. Playing anything then panicked in the monitor
// goroutine, taking the whole TUI with it.
func TestScrobbler_WithoutATraktClientIsANoOp(t *testing.T) {
	s := player.NewTraktScrobbler(nil)
	ctx := context.Background()

	assert.NotPanics(t, func() {
		assert.NoError(t, s.Start(ctx, vishwanath(), 1))
		assert.NoError(t, s.Pause(ctx, vishwanath(), 40))
		resp, err := s.Stop(ctx, vishwanath(), 90)
		assert.NoError(t, err)
		assert.Nil(t, resp)
	})
}
