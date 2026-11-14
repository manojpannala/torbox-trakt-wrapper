package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

const (
	tuiHashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tuiHashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fakeSearcher struct {
	mu       sync.Mutex
	indexers []search.Indexer
	rows     map[int][]search.Result
	block    bool
	reqs     []search.Request
	canceled int
}

func (f *fakeSearcher) Indexers(context.Context) ([]search.Indexer, error) {
	return f.indexers, nil
}

func (f *fakeSearcher) Search(ctx context.Context, req search.Request) ([]search.Result, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	block := f.block
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		f.mu.Lock()
		f.canceled++
		f.mu.Unlock()
		return nil, ctx.Err()
	}
	if req.ByID {
		return nil, nil
	}
	return append([]search.Result(nil), f.rows[req.IndexerID]...), nil
}

func (f *fakeSearcher) queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.reqs))
	for i, r := range f.reqs {
		out[i] = fmt.Sprintf("%d %s %s", r.IndexerID, r.Type, r.Query)
	}
	return out
}

type fakeTitles struct {
	hits []trakt.TitleHit
}

func (f fakeTitles) SearchTitles(context.Context, string) ([]trakt.TitleHit, error) {
	return f.hits, nil
}

func (f fakeTitles) LookupIMDb(_ context.Context, id string) (*trakt.TitleHit, error) {
	for _, h := range f.hits {
		if h.IDs.IMDB == id {
			return &h, nil
		}
	}
	return nil, nil
}

type fakeCached struct {
	mu       sync.Mutex
	cached   map[string]bool
	limitFor int
	batches  [][]string
}

func (f *fakeCached) CheckCached(_ context.Context, hashes []string) (map[string]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, append([]string(nil), hashes...))
	if f.limitFor > 0 {
		f.limitFor--
		return nil, fmt.Errorf("%w: %w", torbox.ErrRateLimited, &torbox.APIError{StatusCode: http.StatusTooManyRequests, RetryAfter: time.Minute})
	}
	out := map[string]bool{}
	for _, h := range hashes {
		out[h] = f.cached[h]
	}
	return out, nil
}

var sampleHits = []trakt.TitleHit{
	{Kind: "movie", Title: "Sample Film", Year: 2014, IDs: trakt.IDs{Trakt: 1, IMDB: "tt0000001"}},
	{Kind: "show", Title: "Sample Show", Year: 2019, IDs: trakt.IDs{Trakt: 2, IMDB: "tt0000002"}},
}

var sampleIndexers = []search.Indexer{
	{ID: 1, Name: "General"},
	{ID: 7, Name: "IDs", MovieIMDb: true, TVIMDb: true},
}

// searchModel is a model with search on, fake backends, and a clock that a
// badge tick moves forward instead of sleeping.
func searchModel(t *testing.T, mutate func(*config.Config)) (AppModel, *fakeSearcher, *fakeCached) {
	t.Helper()
	m := testModelWith(t, func(c *config.Config) {
		c.Search.ProwlarrURL = "http://127.0.0.1:9696"
		c.Search.ProwlarrAPIKey = "test-prowlarr-key"
		if mutate != nil {
			mutate(c)
		}
	})
	fs := &fakeSearcher{indexers: sampleIndexers, rows: map[int][]search.Result{
		1: {
			{Title: "Sample.Film.2014.1080p.BluRay.x264-GRP", Hash: tuiHashA, Size: 2 << 30, Seeders: 40, Indexers: []string{"General"}, IndexerID: 1, GUID: "a"},
			{Title: "Sample.Film.2014.2160p.WEB-DL.HDR-GRP", Hash: tuiHashB, Size: 9 << 30, Seeders: 70, Indexers: []string{"General"}, IndexerID: 1, GUID: "b"},
			{Title: "Other.Film.2014.1080p-GRP", Hash: strings.Repeat("c", 40), Seeders: 90, Indexers: []string{"General"}, IndexerID: 1, GUID: "c"},
		},
	}}
	fc := &fakeCached{cached: map[string]bool{tuiHashA: true}}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.searcher, m.titles, m.cached = fs, fakeTitles{hits: sampleHits}, fc
	m.clock = func() time.Time { return now }
	m.tick = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		return func() tea.Msg {
			now = now.Add(d)
			return fn(now)
		}
	}
	m, _ = sendMsg(m, tea.WindowSizeMsg{Width: 140, Height: 40})
	return m, fs, fc
}

// settle runs cmd and every command its messages lead to, feeding each
// message back through Update, until nothing is left. Spinner and cursor
// ticks repeat forever, so they are not fed back. Each func in each sees the
// model after every message.
func settle(t *testing.T, m AppModel, cmd tea.Cmd, each ...func(AppModel)) AppModel {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		require.Less(t, steps, 500, "the search never settled")
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil, spinner.TickMsg, cursor.BlinkMsg:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			var next tea.Cmd
			m, next = sendMsg(m, msg)
			for _, f := range each {
				f(m)
			}
			queue = append(queue, next)
		}
	}
	return m
}

