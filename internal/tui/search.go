package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search/prowlarr"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

// titleSearcher is the part of the Trakt client search uses.
type titleSearcher interface {
	SearchTitles(ctx context.Context, text string) ([]trakt.TitleHit, error)
	LookupIMDb(ctx context.Context, imdbID string) (*trakt.TitleHit, error)
}

// cachedChecker is the part of the TorBox client the badge checker uses.
type cachedChecker interface {
	CheckCached(ctx context.Context, hashes []string) (map[string]bool, error)
}

type searchStage int

const (
	stageQuery searchStage = iota
	stageTitles
	stageEpisode
	stageReleases
)

const (
	titleTimeout = 15 * time.Second
	badgeTimeout = 15 * time.Second
	settleWait   = 5 * time.Second
)

// indexerRun is one indexer's part in a search.
type indexerRun struct {
	id      int
	name    string
	pending int
	count   int
	err     error
}

// searchView is one search, from the typed query to the release list. Every
// reply carries the seq it was asked under; a reply to an older search is
// dropped.
type searchView struct {
	stage         searchStage
	input         textinput.Model
	filter        textinput.Model
	filtering     bool
	seq           uint64
	ctx           context.Context
	cancel        context.CancelFunc
	hits          []trakt.TitleHit
	work          search.Work
	raw           bool
	runs          []indexerRun
	pool          []search.Result
	drawn         bool
	sections      []search.Section
	prefs         search.Prefs
	tally         search.Tally
	settleArmed   bool
	loggedSummary bool
	cachedOnly    bool
	resolver      *search.Resolver
	resolving     map[string]bool
	addAfter      string
	sortKey       search.SortKey
	cursor        int
	top           int
	status        string
	isErr         bool
}

type searchTitlesMsg struct {
	Seq  uint64
	Hits []trakt.TitleHit
	Err  error
}

type searchWorkMsg struct {
	Seq  uint64
	Work search.Work
	Note string
}

type searchIndexersMsg struct {
	Seq      uint64
	Indexers []search.Indexer
	Err      error
}

type searchReleasesMsg struct {
	Seq  uint64
	Req  search.Request
	Rows []search.Result
	Err  error
}

type searchResolvedMsg struct {
	Seq  uint64
	Row  search.Result
	Hash string
	Err  error
}

type searchAddedMsg struct {
	Title     string
	TorrentID int
	Err       error
}

type searchSettleMsg struct {
	Seq uint64
}

type badgeTickMsg struct{}

type badgeCheckedMsg struct {
	Batch  []string
	Cached map[string]bool
	Err    error
}

// newSearcher builds the Prowlarr client, or says why search is off. Bad
// settings or a bad URL only turn search off; the rest of the app still starts.
func newSearcher(cfg *config.Config, logger *slog.Logger) (search.Searcher, map[search.Group]int, string) {
	caps, err := search.Caps(cfg.Search.Resolutions)
	if err != nil {
		return nil, nil, "Search is off: " + err.Error()
	}
	if !cfg.Search.Enabled() {
		return nil, caps, "Search is off: set prowlarr_url and prowlarr_api_key under [search] in " + cfg.Path()
	}
	client, err := prowlarr.New(cfg.Search.ProwlarrURL, cfg.Search.ProwlarrAPIKey, prowlarr.WithLogger(logger))
	if err != nil {
		return nil, caps, "Search is off: " + prowlarr.Describe(err)
	}
	return client, caps, ""
}

func newSearchInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 128
	ti.SetWidth(50)
	return ti
}

const queryPlaceholder = "Title, or an IMDb ID like tt0000000 S01E02"

// openSearch shows the search box, prefilled with text.
func (m *AppModel) openSearch(text string) tea.Cmd {
	if m.searcher == nil {
		m.statusText, m.isStatusErr = m.searchOff, true
		return nil
	}
	m.stopSearch()
	m.activeView = ViewSearch
	m.sv.input = newSearchInput(queryPlaceholder)
	m.sv.input.SetValue(text)
	m.sv.input.Focus()
	return textinput.Blink
}

// searchSelected searches releases for the library row under the cursor,
// by the IMDb ID its Trakt match carries.
func (m *AppModel) searchSelected() tea.Cmd {
	item := m.selectedCurrentItem()
	if item == nil {
		return nil
	}
	if m.searcher == nil {
		m.statusText, m.isStatusErr = m.searchOff, true
		return nil
	}
	if item.IMDbID == "" {
		m.statusText, m.isStatusErr = "No Trakt match for this row, so there's no IMDb ID to search by. Press s to search by title.", true
		return nil
	}
	query := item.IMDbID
	if item.Parsed.Season > 0 {
		query += fmt.Sprintf(" S%02d", item.Parsed.Season)
		if item.Parsed.Episode > 0 {
			query += fmt.Sprintf("E%02d", item.Parsed.Episode)
		}
	}
	m.openSearch(query)
	return m.submitQuery(false)
}

// stopSearch cancels everything the current search has in flight and
// drops its hashes that are still waiting for a badge check.
func (m *AppModel) stopSearch() {
	if m.sv.cancel != nil {
		m.sv.cancel()
	}
	m.checker.Clear()
	m.sv = searchView{seq: m.sv.seq + 1, sortKey: m.sv.sortKey, input: m.sv.input}
}

func (m *AppModel) closeSearch() {
	m.stopSearch()
	m.activeView = ViewLibrary
}

