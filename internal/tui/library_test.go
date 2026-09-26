package tui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/cache"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

var (
	alpha = torbox.Torrent{ID: 1, Name: "Test.Feature.Alpha.2023.1080p.BluRay.x264.mkv", DownloadState: "completed", Progress: 1}
	beta  = torbox.Torrent{ID: 2, Name: "Test.Feature.Beta.2022.2160p.WEB-DL.mkv", DownloadState: "completed", Progress: 1}
)

func newTestStore(t *testing.T) (*cache.Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	return cache.New(dir, 15*time.Minute, config.Version, nil), dir
}

func playbackFor(title string, year int, pct float64) trakt.PlaybackItem {
	return trakt.PlaybackItem{
		ID: int64(pct * 10), Progress: pct, Type: "movie",
		Movie: &trakt.Movie{Title: title, Year: year},
	}
}

func traktOK(t *testing.T) *recorder {
	t.Helper()
	return stubAPI(t, "TRAKT_BASE_URL", map[string]func(http.ResponseWriter){
		"/sync/watched/movies": respond(200, `[]`),
		"/sync/watched/shows":  respond(200, `[]`),
		"/sync/playback":       respond(200, `[]`),
	})
}

func TestWarmLaunch_RendersEveryTabFromDiskBeforeAnyNetworkCall(t *testing.T) {
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxTorrents, []torbox.Torrent{alpha})
	cache.Write(store, cache.TorBoxUsenet, []torbox.UsenetItem{{ID: 2, Name: "Test.Series.Gamma.S01E01.mkv"}})
	cache.Write(store, cache.TorBoxWebDL, []torbox.WebDLItem{{ID: 3, Name: "Test.Feature.Beta.2022.mkv"}})
	cache.Write(store, cache.TraktCatalog, traktCatalog{
		Playback: []trakt.PlaybackItem{playbackFor("Test Feature Alpha", 2023, 41)},
	})

	m := testModel(t, WithCache(store))

	assert.Len(t, m.torrents, 1)
	assert.Len(t, m.usenet, 1)
	assert.Len(t, m.webdl, 1)
	assert.False(t, m.loading, "the active tab has data, so no spinner")
	for _, tab := range []TabType{TabTorrents, TabUsenet, TabWebDL} {
		assert.False(t, m.cachedAt[tab].IsZero(), "tab %v must know it is showing cached data", tab)
	}
	percent, _ := m.resumeFor(matcher.ParseMedia(alpha.Name))
	assert.Equal(t, 41.0, percent, "the cached catalog must be in the matcher before the first frame")
}

func TestColdLaunch_BehavesAsBefore(t *testing.T) {
	store, _ := newTestStore(t)

	m := testModel(t, WithCache(store))

	assert.Empty(t, m.torrents)
	assert.True(t, m.loading)
	assert.True(t, m.cachedAt[TabTorrents].IsZero())
}

func TestNoCache_BehavesAsBefore(t *testing.T) {
	m := testModel(t)

	assert.Nil(t, m.store)
	assert.True(t, m.loading)
}

func TestLaunch_ReconcilesTheActiveTabThroughTorBoxsCache(t *testing.T) {
	rec := stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/torrents/mylist": respond(200, `{"success":true,"data":[]}`),
	})
	traktOK(t)
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxTorrents, []torbox.Torrent{alpha})
	m := testModel(t, WithCache(store))

	for _, cmd := range m.launchCmds() {
		cmd()
	}

	urls := rec.all()
	require.Len(t, urls, 1, "the active tab reconciles even when its cache is fresh")
	assert.NotContains(t, urls[0], "bypass_cache")
}

func TestSuccessfulFetch_WritesTheCache(t *testing.T) {
	stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/torrents/mylist": respond(200, `{"success":true,"data":[{"id":7,"name":"Test.Feature.Alpha.2023.1080p.mkv"}]}`),
	})
	store, _ := newTestStore(t)
	m := testModel(t, WithCache(store))

	m.fetchLibraryCmd(TabTorrents, false)()

	e, ok := cache.Read[[]torbox.Torrent](store, cache.TorBoxTorrents)
	require.True(t, ok)
	require.Len(t, e.Value, 1)
	assert.Equal(t, 7, e.Value[0].ID)
}