// plain is the view as text, without colours.
func plain(m AppModel) string {
	return ansi.Strip(viewText(m))
}

// moveTo walks the cursor to the first release that matches, up then down,
// and returns the resolves the walk started without running them.
func moveTo(t *testing.T, m AppModel, match func(search.Result) bool) (AppModel, tea.Cmd) {
	t.Helper()
	var cmds []tea.Cmd
	for _, k := range []tea.KeyPressMsg{upKey, downKey} {
		for range len(m.visibleReleases()) {
			if row := m.selectedRelease(); row != nil && match(*row) {
				return m, tea.Batch(cmds...)
			}
			var cmd tea.Cmd
			m, cmd = sendKey(m, k)
			cmds = append(cmds, cmd)
		}
	}
	require.FailNow(t, "no release matches")
	return m, nil
}

func typeText(m AppModel, s string) AppModel {
	for _, r := range s {
		m, _ = sendKey(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func hit(t *testing.T, m AppModel, k tea.KeyPressMsg) AppModel {
	t.Helper()
	m, cmd := sendKey(m, k)
	return settle(t, m, cmd)
}

var (
	sKey     = tea.KeyPressMsg{Code: 's', Text: "s"}
	cKey     = tea.KeyPressMsg{Code: 'c', Text: "c"}
	mKey     = tea.KeyPressMsg{Code: 'm', Text: "m"}
	capSKey  = tea.KeyPressMsg{Code: 'S', Text: "S"}
	capOKey  = tea.KeyPressMsg{Code: 'O', Text: "O"}
	rawKey   = tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	slashKey = tea.KeyPressMsg{Code: '/', Text: "/"}
	upKey    = tea.KeyPressMsg{Code: 'k', Text: "k"}
)

func TestSearch_OffSaysWhereToConfigure(t *testing.T) {
	m := testModel(t)

	m, _ = sendKey(m, sKey)

	assert.Equal(t, ViewLibrary, m.activeView)
	assert.True(t, m.isStatusErr)
	assert.Contains(t, m.statusText, "Search is off: set prowlarr_url and prowlarr_api_key under [search]")
}

func TestSearch_AnInsecureURLTurnsSearchOffButNotTheApp(t *testing.T) {
	m := testModelWith(t, func(c *config.Config) {
		c.Search.ProwlarrURL = "http://prowlarr.example.invalid"
		c.Search.ProwlarrAPIKey = "test-prowlarr-key"
	})

	m, _ = sendKey(m, sKey)

	assert.Equal(t, ViewLibrary, m.activeView)
	assert.Contains(t, m.statusText, "prowlarr_url must use https")
}

func TestSearch_TitleThenReleasesWithBadges(t *testing.T) {
	m, fs, fc := searchModel(t, nil)

	m, _ = sendKey(m, sKey)
	require.Equal(t, ViewSearch, m.activeView)
	m = typeText(m, "sample film")
	m = hit(t, m, enterKey)
	require.Equal(t, stageTitles, m.sv.stage)
	assert.Contains(t, plain(m), "movie Sample Film (2014)")
	assert.Contains(t, plain(m), "show  Sample Show (2019)")
	assert.Empty(t, fs.queries(), "Prowlarr waits until a title is picked")

	m = hit(t, m, enterKey)

	assert.ElementsMatch(t, []string{
		"1 search Sample Film 2014",
		"7 movie {ImdbId:tt0000001}",
		"7 search Sample Film 2014",
	}, fs.queries())
	view := plain(m)
	assert.Contains(t, view, "General ✓ 2 · IDs ✓ 0 · order: rank")
	assert.NotContains(t, view, "Other.Film", "the title filter drops a different film")
	rows := m.shownRows()
	require.Len(t, rows, 2)
	assert.Equal(t, tuiHashB, rows[0].Hash, "rank puts 70-seeder B first")
	assert.Contains(t, view, "● -")
	assert.Contains(t, view, "○ -")
	assert.Contains(t, view, "bluray x264 GRP")
	require.Len(t, fc.batches, 1, "one badge batch for both hashes")
	assert.ElementsMatch(t, []string{tuiHashA, tuiHashB}, fc.batches[0])
}

func TestSearch_TraktTitlesDropControlCharacters(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m.titles = fakeTitles{hits: []trakt.TitleHit{
		{Kind: "show", Title: "Sample\x1b[2J Show\a\u009b", Year: 2019, IDs: trakt.IDs{Trakt: 3, IMDB: "tt0000003"}},
	}}
	m, _ = sendKey(m, sKey)
	m = typeText(m, "sample show")
	m = hit(t, m, enterKey)
	require.Equal(t, stageTitles, m.sv.stage)
	assert.Contains(t, plain(m), "show  Sample[2J Show (2019)")
	for _, bad := range []string{"\x1b[2J", "\a", "\u009b"} {
		assert.NotContains(t, viewText(m), bad)
	}

	m = hit(t, m, enterKey)
	require.Equal(t, stageEpisode, m.sv.stage)
	assert.Contains(t, plain(m), "Season or episode of Sample[2J Show (2019)")
	for _, bad := range []string{"\x1b[2J", "\a", "\u009b"} {
		assert.NotContains(t, viewText(m), bad)
	}
}

func TestSearch_ShowAsksForSeasonOrEpisode(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "sample show")
	m = hit(t, m, enterKey)
	m = hit(t, m, downKey)
	m = hit(t, m, enterKey)
	require.Equal(t, stageEpisode, m.sv.stage)

	m = typeText(m, "2")
	m = hit(t, m, enterKey)
	assert.Equal(t, stageEpisode, m.sv.stage)
	assert.Contains(t, m.sv.status, "Type S2, S2E5")

	m, _ = sendKey(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = typeText(m, "S2E5")
	m = hit(t, m, enterKey)

	assert.ElementsMatch(t, []string{
		"1 search Sample Show S02E05",
		"7 tvsearch {ImdbId:tt0000002}{Season:2}{Episode:5}",
		"7 search Sample Show S02E05",
	}, fs.queries())
}

func TestSearch_AnIMDbIDSkipsTheTitlesStep(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")

	m = hit(t, m, enterKey)

	assert.Equal(t, stageReleases, m.sv.stage)
	assert.Contains(t, fs.queries(), "7 movie {ImdbId:tt0000001}")
	assert.Contains(t, fs.queries(), "1 search Sample Film 2014", "the Trakt lookup supplies the title")
}

func TestSearch_RawSendsTheTextAsTyped(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "other film")

	m = hit(t, m, rawKey)

	assert.ElementsMatch(t, []string{"1 search other film", "7 search other film"}, fs.queries())
	assert.Len(t, m.shownRows(), 3, "no title filter on a raw search")
}

func TestSearch_EscCancelsTheRequestsInFlight(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	fs.block = true
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m, cmd := sendKey(m, enterKey)
	m, cmd = sendMsg(m, cmd())
	m, cmd = sendMsg(m, cmd())
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)

	m, _ = sendKey(m, escKey)
	var wg sync.WaitGroup
	for _, c := range batch {
		wg.Go(func() {
			m2, _ := sendMsg(m, c())
			assert.Empty(t, m2.sv.runs, "a reply to a cancelled search is dropped")
		})
	}
	wg.Wait()

	assert.Equal(t, 3, fs.canceled)
	assert.Equal(t, stageQuery, m.sv.stage)
	assert.Equal(t, "tt0000001", m.sv.input.Value(), "Esc goes back to the box with the query")
}

func TestSearch_AReplyToAnOlderSearchIsIgnored(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)
	rows := len(m.shownRows())

	m, _ = sendMsg(m, searchReleasesMsg{Seq: m.sv.seq - 1, Req: search.Request{IndexerID: 1},
		Rows: []search.Result{{Title: "Sample.Film.2014.720p-OLD", Hash: strings.Repeat("d", 40), IndexerID: 1, GUID: "d"}}})

	assert.Len(t, m.shownRows(), rows)
}