// submitQuery starts a search for the text in the box: raw sends it to
// Prowlarr as typed, an IMDb ID goes straight to releases, and anything
// else asks Trakt for titles first.
func (m *AppModel) submitQuery(raw bool) tea.Cmd {
	text := strings.TrimSpace(m.sv.input.Value())
	if text == "" {
		return nil
	}
	input := m.sv.input
	m.stopSearch()
	m.sv.input = input
	m.sv.input.Blur()
	m.sv.ctx, m.sv.cancel = context.WithCancel(m.ctx)
	m.sv.resolver = search.NewResolver()
	m.sv.resolving = map[string]bool{}
	m.sv.filter = newSearchInput("Filter releases...")
	seq, parent, titles := m.sv.seq, m.sv.ctx, m.titles

	if raw {
		m.sv.raw = true
		m.sv.stage = stageReleases
		return m.fetchIndexers()
	}
	q := search.ParseQuery(text)
	if q.IMDbID != "" {
		m.sv.stage = stageReleases
		m.sv.status = "Looking up " + q.IMDbID + " on Trakt…"
		return func() tea.Msg { return lookupWork(parent, titles, q, seq) }
	}
	if titles == nil {
		m.sv.input.Focus()
		m.sv.status, m.sv.isErr = "Title search needs Trakt (client_id in the config). Type an IMDb ID, or press ctrl+r to search the text as typed.", true
		return nil
	}
	m.sv.stage = stageTitles
	m.sv.status = "Searching Trakt…"
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, titleTimeout)
		defer cancel()
		hits, err := titles.SearchTitles(ctx, q.Text)
		return searchTitlesMsg{Seq: seq, Hits: hits, Err: err}
	}
}

// lookupWork asks Trakt for the title behind an IMDb ID, which the text
// searches need. Without one, only the ID searches run.
func lookupWork(ctx context.Context, titles titleSearcher, q search.Query, seq uint64) tea.Msg {
	if titles == nil {
		return searchWorkMsg{Seq: seq, Work: q.Work("", "", 0), Note: "No Trakt client, so searching by ID only"}
	}
	ctx, cancel := context.WithTimeout(ctx, titleTimeout)
	defer cancel()
	hit, err := titles.LookupIMDb(ctx, q.IMDbID)
	switch {
	case err != nil:
		return searchWorkMsg{Seq: seq, Work: q.Work("", "", 0), Note: "Trakt lookup failed, so searching by ID only"}
	case hit == nil:
		return searchWorkMsg{Seq: seq, Work: q.Work("", "", 0), Note: "Trakt has no title for " + q.IMDbID + ", so searching by ID only"}
	}
	w := q.Work(hit.Kind, hit.Title, hit.Year)
	w.Language = hit.Language
	return searchWorkMsg{Seq: seq, Work: w}
}

func (m *AppModel) fetchIndexers() tea.Cmd {
	seq, ctx, backend := m.sv.seq, m.sv.ctx, m.searcher
	return func() tea.Msg {
		indexers, err := backend.Indexers(ctx)
		return searchIndexersMsg{Seq: seq, Indexers: indexers, Err: err}
	}
}

// pickWork moves on from the titles step: a movie searches now, a show asks
// which season or episode first.
func (m *AppModel) pickWork(hit trakt.TitleHit) tea.Cmd {
	q := search.Query{IMDbID: hit.IDs.IMDB}
	m.sv.work = q.Work(hit.Kind, hit.Title, hit.Year)
	m.sv.work.Language = hit.Language
	if m.sv.work.Kind == search.Show {
		m.sv.stage = stageEpisode
		m.sv.input = newSearchInput("S02, S02E05, or blank for the whole show")
		m.sv.input.Focus()
		m.sv.status, m.sv.isErr = "", false
		return textinput.Blink
	}
	return m.startReleases("")
}

func (m *AppModel) startReleases(note string) tea.Cmd {
	m.sv.stage = stageReleases
	m.sv.status, m.sv.isErr = note, false
	return m.fetchIndexers()
}

// handleSearchMsg handles the search view's replies; ok is false for any
// other message.
func (m AppModel) handleSearchMsg(msg tea.Msg) (AppModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case searchTitlesMsg:
		if msg.Seq != m.sv.seq {
			return m, nil, true
		}
		switch {
		case msg.Err != nil:
			m.sv.stage = stageQuery
			m.sv.input.Focus()
			m.sv.status, m.sv.isErr = "Trakt search failed. Type an IMDb ID, or press ctrl+r to search the text as typed.", true
		case len(msg.Hits) == 0:
			m.sv.stage = stageQuery
			m.sv.input.Focus()
			m.sv.status, m.sv.isErr = "Trakt found no titles. Press ctrl+r to search the text as typed.", true
		default:
			m.sv.hits = msg.Hits
			m.sv.cursor, m.sv.top = 0, 0
			m.sv.status, m.sv.isErr = "", false
		}
		return m, nil, true

	case searchWorkMsg:
		if msg.Seq != m.sv.seq {
			return m, nil, true
		}
		m.sv.work = msg.Work
		return m, m.startReleases(msg.Note), true

	case searchIndexersMsg:
		if msg.Seq != m.sv.seq {
			return m, nil, true
		}
		return m, m.planSearch(msg), true

	case searchReleasesMsg:
		if msg.Seq != m.sv.seq {
			return m, nil, true
		}
		return m, m.takeReleases(msg), true

	case searchSettleMsg:
		if msg.Seq != m.sv.seq {
			return m, nil, true
		}
		return m, m.onSettle(), true

	case searchResolvedMsg:
		if msg.Seq != m.sv.seq {
			return m, nil, true
		}
		return m, m.takeResolved(msg), true

	case searchAddedMsg:
		return m, m.takeAdded(msg), true

	case badgeTickMsg:
		m.badgeTicking = false
		return m, m.pumpBadges(), true

	case badgeCheckedMsg:
		keep := m.selectedKey()
		now := m.clock()
		switch {
		case msg.Err == nil:
			m.checker.Done(msg.Batch, msg.Cached, now)
			m.badgeNote = ""
		case torbox.IsRateLimited(msg.Err):
			m.checker.Limited(msg.Batch, torbox.RetryAfter(msg.Err), now)
			m.badgeNote = fmt.Sprintf("TorBox is rate-limiting badge checks, retrying in %s", m.checker.RetryingIn(now).Round(time.Second))
		default:
			m.checker.Failed()
			m.badgeNote = "Badge check failed, so some badges show ?"
		}
		if m.sv.cachedOnly {
			m.syncSearchCursor(keep)
		}
		return m, m.pumpBadges(), true
	}
	return m, nil, false
}