func TestSuccessfulEmptyFetch_OverwritesTheCache(t *testing.T) {
	stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/torrents/mylist": respond(200, `{"success":true,"data":[]}`),
	})
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxTorrents, []torbox.Torrent{alpha})
	m := testModel(t, WithCache(store))

	m.fetchLibraryCmd(TabTorrents, false)()

	e, ok := cache.Read[[]torbox.Torrent](store, cache.TorBoxTorrents)
	require.True(t, ok)
	assert.Empty(t, e.Value, "a deleted torrent must not come back on the next launch")
}

func TestFailedFetch_LeavesTheCacheAlone(t *testing.T) {
	stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/torrents/mylist": respond(400, `{"success":false,"detail":"nope"}`),
	})
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxTorrents, []torbox.Torrent{alpha})
	m := testModel(t, WithCache(store))

	m.fetchLibraryCmd(TabTorrents, false)()

	e, ok := cache.Read[[]torbox.Torrent](store, cache.TorBoxTorrents)
	require.True(t, ok)
	assert.Len(t, e.Value, 1)
}

func TestTraktFetch_WritesTheCacheOnlyOnSuccess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stubAPI(t, "TRAKT_BASE_URL", map[string]func(http.ResponseWriter){
			"/sync/watched/movies": respond(200, `[]`),
			"/sync/watched/shows":  respond(200, `[]`),
			"/sync/playback":       respond(200, `[{"id":1,"progress":55,"type":"movie","movie":{"title":"Test Feature Alpha","year":2023}}]`),
		})
		store, _ := newTestStore(t)
		m := testModel(t, WithCache(store))

		m.fetchTraktCatalogCmd()()

		e, ok := cache.Read[traktCatalog](store, cache.TraktCatalog)
		require.True(t, ok)
		require.Len(t, e.Value.Playback, 1)
		assert.Equal(t, 55.0, e.Value.Playback[0].Progress)
	})

	t.Run("failure", func(t *testing.T) {
		stubAPI(t, "TRAKT_BASE_URL", map[string]func(http.ResponseWriter){
			"/sync/watched/movies": respond(500, `{}`),
		})
		store, _ := newTestStore(t)
		cache.Write(store, cache.TraktCatalog, traktCatalog{
			Playback: []trakt.PlaybackItem{playbackFor("Test Feature Alpha", 2023, 41)},
		})
		m := testModel(t, WithCache(store))

		m.fetchTraktCatalogCmd()()

		e, ok := cache.Read[traktCatalog](store, cache.TraktCatalog)
		require.True(t, ok)
		assert.Len(t, e.Value.Playback, 1, "a failed fetch must not wipe cached positions")
	})
}

func TestFreshLoad_ClearsTheCachedMarker(t *testing.T) {
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxTorrents, []torbox.Torrent{alpha})
	m := testModel(t, WithCache(store))
	require.False(t, m.cachedAt[TabTorrents].IsZero())

	next, _ := m.Update(TorrentsLoadedMsg{Torrents: []torbox.Torrent{alpha}})

	assert.True(t, next.(AppModel).cachedAt[TabTorrents].IsZero())
}