func TestSearch_OCyclesTheSortAndSlashFilters(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	m = hit(t, m, capOKey)
	assert.Contains(t, plain(m), "order: size")
	assert.Equal(t, tuiHashB, m.shownRows()[0].Hash, "the 9 GiB release is largest")

	m = hit(t, m, slashKey)
	m = typeText(m, "1080p")
	m = hit(t, m, enterKey)
	rows := m.visibleReleases()
	require.Len(t, rows, 1)
	assert.Equal(t, tuiHashA, rows[0].Hash)
}

func TestSearch_PendingRowResolvesThenEnterAddsAndSelectsIt(t *testing.T) {
	var created int
	stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/torrents/createtorrent": func(w http.ResponseWriter) {
			created++
			_, _ = w.Write([]byte(`{"success":true,"data":{"torrent_id":77,"hash":"` + tuiHashB + `"}}`))
		},
		"/torrents/mylist": respond(200, `{"success":true,"data":[
			{"id":5,"name":"Test.Feature.Alpha.2023.1080p.mkv"},
			{"id":77,"name":"Sample.Film.2014.1080p.WEB-DL.HDR-GRP","hash":"`+tuiHashB+`"}]}`),
	})
	m, fs, _ := searchModel(t, nil)
	var resolves int
	fs.rows[1] = []search.Result{
		{Title: "Sample.Film.2014.1080p.WEB-DL.HDR-GRP", Seeders: 70, Indexers: []string{"General"}, IndexerID: 1, GUID: "b",
			Resolve: func(context.Context) (string, error) { resolves++; return tuiHashB, nil }},
		{Title: "Sample.Film.2014.2160p-BAD", Seeders: 95, Indexers: []string{"General"}, IndexerID: 1, GUID: "bad",
			Resolve: func(context.Context) (string, error) { return "", search.ErrNotMagnet }},
	}
	for i := range 10 {
		hash := fmt.Sprintf("%040x", i+1)
		res := "1080p"
		if i < 4 {
			res = "2160p"
		}
		fs.rows[1] = append(fs.rows[1], search.Result{
			Title: fmt.Sprintf("Sample.Film.2014.%s-G%d", res, i), Seeders: 80 + i, Indexers: []string{"General"}, IndexerID: 1, GUID: fmt.Sprint("g", i),
			Resolve: func(context.Context) (string, error) { return hash, nil },
		})
	}
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	assert.Equal(t, 1, m.sv.tally.Dropped[search.DropUnresolved], "a release with no magnet is dropped")
	assert.Contains(t, plain(m), "1 couldn't be resolved")
	assert.Len(t, m.shownRows(), 11)
	assert.Zero(t, resolves, "the ten best-seeded resolve up front, and b is eleventh")

	m, walk := moveTo(t, m, func(r search.Result) bool { return r.GUID == "b" })
	require.True(t, m.sv.resolving["1/b"], "the row under the cursor resolves")
	assert.Contains(t, plain(m), "… -")
	m, enter := sendKey(m, enterKey)
	assert.Contains(t, plain(m), "Getting the magnet for that release…")
	m = settle(t, m, tea.Batch(walk, enter))

	assert.Equal(t, 1, resolves)
	assert.Equal(t, 1, created)
	assert.Equal(t, ViewLibrary, m.activeView)
	assert.Equal(t, TabTorrents, m.activeTab)
	item := m.selectedCurrentItem()
	require.NotNil(t, item)
	assert.Equal(t, 77, item.ID, "the new torrent is selected once the list has it")
	assert.Equal(t, "Added Sample.Film.2014.1080p.WEB-DL.HDR-GRP", m.statusText)

	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)
	assert.Equal(t, 1, resolves, "a resolved hash is remembered for the session")
	assert.Contains(t, plain(m), "■ -", "and it is now in the library")
}