func (m *AppModel) planSearch(msg searchIndexersMsg) tea.Cmd {
	if msg.Err != nil {
		m.sv.status, m.sv.isErr = prowlarr.Describe(msg.Err), true
		return nil
	}
	var reqs []search.Request
	if m.sv.raw {
		reqs = search.PlanRaw(msg.Indexers, m.sv.input.Value())
	} else {
		reqs = search.Plan(msg.Indexers, m.sv.work)
	}
	if len(reqs) == 0 {
		m.sv.status, m.sv.isErr = "None of Prowlarr's enabled torrent indexers can run this search", true
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(reqs))
	for _, req := range reqs {
		if i := m.sv.run(req.IndexerID); i >= 0 {
			m.sv.runs[i].pending++
		} else {
			m.sv.runs = append(m.sv.runs, indexerRun{id: req.IndexerID, name: req.Indexer, pending: 1})
		}
		seq, ctx, backend := m.sv.seq, m.sv.ctx, m.searcher
		cmds = append(cmds, func() tea.Msg {
			rows, err := backend.Search(ctx, req)
			return searchReleasesMsg{Seq: seq, Req: req, Rows: rows, Err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (s searchView) run(id int) int {
	for i, r := range s.runs {
		if r.id == id {
			return i
		}
	}
	return -1
}

func (m *AppModel) takeReleases(msg searchReleasesMsg) tea.Cmd {
	i := m.sv.run(msg.Req.IndexerID)
	if i < 0 {
		return nil
	}
	run := &m.sv.runs[i]
	run.pending--
	if msg.Err != nil {
		run.err = msg.Err
		if errors.Is(msg.Err, search.ErrUnauthorized) {
			m.sv.status, m.sv.isErr = prowlarr.Describe(msg.Err), true
		}
		return m.afterIndexerReply(nil)
	}
	rows, droppedTitle := search.Prepare(msg.Req, m.sv.work, msg.Rows)
	for j := range rows {
		if hash, ok := m.resolved[rows[j].Key()]; ok && rows[j].Hash == "" {
			rows[j].Hash = hash
		}
	}
	m.sv.tally.Received += len(msg.Rows)
	if droppedTitle > 0 {
		m.addDrop(search.DropTitle, droppedTitle)
	}
	run.count += len(rows)
	return m.afterIndexerReply(rows)
}

func (m *AppModel) afterIndexerReply(rows []search.Result) tea.Cmd {
	if m.sv.drawn {
		var cmd tea.Cmd
		if len(rows) > 0 {
			cmd = m.mergeLate(rows)
		}
		m.maybeLogSummary()
		m.checkEmpty()
		return cmd
	}
	if len(rows) > 0 {
		m.sv.pool = search.Merge(append(m.sv.pool, rows...))
	}
	if m.allAnswered() {
		cmd := m.drawReleases()
		m.maybeLogSummary()
		m.checkEmpty()
		return cmd
	}
	// The wait starts with the first rows, so a fast failure can't draw an
	// empty list and push every real result under Late results.
	if !m.sv.settleArmed && len(m.sv.pool) > 0 {
		m.sv.settleArmed = true
		seq := m.sv.seq
		return m.tick(settleWait, func(time.Time) tea.Msg {
			return searchSettleMsg{Seq: seq}
		})
	}
	return nil
}

func (m *AppModel) onSettle() tea.Cmd {
	if m.sv.drawn {
		return nil
	}
	return m.drawReleases()
}

func (m *AppModel) drawReleases() tea.Cmd {
	m.sv.drawn = true
	m.sv.prefs = m.searchPrefs()
	m.sv.tally.Merged = len(m.sv.pool)
	lib := m.libraryHashes()
	kept, dropped := search.Drop(m.sv.pool, func(hash string) bool {
		_, ok := lib[hash]
		return ok
	})
	m.sv.pool = nil
	for reason, n := range dropped {
		m.addDrop(reason, n)
	}
	m.sv.sections = search.Arrange(kept, m.caps(), false, m.sv.prefs)
	m.queueBadges(m.shownRows())
	return tea.Batch(m.pumpResolves(false), m.pumpBadges())
}

func (m *AppModel) mergeLate(rows []search.Result) tea.Cmd {
	lib := m.libraryHashes()
	kept, dropped := search.Drop(rows, func(hash string) bool {
		_, ok := lib[hash]
		return ok
	})
	for reason, n := range dropped {
		m.addDrop(reason, n)
	}
	var fresh []search.Result
	for _, r := range kept {
		if r.Hash != "" && m.addIndexersByHash(r.Hash, r.Indexers) {
			continue
		}
		fresh = append(fresh, r)
	}
	if len(fresh) == 0 {
		return nil
	}
	m.sv.prefs = m.searchPrefs()
	caps := m.caps()
	fresh = search.Merge(fresh)

	lateIdx := -1
	for i := range m.sv.sections {
		if m.sv.sections[i].Late {
			lateIdx = i
			break
		}
	}
	var newlyShown []search.Result
	if lateIdx < 0 {
		sec, ok := search.ArrangeLate(fresh, caps, m.sv.prefs)
		if !ok {
			return nil
		}
		m.sv.tally.Late += len(sec.Rows)
		m.sv.sections = append(m.sv.sections, sec)
		newlyShown = sec.Rows[:sec.Shown]
	} else {
		sec := &m.sv.sections[lateIdx]
		prevShown := min(sec.Shown, len(sec.Rows))
		keptShown := slices.Clone(sec.Rows[:prevShown])
		pool := search.Merge(append(slices.Clone(sec.Rows[prevShown:]), fresh...))
		rest, ok := search.ArrangeLate(pool, caps, m.sv.prefs)
		if !ok {
			return nil
		}
		for _, r := range fresh {
			if caps[search.GroupOf(r.Media)] > 0 {
				m.sv.tally.Late++
			}
		}
		newCap := max(prevShown, min(search.LateCap, prevShown+len(rest.Rows)))
		newlyShown = rest.Rows[:newCap-prevShown]
		sec.Rows = append(keptShown, rest.Rows...)
		sec.Shown = newCap
	}
	m.queueBadges(newlyShown)
	return tea.Batch(m.pumpResolves(false), m.pumpBadges())
}

func (m *AppModel) checkEmpty() {
	if len(m.shownRows()) == 0 && !m.sv.isErr {
		m.sv.status, m.sv.isErr = "No releases found. Try ctrl+r with different words.", true
	}
}

func (m *AppModel) maybeLogSummary() {
	if m.sv.loggedSummary || !m.allAnswered() || m.logger == nil {
		return
	}
	m.sv.loggedSummary = true
	m.logger.Debug("search summary", m.sv.tally.Attrs(m.sv.sections)...)
}

func (m AppModel) answeredCount() int {
	n := 0
	for _, r := range m.sv.runs {
		if r.pending == 0 {
			n++
		}
	}
	return n
}

func (m AppModel) allAnswered() bool {
	return len(m.sv.runs) > 0 && m.answeredCount() == len(m.sv.runs)
}

func (m *AppModel) addDrop(reason search.DropReason, n int) {
	if n <= 0 {
		return
	}
	if m.sv.tally.Dropped == nil {
		m.sv.tally.Dropped = map[search.DropReason]int{}
	}
	m.sv.tally.Dropped[reason] += n
}

func (m AppModel) searchPrefs() search.Prefs {
	var groups []string
	if m.cfg != nil {
		groups = m.cfg.Search.PreferredGroups
	}
	if m.sv.raw {
		return search.Prefs{Groups: groups}
	}
	return search.Prefs{
		Language: m.sv.work.Language,
		Episode:  m.sv.work.Kind == search.Show && m.sv.work.Episode > 0,
		Groups:   groups,
	}
}

func (m AppModel) caps() map[search.Group]int {
	if m.searchCaps != nil {
		return m.searchCaps
	}
	return search.DefaultCaps()
}

func (m *AppModel) addIndexersByHash(hash string, indexers []string) bool {
	for si := range m.sv.sections {
		for ri := range m.sv.sections[si].Rows {
			if m.sv.sections[si].Rows[ri].Hash == hash {
				appendMissingIndexers(&m.sv.sections[si].Rows[ri].Indexers, indexers)
				return true
			}
		}
	}
	return false
}

func appendMissingIndexers(dst *[]string, add []string) {
	for _, name := range add {
		if !slices.Contains(*dst, name) {
			*dst = append(*dst, name)
		}
	}
}

// pumpResolves starts what the resolver allows; the row under the cursor
// may go past the up-front ten, and force takes it past the budget.
func (m *AppModel) pumpResolves(force bool) tea.Cmd {
	if m.sv.resolver == nil {
		return nil
	}
	want := ""
	if row := m.selectedRelease(); row != nil {
		want = row.Key()
	}
	var cmds []tea.Cmd
	seq, ctx := m.sv.seq, m.sv.ctx
	for _, row := range m.sv.resolver.Next(m.shownRows(), want, force) {
		m.sv.resolving[row.Key()] = true
		cmds = append(cmds, func() tea.Msg {
			hash, err := row.Resolve(ctx)
			return searchResolvedMsg{Seq: seq, Row: row, Hash: hash, Err: err}
		})
	}
	return tea.Batch(cmds...)
}

func (m *AppModel) takeResolved(msg searchResolvedMsg) tea.Cmd {
	key, keep := msg.Row.Key(), m.selectedKey()
	m.sv.resolver.Done(msg.Row, msg.Err)
	delete(m.sv.resolving, key)
	var addCmd tea.Cmd
	if msg.Err != nil || msg.Hash == "" {
		dropped := 0
		for i := range m.sv.sections {
			sec := &m.sv.sections[i]
			if sec.Remove(key) {
				dropped++
			}
			for j := 0; j < min(sec.Shown, len(sec.Rows)); {
				r := sec.Rows[j]
				if r.Pending() && !m.sv.resolving[r.Key()] && m.sv.resolver.Limited(r.IndexerID) {
					if sec.Remove(r.Key()) {
						dropped++
						continue
					}
				}
				j++
			}
		}
		m.pruneEmptySections()
		m.addDrop(search.DropUnresolved, dropped)
		if m.sv.addAfter == key {
			m.sv.addAfter = ""
			m.sv.status, m.sv.isErr = "Couldn't get a magnet for that release", true
		}
	} else {
		m.resolved[key] = msg.Hash
		row, found := m.mergeResolvedHash(key, msg.Hash)
		if !found {
			return tea.Batch(m.pumpResolves(false), m.pumpBadges())
		}
		m.queueBadges([]search.Result{row})
		if m.sv.addAfter == key {
			m.sv.addAfter = ""
			addCmd = m.chooseRelease(row)
		}
	}
	m.syncSearchCursor(keep)
	return tea.Batch(addCmd, m.pumpResolves(false), m.pumpBadges())
}

func (m *AppModel) mergeResolvedHash(key, hash string) (search.Result, bool) {
	var resolvedRow search.Result
	found := false
	for si := range m.sv.sections {
		for ri := range m.sv.sections[si].Rows {
			if m.sv.sections[si].Rows[ri].Key() == key {
				m.sv.sections[si].Rows[ri].Hash = hash
				resolvedRow = m.sv.sections[si].Rows[ri]
				found = true
			}
		}
	}
	if !found {
		return search.Result{}, false
	}
	type pos struct{ sec, row int }
	var first *pos
	for si := range m.sv.sections {
		sec := &m.sv.sections[si]
		for ri := 0; ri < min(sec.Shown, len(sec.Rows)); {
			if sec.Rows[ri].Hash != hash {
				ri++
				continue
			}
			if first == nil {
				first = &pos{si, ri}
				ri++
				continue
			}
			dst := &m.sv.sections[first.sec].Rows[first.row]
			cur := sec.Rows[ri]
			appendMissingIndexers(&dst.Indexers, cur.Indexers)
			if cur.Seeders > dst.Seeders {
				cur.Indexers = dst.Indexers
				*dst = cur
			}
			if cur.Key() == key || dst.Key() == key {
				resolvedRow = *dst
			}
			// By index: after the copy above, dst may carry cur's key.
			sec.Rows = slices.Delete(sec.Rows, ri, ri+1)
			sec.Shown--
		}
	}
	m.pruneEmptySections()
	return resolvedRow, true
}

func (m *AppModel) pruneEmptySections() {
	kept := m.sv.sections[:0]
	for _, sec := range m.sv.sections {
		if len(sec.Rows) > 0 {
			kept = append(kept, sec)
		}
	}
	m.sv.sections = kept
}

// queueBadges hands the checker every hash it can't answer yet. A hash in
// the library needs no check.
func (m *AppModel) queueBadges(rows []search.Result) {
	if m.cached == nil {
		return
	}
	lib := m.libraryHashes()
	hashes := make([]string, 0, len(rows))
	for _, r := range rows {
		if _, ok := lib[r.Hash]; !ok && r.Hash != "" {
			hashes = append(hashes, r.Hash)
		}
	}
	m.checker.Enqueue(hashes, m.clock())
}

// pumpBadges sends the checker's next batch, or schedules one tick for when
// it will be due. The checker keeps every request under TorBox's limit.
func (m *AppModel) pumpBadges() tea.Cmd {
	if m.cached == nil {
		return nil
	}
	batch, wait := m.checker.Next(m.clock())
	if batch != nil {
		parent, checker := m.ctx, m.cached
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, badgeTimeout)
			defer cancel()
			cached, err := checker.CheckCached(ctx, batch)
			return badgeCheckedMsg{Batch: batch, Cached: cached, Err: err}
		}
	}
	if wait > 0 && !m.badgeTicking {
		m.badgeTicking = true
		return m.tick(wait, func(time.Time) tea.Msg { return badgeTickMsg{} })
	}
	return nil
}

func (m AppModel) libraryHashes() map[string]int {
	lib := make(map[string]int, len(m.torrents))
	for _, t := range m.torrents {
		if t.Hash != "" {
			lib[t.Hash] = t.ID
		}
	}
	return lib
}

func (m AppModel) badgeFor(r search.Result, lib map[string]int) search.Badge {
	if r.Hash == "" {
		if r.Pending() {
			return search.BadgeResolving
		}
		return search.BadgeUnknown
	}
	if _, ok := lib[r.Hash]; ok {
		return search.BadgeInLibrary
	}
	cached, ok := m.checker.Lookup(r.Hash, m.clock())
	switch {
	case !ok:
		return search.BadgeUnknown
	case cached:
		return search.BadgeCached
	default:
		return search.BadgeUncached
	}
}

func (m *AppModel) syncSearchCursor(keep string) {
	rows := m.visibleReleases()
	m.sv.cursor = min(m.sv.cursor, max(len(rows)-1, 0))
	for i, r := range rows {
		if r.Key() == keep {
			m.sv.cursor = i
			break
		}
	}
	m.clampSearchScroll(len(rows))
}

func (m AppModel) shownRows() []search.Result {
	var out []search.Result
	for _, sec := range m.sv.sections {
		limit := min(sec.Shown, len(sec.Rows))
		out = append(out, sec.Rows[:limit]...)
	}
	return out
}

func (m AppModel) isRowVisible(r search.Result, lib map[string]int, q string) bool {
	if m.sv.cachedOnly {
		b := m.badgeFor(r, lib)
		if b != search.BadgeCached && b != search.BadgeInLibrary {
			return false
		}
	}
	if q != "" && !strings.Contains(strings.ToLower(r.Title), q) {
		return false
	}
	return true
}

type visibleSection struct {
	idx  int
	sec  search.Section
	rows []search.Result
}

func (m AppModel) visibleSections() []visibleSection {
	lib := m.libraryHashes()
	q := strings.ToLower(strings.TrimSpace(m.sv.filter.Value()))
	out := make([]visibleSection, 0, len(m.sv.sections))
	for si, sec := range m.sv.sections {
		limit := min(sec.Shown, len(sec.Rows))
		var rows []search.Result
		for _, r := range sec.Rows[:limit] {
			if m.isRowVisible(r, lib, q) {
				rows = append(rows, r)
			}
		}
		if len(rows) > 0 {
			out = append(out, visibleSection{idx: si, sec: sec, rows: rows})
		}
	}
	return out
}

func (m AppModel) visibleReleases() []search.Result {
	var out []search.Result
	for _, vs := range m.visibleSections() {
		out = append(out, vs.rows...)
	}
	return out
}

func (m AppModel) cursorSectionIdx() (int, bool) {
	pos := 0
	for _, vs := range m.visibleSections() {
		if m.sv.cursor >= pos && m.sv.cursor < pos+len(vs.rows) {
			return vs.idx, true
		}
		pos += len(vs.rows)
	}
	return 0, false
}

func (m *AppModel) showMoreInSection() tea.Cmd {
	secIdx, ok := m.cursorSectionIdx()
	if !ok {
		return nil
	}
	sec := &m.sv.sections[secIdx]
	step := m.caps()[sec.Group]
	if sec.Late {
		step = search.LateCap
	}
	added := sec.More(step)
	if len(added) == 0 {
		m.sv.status, m.sv.isErr = "No more releases in this group", false
		return nil
	}
	m.sv.status = ""
	keep := m.selectedKey()
	m.queueBadges(added)
	m.syncSearchCursor(keep)
	return tea.Batch(m.pumpResolves(false), m.pumpBadges())
}

func (m AppModel) selectedKey() string {
	if row := m.selectedRelease(); row != nil {
		return row.Key()
	}
	return ""
}

func (m AppModel) selectedRelease() *search.Result {
	rows := m.visibleReleases()
	if m.sv.cursor >= 0 && m.sv.cursor < len(rows) {
		return &rows[m.sv.cursor]
	}
	return nil
}

// chooseRelease is Enter on a release: a pending row resolves first, a
// hash already in the library selects that torrent, and anything else is
// added.
func (m *AppModel) chooseRelease(row search.Result) tea.Cmd {
	if row.Hash == "" {
		if !row.Pending() {
			return nil
		}
		m.sv.addAfter = row.Key()
		m.sv.status, m.sv.isErr = "Getting the magnet for that release…", false
		if m.sv.resolving[row.Key()] {
			return nil
		}
		return m.pumpResolves(true)
	}
	if id, ok := m.libraryHashes()[row.Hash]; ok {
		m.closeSearch()
		m.searchInput.Reset()
		cmd := m.showTab(TabTorrents)
		m.jumpTo = id
		m.selectJumpTo()
		return cmd
	}
	if m.torboxClient == nil {
		m.sv.status, m.sv.isErr = "TorBox API key not configured", true
		return nil
	}
	m.sv.status, m.sv.isErr = "Adding "+row.Title+"…", false
	client, parent := m.torboxClient, m.ctx
	magnet := search.Magnet(row.Hash, row.Title)
	title := row.Title
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		resp, err := client.CreateTorrent(ctx, torbox.CreateTorrentRequest{Magnet: magnet})
		if err != nil {
			return searchAddedMsg{Title: title, Err: err}
		}
		return searchAddedMsg{Title: title, TorrentID: resp.TorrentID}
	}
}

func (m *AppModel) takeAdded(msg searchAddedMsg) tea.Cmd {
	if msg.Err != nil {
		if m.activeView == ViewSearch {
			m.sv.status, m.sv.isErr = fmt.Sprintf("Failed to add %s: %v", msg.Title, msg.Err), true
		} else {
			m.statusText, m.isStatusErr = fmt.Sprintf("Failed to add %s: %v", msg.Title, msg.Err), true
		}
		return nil
	}
	m.closeSearch()
	m.searchInput.Reset()
	tabCmd := m.showTab(TabTorrents)
	m.jumpTo = msg.TorrentID
	m.statusText, m.isStatusErr = "Added "+msg.Title+"; waiting for TorBox to list it", false
	return tea.Batch(tabCmd, m.refetchLibrary(TabTorrents, true))
}

// selectJumpTo moves the cursor to the torrent an add or a library hit is
// waiting on, once the list has it.
func (m *AppModel) selectJumpTo() {
	if m.jumpTo == 0 || m.activeTab != TabTorrents {
		return
	}
	for i, item := range m.currentItems() {
		if item.ID == m.jumpTo {
			m.jumpTo = 0
			m.cursor = i
			visibleLines := m.height - 8
			if visibleLines > 0 && m.cursor >= m.topIndex+visibleLines {
				m.topIndex = m.cursor - visibleLines + 1
			}
			if m.cursor < m.topIndex {
				m.topIndex = m.cursor
			}
			if strings.HasPrefix(m.statusText, "Added ") {
				m.statusText = strings.TrimSuffix(m.statusText, "; waiting for TorBox to list it")
			}
			return
		}
	}
	if strings.HasSuffix(m.statusText, "; waiting for TorBox to list it") {
		m.statusText = strings.TrimSuffix(m.statusText, "; waiting for TorBox to list it") + "; it isn't listed yet, and the next refresh will select it"
	}
}

func (m AppModel) searchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		m.cancel()
		return m, tea.Quit
	}
	switch m.sv.stage {
	case stageQuery:
		switch k {
		case "esc":
			m.closeSearch()
			return m, nil
		case "enter":
			return m, m.submitQuery(false)
		case "ctrl+r":
			return m, m.submitQuery(true)
		}
		var cmd tea.Cmd
		m.sv.input, cmd = m.sv.input.Update(msg)
		return m, cmd

	case stageTitles:
		switch k {
		case "esc":
			return m, m.backToQuery()
		case "up", "k":
			m.moveSearchCursor(-1, len(m.sv.hits))
		case "down", "j":
			m.moveSearchCursor(1, len(m.sv.hits))
		case "enter":
			if m.sv.cursor < len(m.sv.hits) {
				return m, m.pickWork(m.sv.hits[m.sv.cursor])
			}
		}
		return m, nil

	case stageEpisode:
		switch k {
		case "esc":
			m.sv.stage = stageTitles
			m.sv.status, m.sv.isErr = "", false
			return m, nil
		case "enter":
			season, episode, ok := search.ParseEpisode(m.sv.input.Value())
			if !ok {
				m.sv.status, m.sv.isErr = "Type S2, S2E5, or leave it blank for the whole show", true
				return m, nil
			}
			m.sv.work.Season, m.sv.work.Episode = season, episode
			return m, m.startReleases("")
		}
		var cmd tea.Cmd
		m.sv.input, cmd = m.sv.input.Update(msg)
		return m, cmd
	}

	if m.sv.filtering {
		keep := m.selectedKey()
		switch k {
		case "esc":
			m.sv.filtering = false
			m.sv.filter.Blur()
			m.sv.filter.Reset()
		case "enter":
			m.sv.filtering = false
			m.sv.filter.Blur()
		default:
			var cmd tea.Cmd
			m.sv.filter, cmd = m.sv.filter.Update(msg)
			m.syncSearchCursor(keep)
			return m, cmd
		}
		m.syncSearchCursor(keep)
		return m, nil
	}

	switch k {
	case "esc":
		return m, m.backToQuery()
	case "s":
		return m, m.backToQuery()
	case "/":
		m.sv.filtering = true
		m.sv.filter.Focus()
		return m, textinput.Blink
	case "O":
		m.sv.sortKey = m.sv.sortKey.Next()
		keep := m.selectedKey()
		m.sv.prefs = m.searchPrefs()
		for i := range m.sv.sections {
			m.sv.sections[i].Order(m.sv.sortKey, m.sv.prefs)
		}
		m.syncSearchCursor(keep)
		return m, nil
	case "c":
		keep := m.selectedKey()
		m.sv.cachedOnly = !m.sv.cachedOnly
		m.syncSearchCursor(keep)
		return m, nil
	case "m":
		return m, m.showMoreInSection()
	case "up", "k":
		m.moveSearchCursor(-1, len(m.visibleReleases()))
		return m, m.pumpResolves(false)
	case "down", "j":
		m.moveSearchCursor(1, len(m.visibleReleases()))
		return m, m.pumpResolves(false)
	case "enter":
		if row := m.selectedRelease(); row != nil {
			return m, m.chooseRelease(*row)
		}
	}
	return m, nil
}

