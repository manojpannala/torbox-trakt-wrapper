package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/cache"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type traktCatalog struct {
	Movies   []trakt.WatchedMovie `json:"movies"`
	Shows    []trakt.WatchedShow  `json:"shows"`
	Playback []trakt.PlaybackItem `json:"playback"`
}

// seedFromCache runs before the first frame, so cached data can never race
// and overwrite a fresh network response.
func (m *AppModel) seedFromCache() {
	if m.store == nil {
		return
	}
	if e, ok := cache.Read[traktCatalog](m.store, cache.TraktCatalog); ok {
		m.matcher.UpdateCatalog(e.Value.Movies, e.Value.Shows, e.Value.Playback)
	}
	if e, ok := cache.Read[[]torbox.Torrent](m.store, cache.TorBoxTorrents); ok {
		m.torrents = m.convertTorrents(e.Value)
		m.cachedAt[TabTorrents] = e.StoredAt
	}
	if e, ok := cache.Read[[]torbox.UsenetItem](m.store, cache.TorBoxUsenet); ok {
		m.usenet = m.convertUsenet(e.Value)
		m.cachedAt[TabUsenet] = e.StoredAt
	}
	if e, ok := cache.Read[[]torbox.WebDLItem](m.store, cache.TorBoxWebDL); ok {
		m.webdl = m.convertWebDL(e.Value)
		m.cachedAt[TabWebDL] = e.StoredAt
	}
	if !m.cachedAt[m.activeTab].IsZero() {
		m.loading = false
	}
	m.reapplyFilter()
}

func (m AppModel) launchCmds() []tea.Cmd {
	return []tea.Cmd{
		m.fetchLibraryCmd(m.activeTab, false, m.libGen[m.activeTab]),
		m.fetchTraktCatalogCmd(m.traktGen),
	}
}

// writeCacheCmd defers a cache write off the update loop. cache.Write is
// nil-safe on a nil store, so this can be returned unconditionally.
func writeCacheCmd[T any](store *cache.Store, key cache.Key, v T) tea.Cmd {
	return func() tea.Msg {
		cache.Write(store, key, v)
		return nil
	}
}

func tabBit(t TabType) uint8 {
	return 1 << uint(t)
}

func stalenessHint(cachedAt time.Time, failed, refreshing bool, now time.Time) string {
	if cachedAt.IsZero() {
		return ""
	}
	when := humanizeSince(cachedAt, now)
	if failed {
		return "Offline — showing data from " + when
	}
	if refreshing {
		return "Updated " + when + " · refreshing…"
	}
	return "Updated " + when
}

const refreshingStatus = "Refreshing library..."

func (m *AppModel) markFresh(tab TabType) {
	m.cachedAt[tab] = time.Time{}
	m.fetchFailed &^= tabBit(tab)
	m.inFlight &^= tabBit(tab)
	if m.statusText == refreshingStatus {
		m.statusText = "Ready"
	}
}

func (m *AppModel) showTab(tab TabType) tea.Cmd {
	m.activeTab = tab
	m.cursor = 0
	m.topIndex = 0
	m.reapplyFilter()

	if m.reconciledTabs&tabBit(tab) != 0 {
		return nil
	}
	m.reconciledTabs |= tabBit(tab)

	cachedAt := m.cachedAt[tab]
	ttl := time.Duration(m.cfg.TorBox.CacheTTLMinutes) * time.Minute
	if !cachedAt.IsZero() && ttl > 0 && time.Since(cachedAt) < ttl {
		return nil
	}
	if cachedAt.IsZero() {
		m.loading = true
	}
	return m.refetchLibrary(tab, false)
}

// refetchLibrary is the single place that issues a library fetch: it bumps
// the tab's generation so a result from an earlier fetch is dropped on
// arrival, and marks the tab in flight only when there is a client to answer.
func (m *AppModel) refetchLibrary(tab TabType, bypass bool) tea.Cmd {
	m.libGen[tab]++
	if m.torboxClient != nil {
		m.inFlight |= tabBit(tab)
	}
	return m.fetchLibraryCmd(tab, bypass, m.libGen[tab])
}

// refetchTrakt bumps the Trakt catalog generation so a result fetched for a
// previous account (or before a re-pair) is dropped on arrival.
func (m *AppModel) refetchTrakt() tea.Cmd {
	m.traktGen++
	return m.fetchTraktCatalogCmd(m.traktGen)
}

type heldLaunch struct {
	title  string
	parsed matcher.ParsedMedia
	play   func(float64) tea.Cmd
}

func (m AppModel) awaitingTrakt() bool {
	return !m.traktSettled && m.traktClient != nil && m.cfg.Trakt.HasAuth()
}

func (m *AppModel) releaseHeldLaunch() tea.Cmd {
	h := m.heldLaunch
	m.heldLaunch = nil
	if h == nil {
		return nil
	}
	m.statusText = "Ready"
	m.isStatusErr = false
	return m.beginStream(h.title, h.parsed, h.play)
}