func TestSearch_EnterOnALibraryHashSelectsTheExistingRow(t *testing.T) {
	m, _, fc := searchModel(t, nil)
	m, _ = sendMsg(m, TorrentsLoadedMsg{Gen: m.libGen[TabTorrents], Torrents: []torbox.Torrent{
		{ID: 3, Name: "Test.Feature.Alpha.2023.1080p.mkv"},
		{ID: 9, Name: "Sample.Film.2014.2160p.WEB-DL.HDR-GRP", Hash: strings.ToUpper(tuiHashB)},
	}})
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)
	require.Len(t, fc.batches, 1)
	assert.Equal(t, []string{tuiHashA}, fc.batches[0], "a library hash needs no check")

	m, walk := moveTo(t, m, func(r search.Result) bool { return r.Hash == tuiHashB })
	m = settle(t, m, walk)
	assert.Contains(t, plain(m), "■ -")
	m = hit(t, m, enterKey)

	assert.Equal(t, ViewLibrary, m.activeView)
	item := m.selectedCurrentItem()
	require.NotNil(t, item)
	assert.Equal(t, 9, item.ID)
}

func TestSearch_ARateLimitedBadgeCheckWaitsThenRetries(t *testing.T) {
	m, _, fc := searchModel(t, nil)
	fc.limitFor = 1
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m, cmd := sendKey(m, enterKey)

	var sawWait bool
	m = settle(t, m, cmd, func(m AppModel) {
		if strings.Contains(plain(m), "rate-limiting badge checks, retrying in 1m0s") {
			sawWait = true
			assert.Contains(t, plain(m), "? -", "a limited check leaves the badge unknown")
		}
	})

	assert.True(t, sawWait, "the wait is shown")
	assert.Len(t, fc.batches, 2, "one retry, after the wait")
	assert.Contains(t, plain(m), "● -")
	assert.NotContains(t, plain(m), "rate-limiting")
}