// backToQuery cancels the search and returns to the box with its text.
func (m *AppModel) backToQuery() tea.Cmd {
	input := m.sv.input
	m.stopSearch()
	m.sv.input = newSearchInput(queryPlaceholder)
	if input.Placeholder == queryPlaceholder {
		m.sv.input.SetValue(input.Value())
	}
	m.sv.input.Focus()
	return textinput.Blink
}

func (m *AppModel) moveSearchCursor(delta, n int) {
	m.sv.cursor = max(0, min(m.sv.cursor+delta, n-1))
	m.clampSearchScroll(n)
}

func (m AppModel) searchLines() int {
	if n := m.height - 12; n > 0 {
		return n
	}
	return 10
}

func (m *AppModel) clampSearchScroll(n int) {
	lines := m.searchLines()
	if m.sv.stage == stageReleases && m.sv.drawn {
		cursorLine, anchorLine, totalLines := m.releaseScrollBounds()
		if anchorLine < m.sv.top {
			m.sv.top = anchorLine
		}
		if cursorLine >= m.sv.top+lines {
			m.sv.top = cursorLine - lines + 1
		}
		m.sv.top = max(0, min(m.sv.top, max(totalLines-lines, 0)))
		return
	}
	if m.sv.cursor < m.sv.top {
		m.sv.top = m.sv.cursor
	}
	if m.sv.cursor >= m.sv.top+lines {
		m.sv.top = m.sv.cursor - lines + 1
	}
	m.sv.top = max(0, min(m.sv.top, max(n-lines, 0)))
}

