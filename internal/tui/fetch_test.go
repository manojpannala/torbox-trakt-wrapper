package tui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type recorder struct {
	mu   sync.Mutex
	urls []string
}

func (r *recorder) add(u string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.urls = append(r.urls, u)
}

func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.urls...)
}

func stubAPI(t *testing.T, env string, routes map[string]func(http.ResponseWriter)) *recorder {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		if h, ok := routes[r.URL.Path]; ok {
			h(w)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	t.Setenv(env, srv.URL)
	return rec
}

func respond(status int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func testModelWith(t *testing.T, mutate func(*config.Config), opts ...AppOption) AppModel {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "test-api-key"
	cfg.Trakt.ClientID = "test-client-id"
	cfg.Trakt.AccessToken = "test-access-token"
	if mutate != nil {
		mutate(cfg)
	}
	return NewAppModel(context.Background(), cfg, opts...)
}

func testModel(t *testing.T, opts ...AppOption) AppModel {
	t.Helper()
	return testModelWith(t, nil, opts...)
}

func TestFetchTraktCatalog_ReportsFailureRatherThanAnEmptyCatalog(t *testing.T) {
	stubAPI(t, "TRAKT_BASE_URL", map[string]func(http.ResponseWriter){
		"/sync/watched/movies": respond(200, `[]`),
		"/sync/watched/shows":  respond(200, `[]`),
		"/sync/playback":       respond(500, `{}`),
	})
	m := testModel(t)

	msg := m.fetchTraktCatalogCmd()()

	_, failed := msg.(TraktCatalogFailedMsg)
	assert.True(t, failed, "an empty-looking catalog would overwrite real cached data; got %T", msg)
}

func TestFetchTraktCatalog_ReturnsTheCatalogOnSuccess(t *testing.T) {
	stubAPI(t, "TRAKT_BASE_URL", map[string]func(http.ResponseWriter){
		"/sync/watched/movies": respond(200, `[{"plays":1,"movie":{"title":"Test Feature Alpha","year":2023,"ids":{"trakt":1}}}]`),
		"/sync/watched/shows":  respond(200, `[]`),
		"/sync/playback":       respond(200, `[]`),
	})
	m := testModel(t)

	msg := m.fetchTraktCatalogCmd()()

	loaded, ok := msg.(TraktCatalogLoadedMsg)
	require.True(t, ok, "got %T", msg)
	assert.Len(t, loaded.Movies, 1)
}

func TestFetchLibrary_ReportsTheTabThatFailed(t *testing.T) {
	stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/usenet/mylist": respond(400, `{"success":false,"detail":"nope"}`),
	})
	m := testModel(t)

	msg := m.fetchLibraryCmd(TabUsenet, false)()

	failed, ok := msg.(LibraryFetchFailedMsg)
	require.True(t, ok, "got %T", msg)
	assert.Equal(t, TabUsenet, failed.Tab)
	assert.Error(t, failed.Err)
}

func TestFetchLibrary_PassesTheBypassFlagThrough(t *testing.T) {
	rec := stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/torrents/mylist": respond(200, `{"success":true,"data":[]}`),
	})
	m := testModel(t)

	m.fetchLibraryCmd(TabTorrents, false)()
	m.fetchLibraryCmd(TabTorrents, true)()

	urls := rec.all()
	require.Len(t, urls, 2)
	assert.NotContains(t, urls[0], "bypass_cache", "a reconcile uses TorBox's server-side cache")
	assert.Contains(t, urls[1], "bypass_cache=true")
}

func TestLibraryFetchFailure_KeepsTheExistingErrorText(t *testing.T) {
	m := testModel(t)

	next, _ := m.Update(LibraryFetchFailedMsg{Tab: TabUsenet, Err: errors.New("boom")})

	got := next.(AppModel)
	assert.True(t, got.isStatusErr)
	assert.Equal(t, "Failed to load usenet: boom", got.statusText)
}

func TestTraktCatalogFailure_KeepsTheCatalogAlreadyLoaded(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(TraktCatalogLoadedMsg{Playback: []trakt.PlaybackItem{{
		ID: 1, Progress: 41, Type: "movie",
		Movie: &trakt.Movie{Title: "Test Feature Alpha", Year: 2023},
	}}})

	next, _ = next.(AppModel).Update(TraktCatalogFailedMsg{Err: errors.New("offline")})

	percent, _ := next.(AppModel).resumeFor(matcher.ParseMedia("Test.Feature.Alpha.2023.1080p.mkv"))
	assert.Equal(t, 41.0, percent)
}