func TestSearch_SOnAMatchedRowSearchesByItsIMDbID(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	m, _ = sendMsg(m, TraktCatalogLoadedMsg{Gen: m.traktGen, Shows: []trakt.WatchedShow{
		{Plays: 1, Show: trakt.Show{Title: "Sample Show", Year: 2019, IDs: trakt.IDs{Trakt: 2, IMDB: "tt0000002"}}},
	}})
	m, _ = sendMsg(m, TorrentsLoadedMsg{Gen: m.libGen[TabTorrents], Torrents: []torbox.Torrent{
		{ID: 4, Name: "Sample.Show.S02E05.1080p.WEB.mkv"},
	}})

	m = hit(t, m, capSKey)

	assert.Equal(t, ViewSearch, m.activeView)
	assert.Contains(t, fs.queries(), "7 tvsearch {ImdbId:tt0000002}{Season:2}{Episode:5}")
}

func TestSearch_SOnAnUnmatchedRowSaysSo(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	m, _ = sendMsg(m, TorrentsLoadedMsg{Gen: m.libGen[TabTorrents], Torrents: []torbox.Torrent{
		{ID: 4, Name: "Unknown.Film.2014.1080p.mkv"},
	}})

	m = hit(t, m, capSKey)

	assert.Equal(t, ViewLibrary, m.activeView)
	assert.Contains(t, m.statusText, "No Trakt match for this row")
	assert.Empty(t, fs.queries())
}

func TestSearch_ShowsPendingCounterUntilAllAnswer(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	// Enter -> lookupWork -> searchWorkMsg -> fetchIndexers -> searchIndexersMsg
	m, cmd := sendKey(m, enterKey)
	m, cmd = sendMsg(m, cmd())
	m, cmd = sendMsg(m, cmd())
	m, _ = sendMsg(m, cmd())

	m, _ = sendMsg(m, searchReleasesMsg{
		Seq: m.sv.seq,
		Req: search.Request{IndexerID: 1, Indexer: "General"},
		Rows: []search.Result{
			{Title: "Sample.Film.2014.1080p-GRP", Hash: tuiHashA, Size: 2 << 30, Seeders: 10, Indexers: []string{"General"}, IndexerID: 1, GUID: "a"},
		},
	})
	view := plain(m)
	assert.Contains(t, view, "Searching… 1 of 2 indexers")
	assert.NotContains(t, view, "Sample.Film.2014.1080p-GRP")

	m, _ = sendMsg(m, searchReleasesMsg{
		Seq: m.sv.seq,
		Req: search.Request{IndexerID: 7, Indexer: "IDs", ByID: true},
		Rows: []search.Result{
			{Title: "Sample.Film.2014.2160p-GRP", Hash: tuiHashB, Size: 5 << 30, Seeders: 10, Indexers: []string{"IDs"}, IndexerID: 7, GUID: "b"},
		},
	})
	m, _ = sendMsg(m, searchReleasesMsg{
		Seq: m.sv.seq,
		Req: search.Request{IndexerID: 7, Indexer: "IDs"},
	})
	require.True(t, m.sv.drawn)
	require.Len(t, m.sv.sections, 2)
	assert.Equal(t, search.Group4K, m.sv.sections[0].Group)
	assert.Equal(t, search.Group1080p, m.sv.sections[1].Group)
}

func TestSearch_AFailedFirstAnswerDoesNotStartTheSettleWait(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m, cmd := sendKey(m, enterKey)
	m, cmd = sendMsg(m, cmd())
	m, cmd = sendMsg(m, cmd())
	m, _ = sendMsg(m, cmd())

	m, _ = sendMsg(m, searchReleasesMsg{
		Seq: m.sv.seq,
		Req: search.Request{IndexerID: 1, Indexer: "General"},
		Err: errors.New("indexer down"),
	})
	assert.False(t, m.sv.settleArmed, "an answer with no rows has nothing to draw")

	m, cmd = sendMsg(m, searchReleasesMsg{
		Seq: m.sv.seq,
		Req: search.Request{IndexerID: 7, Indexer: "IDs", ByID: true},
		Rows: []search.Result{
			{Title: "Sample.Film.2014.2160p-GRP", Hash: tuiHashB, Size: 5 << 30, Seeders: 10, Indexers: []string{"IDs"}, IndexerID: 7, GUID: "b"},
		},
	})
	require.True(t, m.sv.settleArmed)
	require.False(t, m.sv.drawn)
	m, _ = sendMsg(m, cmd())

	require.True(t, m.sv.drawn)
	require.Len(t, m.sv.sections, 1)
	assert.Equal(t, search.Group4K, m.sv.sections[0].Group)
	assert.False(t, m.sv.sections[0].Late)
}