func (m AppModel) releaseScrollBounds() (cursorLine, anchorLine, totalLines int) {
	rowIdx := 0
	lineIdx := 0
	for _, vs := range m.visibleSections() {
		headerLine := lineIdx
		lineIdx++
		for ri := range vs.rows {
			if rowIdx == m.sv.cursor {
				cursorLine = lineIdx
				anchorLine = lineIdx
				if ri == 0 {
					anchorLine = headerLine
				}
			}
			rowIdx++
			lineIdx++
		}
	}
	return cursorLine, anchorLine, lineIdx
}

func (m AppModel) renderSearch() string {
	var sb strings.Builder
	pad := lipgloss.NewStyle().Padding(0, 1)
	muted := lipgloss.NewStyle().Foreground(ColorSubtext0)

	switch m.sv.stage {
	case stageQuery, stageEpisode:
		label := "Search"
		if m.sv.stage == stageEpisode {
			label = "Season or episode of " + titleYear(m.sv.work.Title, m.sv.work.Year)
		}
		sb.WriteString(pad.Render(m.theme.ModalHeader.Render(label)))
		sb.WriteString("\n")
		sb.WriteString(pad.Render(m.sv.input.View()))
		sb.WriteString("\n")

	case stageTitles:
		sb.WriteString(pad.Render(m.theme.ModalHeader.Render("Pick a title for “" + m.sv.input.Value() + "”")))
		sb.WriteString("\n")
		end := min(m.sv.top+m.searchLines(), len(m.sv.hits))
		for i := m.sv.top; i < end; i++ {
			h := m.sv.hits[i]
			cursor, style := "  ", m.theme.ItemTitle
			if i == m.sv.cursor {
				cursor, style = "❯ ", m.theme.ItemSelected
			}
			fmt.Fprintf(&sb, "%s%s %s\n", cursor, muted.Render(fmt.Sprintf("%-5s", h.Kind)), style.Render(titleYear(h.Title, h.Year)))
		}

	case stageReleases:
		sb.WriteString(pad.Render(m.searchSummary()))
		sb.WriteString("\n")
		if m.sv.filtering || m.sv.filter.Value() != "" {
			sb.WriteString(pad.Render(m.sv.filter.View()))
			sb.WriteString("\n")
		}
		sb.WriteString(m.renderReleases())
	}

	if m.sv.status != "" {
		style := muted
		if m.sv.isErr {
			style = m.theme.StatusError
		}
		sb.WriteString(pad.Render(style.Render(m.sv.status)))
		sb.WriteString("\n")
	}
	if m.badgeNote != "" {
		sb.WriteString(pad.Render(muted.Render(m.badgeNote)))
		sb.WriteString("\n")
	}
	return sb.String()
}