func TestCachedPayloadsCarryNoCredentials(t *testing.T) {
	store, dir := newTestStore(t)
	cache.Write(store, cache.TorBoxTorrents, []torbox.Torrent{{
		ID: 1, Name: "x", Hash: "abc",
		Files: []torbox.TorrentFile{{ID: 1, Name: "f.mkv", S3Path: "p", AbsolutePath: "/a"}},
	}, beta})
	cache.Write(store, cache.TorBoxUsenet, []torbox.UsenetItem{{ID: 2, Name: "y"}})
	cache.Write(store, cache.TorBoxWebDL, []torbox.WebDLItem{{ID: 3, Name: "z"}})
	cache.Write(store, cache.TraktCatalog, traktCatalog{
		Movies: []trakt.WatchedMovie{{Plays: 1, Movie: trakt.Movie{
			Title: "t", Year: 2023, IDs: trakt.IDs{Trakt: 1, Slug: "s", IMDB: "tt1"},
		}}},
		Playback: []trakt.PlaybackItem{playbackFor("t", 2023, 10)},
	})

	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 4)
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dir, f.Name())) // #nosec G304
		require.NoError(t, err)
		lower := strings.ToLower(string(raw))
		for _, secret := range []string{"access_token", "refresh_token", "api_key", "apikey", "client_secret", "authorization", "bearer", `"token"`} {
			assert.NotContains(t, lower, secret, "%s leaked into %s", secret, f.Name())
		}
	}
}

func usenetOK(t *testing.T) *recorder {
	t.Helper()
	return stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/usenet/mylist":   respond(200, `{"success":true,"data":[]}`),
		"/torrents/mylist": respond(200, `{"success":true,"data":[]}`),
	})
}

func TestFirstShow_FetchesATabWithNoData(t *testing.T) {
	rec := usenetOK(t)
	store, _ := newTestStore(t)
	m := testModel(t, WithCache(store))

	cmd := m.showTab(TabUsenet)

	require.NotNil(t, cmd, "Usenet used to stay empty until r")
	cmd()
	require.Len(t, rec.all(), 1)
	assert.NotContains(t, rec.all()[0], "bypass_cache")
}

func TestFirstShow_SkipsAFreshCache(t *testing.T) {
	usenetOK(t)
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxUsenet, []torbox.UsenetItem{{ID: 2, Name: "y"}})
	m := testModel(t, WithCache(store))

	assert.Nil(t, m.showTab(TabUsenet))
}

func TestFirstShow_FetchesAStaleCache(t *testing.T) {
	usenetOK(t)
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxUsenet, []torbox.UsenetItem{{ID: 2, Name: "y"}})
	m := testModel(t, WithCache(store))
	m.cachedAt[TabUsenet] = time.Now().Add(-time.Hour)

	assert.NotNil(t, m.showTab(TabUsenet))
}

func TestFirstShow_ZeroTTLAlwaysFetches(t *testing.T) {
	usenetOK(t)
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxUsenet, []torbox.UsenetItem{{ID: 2, Name: "y"}})
	m := testModelWith(t, func(c *config.Config) { c.TorBox.CacheTTLMinutes = 0 }, WithCache(store))

	assert.NotNil(t, m.showTab(TabUsenet))
}

func TestSecondShow_NeverFetches(t *testing.T) {
	usenetOK(t)
	store, _ := newTestStore(t)
	m := testModel(t, WithCache(store))

	require.NotNil(t, m.showTab(TabUsenet))
	m.showTab(TabTorrents)

	assert.Nil(t, m.showTab(TabUsenet))
}

func TestActiveTab_IsNotRefetchedOnFirstShow(t *testing.T) {
	store, _ := newTestStore(t)
	m := testModel(t, WithCache(store))

	assert.Nil(t, m.showTab(TabTorrents), "launch already reconciled it")
}

func TestNumberKey_ShowsTheTabAndReconcilesIt(t *testing.T) {
	usenetOK(t)
	store, _ := newTestStore(t)
	m := testModel(t, WithCache(store))

	next, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: '2', Text: "2"})

	assert.Equal(t, TabUsenet, next.(AppModel).activeTab)
	assert.NotNil(t, cmd)
}

func TestRefreshKey_BypassesTorBoxsCache(t *testing.T) {
	rec := usenetOK(t)
	traktOK(t)
	m := testModel(t)

	_, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: 'r', Text: "r"})
	require.NotNil(t, cmd)
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				c()
			}
		}
	}

	var torrentURLs []string
	for _, u := range rec.all() {
		if strings.HasPrefix(u, "/torrents/") {
			torrentURLs = append(torrentURLs, u)
		}
	}
	require.Len(t, torrentURLs, 1)
	assert.Contains(t, torrentURLs[0], "bypass_cache=true")
}