func TestSearch_SettleTimeoutDrawsThenLateResultsAppend(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m, cmd := sendKey(m, enterKey)
	m, cmd = sendMsg(m, cmd())
	m, cmd = sendMsg(m, cmd())
	m, _ = sendMsg(m, cmd())

	m, _ = sendMsg(m, searchReleasesMsg{
		Seq: m.sv.seq,
		Req: search.Request{IndexerID: 1, Indexer: "General"},
		Rows: []search.Result{
			{Title: "Sample.Film.2014.1080p-GRP", Hash: tuiHashA, Size: 2 << 30, Seeders: 10, Indexers: []string{"General"}, IndexerID: 1, GUID: "a"},
		},
	})
	assert.False(t, m.sv.drawn)

	m, _ = sendMsg(m, searchSettleMsg{Seq: m.sv.seq})
	require.True(t, m.sv.drawn)
	require.Len(t, m.sv.sections, 1)

	lateHash := strings.Repeat("e", 40)
	m, _ = sendMsg(m, searchReleasesMsg{
		Seq: m.sv.seq,
		Req: search.Request{IndexerID: 7, Indexer: "IDs", ByID: true},
		Rows: []search.Result{
			{Title: "Sample.Film.2014.2160p-LATE", Hash: lateHash, Size: 5 << 30, Seeders: 99, Indexers: []string{"IDs"}, IndexerID: 7, GUID: "late"},
		},
	})
	require.Len(t, m.sv.sections, 2)
	assert.Equal(t, tuiHashA, m.sv.sections[0].Rows[0].Hash, "existing row order unchanged")
	assert.True(t, m.sv.sections[1].Late)
	assert.Equal(t, lateHash, m.sv.sections[1].Rows[0].Hash)
}

func TestSearch_LateDuplicateMergesIndexerWithoutLateSection(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m, cmd := sendKey(m, enterKey)
	m, cmd = sendMsg(m, cmd())
	m, cmd = sendMsg(m, cmd())
	m, _ = sendMsg(m, cmd())

	m, _ = sendMsg(m, searchReleasesMsg{
		Seq: m.sv.seq,
		Req: search.Request{IndexerID: 1, Indexer: "General"},
		Rows: []search.Result{
			{Title: "Sample.Film.2014.1080p-GRP", Hash: tuiHashA, Size: 2 << 30, Seeders: 10, Indexers: []string{"General"}, IndexerID: 1, GUID: "a"},
		},
	})
	m, _ = sendMsg(m, searchSettleMsg{Seq: m.sv.seq})

	m, _ = sendMsg(m, searchReleasesMsg{
		Seq: m.sv.seq,
		Req: search.Request{IndexerID: 7, Indexer: "IDs", ByID: true},
		Rows: []search.Result{
			{Title: "Sample.Film.2014.1080p-DUP", Hash: tuiHashA, Size: 2 << 30, Seeders: 99, Indexers: []string{"IDs"}, IndexerID: 7, GUID: "dup"},
		},
	})
	require.Len(t, m.sv.sections, 1)
	assert.False(t, m.sv.sections[0].Late)
	assert.Equal(t, []string{"General", "IDs"}, m.sv.sections[0].Rows[0].Indexers)
}

func TestSearch_BadgeCheckedDoesNotReorderAfterDraw(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	fs.rows[1] = []search.Result{
		{Title: "Sample.Film.2014.1080p.First-GRP", Hash: tuiHashB, Size: 2 << 30, Seeders: 90, Indexers: []string{"General"}, IndexerID: 1, GUID: "b"},
		{Title: "Sample.Film.2014.1080p.Second-GRP", Hash: tuiHashA, Size: 2 << 30, Seeders: 40, Indexers: []string{"General"}, IndexerID: 1, GUID: "a"},
	}
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	rows := m.visibleReleases()
	require.Len(t, rows, 2)
	assert.Equal(t, tuiHashB, rows[0].Hash)
	assert.Equal(t, tuiHashA, rows[1].Hash)
}

func TestSearch_CapsBadgeCheckToShownHashes(t *testing.T) {
	m, fs, fc := searchModel(t, nil)
	fs.rows[1] = nil
	for i := range 40 {
		fs.rows[1] = append(fs.rows[1], search.Result{
			Title:     fmt.Sprintf("Sample.Film.2014.1080p-G%02d", i),
			Hash:      fmt.Sprintf("%040x", i+10),
			Size:      2 << 30,
			Seeders:   100 - i,
			Indexers:  []string{"General"},
			IndexerID: 1,
			GUID:      fmt.Sprintf("g%02d", i),
		})
	}
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	_ = hit(t, m, enterKey)

	require.Len(t, fc.batches, 1)
	assert.Len(t, fc.batches[0], 10)
}