// searchSummary is the status line: each indexer running, done with a
// count, or failed, then what was dropped and the sort.
func (m AppModel) searchSummary() string {
	parts := make([]string, 0, len(m.sv.runs)+2)
	if len(m.sv.runs) == 0 && !m.sv.isErr {
		parts = append(parts, "Asking Prowlarr for its indexers…")
	}
	for _, r := range m.sv.runs {
		switch {
		case r.pending > 0:
			parts = append(parts, r.name+" …")
		case r.err != nil && r.count == 0:
			parts = append(parts, r.name+" ✗ "+prowlarr.Describe(r.err))
		default:
			parts = append(parts, fmt.Sprintf("%s ✓ %d", r.name, r.count))
		}
	}
	if unresolved := m.sv.tally.Dropped[search.DropUnresolved]; unresolved > 0 {
		parts = append(parts, fmt.Sprintf("%d couldn't be resolved", unresolved))
	}
	parts = append(parts, "order: "+m.sv.sortKey.String())
	return strings.Join(parts, " · ")
}

func (m AppModel) renderReleases() string {
	muted := lipgloss.NewStyle().Foreground(ColorSubtext0)
	if !m.sv.drawn {
		if len(m.sv.runs) > 0 {
			return "  " + muted.Render(fmt.Sprintf("Searching… %d of %d indexers", m.answeredCount(), len(m.sv.runs))) + "\n"
		}
		return ""
	}
	sections := m.visibleSections()
	if len(sections) == 0 && m.sv.cachedOnly {
		return "  " + muted.Render("No shown release is cached yet. Press c to show all.") + "\n"
	}
	lib := m.libraryHashes()
	var lines []string
	rowIdx := 0
	for _, vs := range sections {
		title := vs.sec.Group.Header()
		if vs.sec.Late {
			title = "Late results"
		}
		lines = append(lines, "  "+muted.Render(fmt.Sprintf("── %s · %d of %d ──", title, len(vs.rows), len(vs.sec.Rows))))
		for _, r := range vs.rows {
			cursor, style := "  ", m.theme.ItemTitle
			if rowIdx == m.sv.cursor {
				cursor, style = "❯ ", m.theme.ItemSelected
			}
			media := releaseMedia(r, vs.sec.Late)
			name := r.Title
			if r.Pack {
				name += " [pack]"
			}
			if avail := m.width - 60; avail > 10 {
				name = truncateToWidth(name, avail)
			}
			lines = append(lines, fmt.Sprintf("%s%s %-5s %s %5d  %s  %s",
				cursor, m.badgeFor(r, lib), dashIfEmpty(r.Lang.Label()),
				m.theme.ItemSize.Render(fmt.Sprintf("%9s", formatBytes(r.Size))), r.Seeders,
				padToWidth(truncateToWidth(media, 24), 24), style.Render(name)))
			rowIdx++
		}
	}
	if len(lines) == 0 {
		return ""
	}
	start := min(m.sv.top, len(lines))
	end := min(start+m.searchLines(), len(lines))
	return strings.Join(lines[start:end], "\n") + "\n"
}

