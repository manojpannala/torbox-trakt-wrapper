package tui

import (
	"errors"
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
	return cache.New(dir, config.Version, nil), dir
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

	msg := m.fetchLibraryCmd(TabTorrents, false, m.libGen[TabTorrents])()
	_, cmd := m.Update(msg)
	require.NotNil(t, cmd, "an accepted load must return the cache-write command")
	cmd()

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

	msg := m.fetchLibraryCmd(TabTorrents, false, m.libGen[TabTorrents])()
	_, cmd := m.Update(msg)
	require.NotNil(t, cmd, "an accepted load must return the cache-write command")
	cmd()

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

	msg := m.fetchLibraryCmd(TabTorrents, false, m.libGen[TabTorrents])()
	_, cmd := m.Update(msg)
	assert.Nil(t, cmd, "a failed fetch must not write the cache")

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

		msg := m.fetchTraktCatalogCmd(m.traktGen)()
		_, cmd := m.Update(msg)
		require.NotNil(t, cmd, "an accepted catalog load must return the cache-write command")
		cmd()

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

		msg := m.fetchTraktCatalogCmd(m.traktGen)()
		_, cmd := m.Update(msg)
		assert.Nil(t, cmd, "a failed fetch must not write the cache")
		if cmd != nil {
			cmd()
		}

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

func TestFirstShow_ColdTabShowsASpinnerWhileItLoads(t *testing.T) {
	usenetOK(t)
	store, _ := newTestStore(t)
	m := testModel(t, WithCache(store))
	m.loading = false

	cmd := m.showTab(TabUsenet)

	require.NotNil(t, cmd)
	assert.True(t, m.loading, "a cold tab with no data must show a spinner while its fetch runs")
}

func TestFirstShow_StaleCachedTabKeepsItsListVisibleWithNoSpinner(t *testing.T) {
	usenetOK(t)
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxUsenet, []torbox.UsenetItem{{ID: 2, Name: "y"}})
	m := testModel(t, WithCache(store))
	m.cachedAt[TabUsenet] = time.Now().Add(-time.Hour)
	m.loading = false

	cmd := m.showTab(TabUsenet)

	require.NotNil(t, cmd)
	assert.False(t, m.loading, "a stale cached tab keeps its list visible with no spinner")
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

func TestStaleTorrentsLoad_AfterRefresh_IsDropped(t *testing.T) {
	store, _ := newTestStore(t)
	m := testModel(t, WithCache(store))
	staleGen := m.libGen[TabTorrents]

	next, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'r', Text: "r"})
	got := next.(AppModel)
	require.NotEqual(t, staleGen, got.libGen[TabTorrents], "r must bump the generation for this regression to be meaningful")

	next2, cmd := got.Update(TorrentsLoadedMsg{Torrents: []torbox.Torrent{alpha}, Gen: staleGen})

	assert.Nil(t, cmd, "a stale load must not write the cache")
	assert.Empty(t, next2.(AppModel).torrents, "a stale load must not replace newer data")
	_, ok := cache.Read[[]torbox.Torrent](store, cache.TorBoxTorrents)
	assert.False(t, ok, "a stale load must leave the cache file untouched")
}

func TestStaleTraktCatalog_AfterRePairing_IsDropped(t *testing.T) {
	store, _ := newTestStore(t)
	m := testModel(t, WithCache(store))
	preGen := m.traktGen

	next, _ := m.Update(TokenPollSuccessMsg{Token: &trakt.TokenResponse{AccessToken: "new-account"}})
	got := next.(AppModel)
	require.NotEqual(t, preGen, got.traktGen, "re-pairing must bump the Trakt generation for this regression to be meaningful")

	next2, cmd := got.Update(TraktCatalogLoadedMsg{
		Movies: []trakt.WatchedMovie{{Plays: 1, Movie: trakt.Movie{Title: "Test Feature Alpha", Year: 2023}}},
		Gen:    preGen,
	})

	final := next2.(AppModel)
	assert.False(t, final.traktSettled, "a pre-pairing catalog must not settle the new account's wait")
	assert.Nil(t, cmd)
	_, ok := cache.Read[traktCatalog](store, cache.TraktCatalog)
	assert.False(t, ok, "the stale catalog must not be written under the new account")
}

func TestStaleLibraryFetchFailed_DoesNotClearANewerInFlightBit(t *testing.T) {
	m := testModel(t)
	staleGen := m.libGen[TabTorrents]

	next, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'r', Text: "r"})
	got := next.(AppModel)
	require.NotZero(t, got.inFlight&tabBit(TabTorrents), "r must mark the tab in flight for this regression to be meaningful")

	next2, _ := got.Update(LibraryFetchFailedMsg{Tab: TabTorrents, Err: errors.New("stale"), Gen: staleGen})

	assert.NotZero(t, next2.(AppModel).inFlight&tabBit(TabTorrents), "a stale failure must not clear the current fetch's in-flight bit")
}