func TestSearch_FailedResolveRemovesRowWithoutPullingReplacement(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	h2 := strings.Repeat("2", 40)
	h3 := strings.Repeat("3", 40)
	fs.rows[1] = nil
	for i := range 11 {
		row := search.Result{
			Title:     fmt.Sprintf("Sample.Film.2014.1080p-G%02d", i),
			Size:      2 << 30,
			Seeders:   100 - i,
			Indexers:  []string{"General"},
			IndexerID: 1,
			GUID:      fmt.Sprintf("g%02d", i),
		}
		switch i {
		case 0:
			row.Resolve = func(context.Context) (string, error) { return "", search.ErrNotMagnet }
		case 1:
			row.Hash = h2
		default:
			row.Hash = fmt.Sprintf("%040x", i+100)
			if i == 2 {
				row.Hash = h3
			}
		}
		fs.rows[1] = append(fs.rows[1], row)
	}
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	require.Len(t, m.sv.sections, 1)
	assert.Equal(t, 9, m.sv.sections[0].Shown)
	assert.Equal(t, h2, m.sv.sections[0].Rows[0].Hash)
	assert.Equal(t, h3, m.sv.sections[0].Rows[1].Hash)
}

func TestSearch_InvalidCapsTurnsSearchOffWithoutStoppingApp(t *testing.T) {
	m := testModelWith(t, func(c *config.Config) {
		c.Search.ProwlarrURL = "http://127.0.0.1:9696"
		c.Search.ProwlarrAPIKey = "test-prowlarr-key"
		c.Search.Resolutions = map[string]int{"720p": 99}
	})

	m, _ = sendKey(m, sKey)

	assert.Equal(t, ViewLibrary, m.activeView)
	assert.Contains(t, m.statusText, "Search is off:")
	assert.Contains(t, m.statusText, "720p")
	assert.NotEmpty(t, plain(m))
}

func TestSearch_SUsesTraktLanguageToRankReleases(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	m.titles = fakeTitles{hits: []trakt.TitleHit{
		{Kind: "show", Title: "Sample Show", Year: 2019, Language: "ta", IDs: trakt.IDs{Trakt: 2, IMDB: "tt0000002"}},
	}}
	hTa := strings.Repeat("a", 40)
	hUntagged := strings.Repeat("b", 40)
	fs.rows[1] = []search.Result{
		{Title: "Sample.Show.S02E05.1080p-GRP", Hash: hUntagged, Size: 2 << 30, Seeders: 90, Indexers: []string{"General"}, IndexerID: 1, GUID: "u"},
		{Title: "Sample.Show.S02E05.1080p.Tamil-GRP", Hash: hTa, Size: 2 << 30, Seeders: 10, Indexers: []string{"General"}, IndexerID: 1, GUID: "t"},
	}
	m, _ = sendMsg(m, TraktCatalogLoadedMsg{Gen: m.traktGen, Shows: []trakt.WatchedShow{
		{Plays: 1, Show: trakt.Show{Title: "Sample Show", Year: 2019, IDs: trakt.IDs{Trakt: 2, IMDB: "tt0000002"}}},
	}})
	m, _ = sendMsg(m, TorrentsLoadedMsg{Gen: m.libGen[TabTorrents], Torrents: []torbox.Torrent{
		{ID: 4, Name: "Sample.Show.S02E05.1080p.WEB.mkv"},
	}})

	m = hit(t, m, capSKey)

	rows := m.visibleReleases()
	require.Len(t, rows, 2)
	assert.Equal(t, hTa, rows[0].Hash)
}

func TestSearch_ShowsResolutionHeadersAndLanguageColumn(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	hTa := strings.Repeat("c", 40)
	fs.rows[1] = []search.Result{
		{Title: "Sample.Film.2014.2160p.WEB-DL.Tamil-GRP", Hash: hTa, Size: 6 << 30, Seeders: 15, Indexers: []string{"General"}, IndexerID: 1, GUID: "ta"},
	}
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	view := plain(m)
	assert.Contains(t, view, "── 4K · 1 of 1 ──")
	assert.Contains(t, view, "TAM")
	assert.NotContains(t, view, "○ 2160p")
	headerIdx := strings.Index(view, "── 4K · 1 of 1 ──")
	rowIdx := strings.Index(view, "Sample.Film.2014.2160p")
	require.NotEqual(t, -1, headerIdx)
	require.NotEqual(t, -1, rowIdx)
	assert.Less(t, headerIdx, rowIdx)
	assert.NotContains(t, ansi.Strip(viewText(m)), "○ 2160p")
}

func TestSearch_CursorSkipsSectionHeadersAndStaysAtEnds(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	require.Equal(t, 0, m.sv.cursor)
	m = hit(t, m, upKey)
	assert.Equal(t, 0, m.sv.cursor, "Up on first row stays put")

	m = hit(t, m, downKey)
	assert.Equal(t, 1, m.sv.cursor, "Down from 4K row lands on 1080p row")
	sel := m.selectedRelease()
	require.NotNil(t, sel)
	assert.Equal(t, tuiHashA, sel.Hash)

	m = hit(t, m, downKey)
	assert.Equal(t, 1, m.sv.cursor, "Down on last row stays put")
}

