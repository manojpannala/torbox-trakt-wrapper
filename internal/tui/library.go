package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/cache"
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
	return []tea.Cmd{m.fetchLibraryCmd(m.activeTab, false), m.fetchTraktCatalogCmd()}
}