func TestRefreshKey_WithNilTorBoxClient_DoesNotSetInFlight(t *testing.T) {
	m := testModelWith(t, func(c *config.Config) { c.TorBox.APIKey = "" })

	next, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'r', Text: "r"})

	assert.Zero(t, next.(AppModel).inFlight, "no TorBox client means nothing to mark in flight")
}

func TestStalenessHint(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

	assert.Equal(t, "", stalenessHint(time.Time{}, false, false, now), "fresh data needs no hint")
	assert.Equal(t, "Updated 14 minutes ago · refreshing…", stalenessHint(now.Add(-14*time.Minute), false, true, now))
	assert.Equal(t, "Offline — showing data from 14 minutes ago", stalenessHint(now.Add(-14*time.Minute), true, true, now))
	assert.Equal(t, "Updated just now · refreshing…", stalenessHint(now.Add(-10*time.Second), false, true, now))
	assert.Equal(t, "Updated 14 minutes ago", stalenessHint(now.Add(-14*time.Minute), false, false, now), "no fetch is in flight, so it must not claim to be refreshing")
}

func TestFooter_ATabSkippedByTTLDoesNotClaimToRefresh(t *testing.T) {
	usenetOK(t)
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxUsenet, []torbox.UsenetItem{{ID: 2, Name: "y"}})
	m := testModel(t, WithCache(store))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	m = next.(AppModel)

	cmd := m.showTab(TabUsenet)

	require.Nil(t, cmd, "the cache is fresh, so nothing fetches")
	footer := m.renderFooter()
	assert.Contains(t, footer, "Updated")
	assert.NotContains(t, footer, "refreshing")
}

func TestFooter_AColdShowThatFetchesDoesShowRefreshing(t *testing.T) {
	usenetOK(t)
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxUsenet, []torbox.UsenetItem{{ID: 2, Name: "y"}})
	m := testModel(t, WithCache(store))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	m = next.(AppModel)
	m.cachedAt[TabUsenet] = time.Now().Add(-time.Hour)

	cmd := m.showTab(TabUsenet)

	require.NotNil(t, cmd, "the cache is stale, so it must fetch")
	assert.NotZero(t, m.inFlight&tabBit(TabUsenet), "showTab must mark the tab in flight before it returns the fetch")
	footer := m.renderFooter()
	assert.Contains(t, footer, "refreshing")
}

func TestReleaseHeldLaunch_ClearsAStaleErrorFlag(t *testing.T) {
	m := testModel(t)
	m.traktSettled = true
	m.isStatusErr = true
	played := false
	m.heldLaunch = &heldLaunch{title: "Test Feature Alpha", play: func(float64) tea.Cmd {
		played = true
		return nil
	}}

	m.releaseHeldLaunch()

	require.True(t, played, "beginStream must take the immediate-play branch for this assertion to be meaningful")
	assert.False(t, m.isStatusErr, "a stale error flag must not render as '✖ Ready' and hide the staleness hint")
}

func TestEscapeCancelHeldLaunch_ClearsAStaleErrorFlag(t *testing.T) {
	m := deferModel(t, nil)
	m, _ = sendKey(m, enterKey)
	m.isStatusErr = true

	m, _ = sendKey(m, escKey)

	assert.False(t, m.isStatusErr)
	assert.Equal(t, "Ready", m.statusText)
}

func TestLibraryFetchFailedOverCache_LeavesAnUnrelatedStatusMessageAlone(t *testing.T) {
	m := wideCachedModel(t)
	m.statusText = "Checking your position…"

	next, _ := m.Update(LibraryFetchFailedMsg{Tab: TabTorrents, Err: errors.New("dial tcp: refused")})

	got := next.(AppModel)
	assert.Equal(t, "Checking your position…", got.statusText, "a background reconcile over cached data must never clobber an unrelated status message")
}

func wideCachedModel(t *testing.T) AppModel {
	t.Helper()
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxTorrents, []torbox.Torrent{alpha})
	m := testModel(t, WithCache(store))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	return next.(AppModel)
}

func TestFooter_ShowsTheHintOnlyWhileCachedDataIsOnScreen(t *testing.T) {
	m := wideCachedModel(t)
	assert.Contains(t, m.renderFooter(), "refreshing…")

	next, _ := m.Update(TorrentsLoadedMsg{Torrents: []torbox.Torrent{alpha}})

	assert.NotContains(t, next.(AppModel).renderFooter(), "refreshing…")
}

func TestFooter_SaysOfflineWhenAReconcileFailsOverCachedData(t *testing.T) {
	m := wideCachedModel(t)

	next, _ := m.Update(LibraryFetchFailedMsg{Tab: TabTorrents, Err: errors.New("dial tcp: refused")})

	footer := next.(AppModel).renderFooter()
	assert.Contains(t, footer, "Offline — showing data from")
	assert.NotContains(t, footer, "Failed to load")
}

func TestFooter_KeepsTheErrorWhenThereIsNoCachedData(t *testing.T) {
	store, _ := newTestStore(t)
	m := testModel(t, WithCache(store))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})

	next, _ = next.(AppModel).Update(LibraryFetchFailedMsg{Tab: TabTorrents, Err: errors.New("boom")})

	assert.Contains(t, next.(AppModel).renderFooter(), "Failed to load torrents")
}