func TestSearch_CachedToggleHidesUncachedAndShowsEmptyMessage(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	m = hit(t, m, cKey)
	view := plain(m)
	assert.Contains(t, view, "── 1080p · 1 of 1 ──")
	assert.NotContains(t, view, "── 4K ·", "4K only has an uncached release")

	m = hit(t, m, cKey)
	assert.Contains(t, plain(m), "── 4K · 1 of 1 ──", "second c shows uncached again")

	// With only uncached rows, c shows the empty cached message.
	m2, fs, _ := searchModel(t, nil)
	fs.rows[1] = []search.Result{
		{Title: "Sample.Film.2014.2160p-GRP", Hash: tuiHashB, Size: 5 << 30, Seeders: 20, Indexers: []string{"General"}, IndexerID: 1, GUID: "b"},
	}
	m2, _ = sendKey(m2, sKey)
	m2 = typeText(m2, "tt0000001")
	m2 = hit(t, m2, enterKey)
	m2 = hit(t, m2, cKey)
	assert.Contains(t, plain(m2), "No shown release is cached yet. Press c to show all.")
}

func TestSearch_MoreRevealsNextBatchAndQueuesBadges(t *testing.T) {
	m, fs, fc := searchModel(t, nil)
	fs.rows[1] = nil
	for i := range 15 {
		fs.rows[1] = append(fs.rows[1], search.Result{
			Title:     fmt.Sprintf("Sample.Film.2014.1080p-G%02d", i),
			Hash:      fmt.Sprintf("%040x", i+10),
			Size:      2 << 30,
			Seeders:   100 - i,
			Indexers:  []string{"General"},
			IndexerID: 1,
			GUID:      fmt.Sprintf("g%02d", i),
		})
	}
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)
	require.Len(t, fc.batches, 1)
	assert.Len(t, fc.batches[0], 10)
	assert.Contains(t, plain(m), "── 1080p · 10 of 15 ──")

	m = hit(t, m, mKey)
	assert.Len(t, m.shownRows(), 15)
	assert.Contains(t, plain(m), "── 1080p · 15 of 15 ──")
	require.Len(t, fc.batches, 2)
	assert.Len(t, fc.batches[1], 5)

	m = hit(t, m, mKey)
	assert.Contains(t, m.sv.status, "No more releases in this group")
}

func TestSearch_OrderSortsShownRowsWithoutPullingHiddenRows(t *testing.T) {
	m, fs, _ := searchModel(t, nil)
	fs.rows[1] = nil
	var hiddenLargest string
	for i := range 12 {
		hash := fmt.Sprintf("%040x", i+1)
		size := int64(2+i) << 30
		seeders := 100 - i
		if i == 11 {
			size = 50 << 30
			seeders = 1
			hiddenLargest = hash
		}
		fs.rows[1] = append(fs.rows[1], search.Result{
			Title:     fmt.Sprintf("Sample.Film.2014.1080p-G%02d", i),
			Hash:      hash,
			Size:      size,
			Seeders:   seeders,
			Indexers:  []string{"General"},
			IndexerID: 1,
			GUID:      fmt.Sprintf("g%02d", i),
		})
	}
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	m = hit(t, m, capOKey)
	assert.Contains(t, plain(m), "order: size")
	rows := m.shownRows()
	require.Len(t, rows, 10)
	assert.GreaterOrEqual(t, rows[0].Size, rows[len(rows)-1].Size)
	for _, r := range rows {
		assert.NotEqual(t, hiddenLargest, r.Hash, "hidden 50 GiB row does not enter section on O")
	}
}

func TestSearch_FilterHidesEmptyHeaders(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	m = hit(t, m, slashKey)
	m = typeText(m, "1080p")
	m = hit(t, m, enterKey)
	view := plain(m)
	assert.Contains(t, view, "── 1080p · 1 of 1 ──")
	assert.NotContains(t, view, "── 4K ·")
}

func TestSearch_FooterFitsAtWidth80(t *testing.T) {
	m, _, _ := searchModel(t, nil)
	m.width = 80
	m, _ = sendKey(m, sKey)
	m = typeText(m, "tt0000001")
	m = hit(t, m, enterKey)

	footer := ansi.Strip(m.renderFooter())
	assert.NotContains(t, footer, "\n", "footer stays one line at width 80")
	assert.Contains(t, footer, "[Enter] Add")
	assert.Contains(t, footer, "[Esc] Back")
}