// releaseMedia is what ParsedMedia knows about a release beyond its
// resolution: source, codec, HDR, audio and group (plus resolution for late rows).
func releaseMedia(r search.Result, includeRes bool) string {
	var parts []string
	if includeRes && r.Media.Resolution != "" {
		parts = append(parts, r.Media.Resolution)
	}
	for _, s := range []string{r.Media.Source, r.Media.Codec, r.Media.HDR, r.Media.Audio, r.Media.ReleaseGroup} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

var releaseHints = []footerHint{
	{"[Enter] Add", 0},
	{"[c] Cached", 4},
	{"[m] More", 3},
	{"[/] Filter", 5},
	{"[O] Order", 2},
	{"[s] New search", 1},
	{"[Esc] Back", 0},
}

func (m AppModel) searchShortcuts(width int) string {
	switch m.sv.stage {
	case stageQuery:
		return "[Enter] Search  [ctrl+r] Raw search  [Esc] Close"
	case stageTitles:
		return "[Enter] Pick  [Esc] Back"
	case stageEpisode:
		return "[Enter] Search  [Esc] Back"
	}
	return fitHints(releaseHints, width)
}

func titleYear(title string, year int) string {
	title = matcher.SanitizeDisplay(title)
	if year > 0 {
		return fmt.Sprintf("%s (%d)", title, year)
	}
	return title
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