func TestFooter_ListsTheSearchKey(t *testing.T) {
	m := testModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})

	footer := next.(AppModel).renderFooter()
	assert.Contains(t, footer, "[s] Search")
	assert.Contains(t, footer, "[Tab] Switch")
}

func TestFooter_DropsHintsToStayOnOneLine(t *testing.T) {
	m := wideCachedModel(t)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	footer := next.(AppModel).renderFooter()
	assert.NotContains(t, footer, "\n")
	assert.Contains(t, footer, "refreshing…")
	assert.Contains(t, footer, "[s] Search")
	assert.Contains(t, footer, "[?] Help")
	assert.NotContains(t, footer, "[Tab] Switch", "the tab row already shows how to switch")
}

func TestRefresh_ClearsItsOwnStatusWhenFreshDataLands(t *testing.T) {
	m := testModel(t)

	next, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'r', Text: "r"})
	require.Equal(t, "Refreshing library...", next.(AppModel).statusText)

	next, _ = next.(AppModel).Update(TorrentsLoadedMsg{Torrents: []torbox.Torrent{alpha}, Gen: next.(AppModel).libGen[TabTorrents]})

	assert.Equal(t, "Ready", next.(AppModel).statusText)
}

func TestFreshData_LeavesOtherStatusMessagesAlone(t *testing.T) {
	m := testModel(t)
	m.statusText = "Playback finished"

	next, _ := m.Update(TorrentsLoadedMsg{Torrents: []torbox.Torrent{alpha}})

	assert.Equal(t, "Playback finished", next.(AppModel).statusText)
}

func TestPairing_ClearsTheTraktCacheAndTheInMemoryCatalog(t *testing.T) {
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxTorrents, []torbox.Torrent{alpha})
	cache.Write(store, cache.TraktCatalog, traktCatalog{
		Playback: []trakt.PlaybackItem{playbackFor("Test Feature Alpha", 2023, 40)},
	})
	m := testModel(t, WithCache(store))
	next, _ := m.Update(TraktCatalogLoadedMsg{Playback: []trakt.PlaybackItem{playbackFor("Test Feature Alpha", 2023, 40)}})
	require.True(t, next.(AppModel).traktSettled)

	next, _ = next.(AppModel).Update(TokenPollSuccessMsg{Token: &trakt.TokenResponse{AccessToken: "new-account"}})
	got := next.(AppModel)

	_, ok := cache.Read[traktCatalog](store, cache.TraktCatalog)
	assert.False(t, ok, "the previous account's history must not survive a re-pair")
	_, ok = cache.Read[[]torbox.Torrent](store, cache.TorBoxTorrents)
	assert.True(t, ok, "pairing Trakt says nothing about the TorBox account")
	percent, _ := got.resumeFor(matcher.ParseMedia(alpha.Name))
	assert.Zero(t, percent, "the old account's positions must leave memory too")
	assert.False(t, got.traktSettled, "a launch right after pairing must wait for the new account")
}

func TestTraktRefresh_ClearsNothing(t *testing.T) {
	store, _ := newTestStore(t)
	cache.Write(store, cache.TraktCatalog, traktCatalog{
		Playback: []trakt.PlaybackItem{playbackFor("Test Feature Alpha", 2023, 40)},
	})
	m := testModel(t, WithCache(store))

	_, _ = m.Update(TraktCatalogLoadedMsg{})

	_, ok := cache.Read[traktCatalog](store, cache.TraktCatalog)
	assert.True(t, ok)
}

func TestPairing_ClearsStaleBadgesOnEveryTab(t *testing.T) {
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxUsenet, []torbox.UsenetItem{
		{ID: 1, Name: "Test.Feature.Alpha.2023.1080p.mkv", DownloadState: "completed", Progress: 1},
	})
	cache.Write(store, cache.TorBoxWebDL, []torbox.WebDLItem{
		{ID: 1, Name: "Test.Feature.Alpha.2023.1080p.mkv", DownloadState: "completed", Progress: 1},
	})
	cache.Write(store, cache.TraktCatalog, traktCatalog{
		Playback: []trakt.PlaybackItem{playbackFor("Test Feature Alpha", 2023, 40)},
	})
	m := testModel(t, WithCache(store))

	require.NotEmpty(t, m.usenet[0].TraktBadge, "the test setup must actually produce a badge to clear")
	require.NotEmpty(t, m.webdl[0].TraktBadge, "the test setup must actually produce a badge to clear")

	next, _ := m.Update(TokenPollSuccessMsg{Token: &trakt.TokenResponse{AccessToken: "new-account"}})
	got := next.(AppModel)

	for _, item := range got.usenet {
		assert.Zero(t, item.TraktProgress, "usenet badges must not survive a re-pair")
		assert.Empty(t, item.TraktBadge, "usenet badges must not survive a re-pair")
	}
	for _, item := range got.webdl {
		assert.Zero(t, item.TraktProgress, "web-DL badges must not survive a re-pair")
		assert.Empty(t, item.TraktBadge, "web-DL badges must not survive a re-pair")
	}
}
