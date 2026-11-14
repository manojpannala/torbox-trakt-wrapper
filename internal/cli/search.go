package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search/prowlarr"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

var (
	searchRawFlag        bool
	searchJSONFlag       bool
	searchSortFlag       string
	searchCachedOnlyFlag bool
	searchAllFlag        bool
)

var searchCmd = &cobra.Command{
	Use:   "search <title | imdb-id [SxxEyy]>",
	Short: "Find a title, then its releases, through your Prowlarr",
	Long: `Search by title to list matching works with their IMDb IDs, then search
by IMDb ID (optionally with a season or episode, like "tt0000000 S02E05")
to list releases with their TorBox cached badge:

  ● cached   ○ not cached   ? unknown

Nothing is added; copy a magnet into "tt-wrapper add".`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetConfig()
		if !c.Search.Enabled() {
			return fmt.Errorf("search is off: set prowlarr_url and prowlarr_api_key under [search] in %s", c.Path())
		}
		sortKey, ok := search.ParseSortKey(searchSortFlag)
		if !ok {
			return fmt.Errorf("--sort must be one of rank, size, seeders")
		}
		caps, err := search.Caps(c.Search.Resolutions)
		if err != nil {
			return err
		}
		backend, err := prowlarr.New(c.Search.ProwlarrURL, c.Search.ProwlarrAPIKey, prowlarr.WithLogger(logger))
		if err != nil {
			return errors.New(prowlarr.Describe(err))
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		s := cliSearch{
			backend:    backend,
			titles:     trakt.NewClient(c.Trakt.ClientID, c.Trakt.ClientSecret, trakt.WithLogger(logger)),
			out:        cmd.OutOrStdout(),
			errOut:     cmd.ErrOrStderr(),
			sortKey:    sortKey,
			json:       searchJSONFlag,
			cachedOnly: searchCachedOnlyFlag,
			all:        searchAllFlag,
			caps:       caps,
			groups:     c.Search.PreferredGroups,
		}
		if c.TorBox.HasAuth() {
			s.cached = torbox.NewClient(c.TorBox.APIKey, torbox.WithLogger(logger))
		}
		text := strings.Join(args, " ")
		if searchRawFlag {
			return s.releases(ctx, nil, text)
		}
		q := search.ParseQuery(text)
		if q.IMDbID == "" {
			if c.Trakt.ClientID == "" {
				return fmt.Errorf("title search needs trakt client_id; search by IMDb ID or use --raw")
			}
			return s.titleHits(ctx, q.Text)
		}
		return s.releases(ctx, s.work(ctx, q), "")
	},
}

type cachedChecker interface {
	CheckCached(ctx context.Context, hashes []string) (map[string]bool, error)
}

type titleSearcher interface {
	SearchTitles(ctx context.Context, text string) ([]trakt.TitleHit, error)
	LookupIMDb(ctx context.Context, imdbID string) (*trakt.TitleHit, error)
}

type cliSearch struct {
	backend    search.Searcher
	titles     titleSearcher
	cached     cachedChecker
	out        io.Writer
	errOut     io.Writer
	sortKey    search.SortKey
	json       bool
	cachedOnly bool
	all        bool
	caps       map[search.Group]int
	groups     []string
}

func (s cliSearch) titleHits(ctx context.Context, text string) error {
	hits, err := s.titles.SearchTitles(ctx, text)
	if err != nil {
		return fmt.Errorf("trakt title search failed (try --raw): %w", err)
	}
	if s.json {
		return writeJSON(s.out, hits)
	}
	if len(hits) == 0 {
		_, _ = fmt.Fprintln(s.errOut, "No titles found. Try --raw to search releases by the text as typed.")
		return nil
	}
	w := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)
	for _, h := range hits {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", matcher.SanitizeDisplay(h.IDs.IMDB), h.Kind, titleYear(h.Title, h.Year))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(s.errOut, "\nList releases with: tt-wrapper search <imdb-id> [S02 | S02E05]")
	return nil
}

// work turns an ID query into the work it names, by asking Trakt for the
// title the text search needs. Without one, only ID searches run.
func (s cliSearch) work(ctx context.Context, q search.Query) *search.Work {
	hit, err := s.titles.LookupIMDb(ctx, q.IMDbID)
	switch {
	case err != nil:
		_, _ = fmt.Fprintf(s.errOut, "Trakt lookup failed, searching by ID only: %v\n", err)
	case hit == nil:
		_, _ = fmt.Fprintf(s.errOut, "Trakt has no title for %s, searching by ID only\n", q.IMDbID)
	default:
		w := q.Work(hit.Kind, hit.Title, hit.Year)
		w.Language = hit.Language
		return &w
	}
	w := q.Work("", "", 0)
	return &w
}

type indexerTally struct {
	name   string
	ok     bool
	count  int
	failed error
}

// releases searches every indexer at once and prints what comes back. A nil
// w means a raw search for text.
func (s cliSearch) releases(ctx context.Context, w *search.Work, text string) error {
	indexers, err := s.backend.Indexers(ctx)
	if err != nil {
		return errors.New(prowlarr.Describe(err))
	}
	var reqs []search.Request
	if w == nil {
		reqs = search.PlanRaw(indexers, text)
	} else {
		reqs = search.Plan(indexers, *w)
	}
	if len(reqs) == 0 {
		return errors.New("none of Prowlarr's enabled torrent indexers can run this search")
	}

	var mu sync.Mutex
	var rows []search.Result
	sumTally := search.Tally{Dropped: map[search.DropReason]int{}}
	tally := make(map[int]*indexerTally, len(indexers))
	for _, req := range reqs {
		tally[req.IndexerID] = &indexerTally{name: req.Indexer}
	}
	var wk search.Work
	if w != nil {
		wk = *w
	}
	var wg sync.WaitGroup
	for _, req := range reqs {
		wg.Go(func() {
			got, err := s.backend.Search(ctx, req)
			mu.Lock()
			defer mu.Unlock()
			t := tally[req.IndexerID]
			if err != nil {
				t.failed = err
				return
			}
			sumTally.Received += len(got)
			kept, filtered := search.Prepare(req, wk, got)
			sumTally.Dropped[search.DropTitle] += filtered
			t.ok = true
			t.count += len(kept)
			rows = append(rows, kept...)
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, t := range tally {
		if errors.Is(t.failed, search.ErrUnauthorized) {
			return errors.New(prowlarr.Describe(t.failed))
		}
	}
	s.printTally(reqs, tally)

	rows = search.Merge(rows)
	sumTally.Merged = len(rows)
	rows, dropped := search.Drop(rows, func(string) bool { return false })
	for reason, n := range dropped {
		sumTally.Dropped[reason] += n
	}

	prefs := search.Prefs{Groups: s.groups}
	if w != nil {
		prefs.Language = w.Language
		prefs.Episode = w.Kind == search.Show && w.Episode > 0
	}
	sections := search.Arrange(rows, s.caps, s.all, prefs)
	if s.sortKey != search.SortRank {
		for i := range sections {
			sections[i].Order(s.sortKey, prefs)
		}
	}

	unresolved := s.resolveSections(ctx, sections)
	sumTally.Dropped[search.DropUnresolved] += unresolved
	if s.sortKey != search.SortRank {
		for i := range sections {
			sections[i].Order(s.sortKey, prefs)
		}
	}

	shown := shownRows(sections)
	cached := s.check(ctx, shown)
	badge := func(r search.Result) search.Badge {
		switch v, ok := cached[r.Hash]; {
		case !ok:
			return search.BadgeUnknown
		case v:
			return search.BadgeCached
		default:
			return search.BadgeUncached
		}
	}

	if s.json {
		out := make([]releaseJSON, 0, len(shown))
		for _, sec := range sections {
			for _, r := range sec.Rows[:sec.Shown] {
				if s.cachedOnly && badge(r) != search.BadgeCached {
					continue
				}
				j := releaseJSON{
					Result:     r,
					Magnet:     search.Magnet(r.Hash, ""),
					Resolution: r.Media.Resolution,
					Group:      sec.Group.Key(),
					Language:   r.Lang.Name(),
				}
				if v, ok := cached[r.Hash]; ok {
					j.Cached = &v
				}
				out = append(out, j)
			}
		}
		logger.Debug("search summary", sumTally.Attrs(sections)...)
		return writeJSON(s.out, out)
	}

	printedTotal := 0
	for _, sec := range sections {
		var printable []search.Result
		for _, r := range sec.Rows[:sec.Shown] {
			if s.cachedOnly && badge(r) != search.BadgeCached {
				continue
			}
			printable = append(printable, r)
		}
		if len(printable) == 0 {
			continue
		}
		printedTotal += len(printable)
		_, _ = fmt.Fprintf(s.out, "── %s · %d of %d ──\n", sec.Group.Header(), len(printable), len(sec.Rows))
		tw := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)
		for _, r := range printable {
			pack := ""
			if r.Pack {
				pack = " [pack]"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s%s\n",
				badge(r), dash(r.Lang.Label()), formatBytes(r.Size), r.Seeders,
				strings.Join(r.Indexers, ","), r.Title, pack)
			_, _ = fmt.Fprintf(tw, "\t\t\t\t\t%s\n", search.Magnet(r.Hash, ""))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if printedTotal == 0 {
		_, _ = fmt.Fprintln(s.errOut, "No releases found. Try --raw with different words.")
	}
	if unresolved > 0 {
		_, _ = fmt.Fprintf(s.errOut, "%d releases without a hash were left out; the TUI resolves more on demand.\n", unresolved)
	}
	logger.Debug("search summary", sumTally.Attrs(sections)...)
	return nil
}

type releaseJSON struct {
	search.Result
	Cached     *bool  `json:"cached"`
	Magnet     string `json:"magnet"`
	Resolution string `json:"resolution,omitempty"`
	Group      string `json:"group"`
	Language   string `json:"language,omitempty"`
}

// printTally reports each searched indexer once, in plan order.
func (s cliSearch) printTally(reqs []search.Request, tally map[int]*indexerTally) {
	parts := make([]string, 0, len(tally))
	seen := map[int]bool{}
	for _, req := range reqs {
		if seen[req.IndexerID] {
			continue
		}
		seen[req.IndexerID] = true
		t := tally[req.IndexerID]
		if t.ok {
			parts = append(parts, fmt.Sprintf("%s ✓ %d", t.name, t.count))
		} else {
			parts = append(parts, fmt.Sprintf("%s ✗ %s", t.name, prowlarr.Describe(t.failed)))
		}
	}
	_, _ = fmt.Fprintln(s.errOut, strings.Join(parts, " · "))
}

func shownRows(sections []search.Section) []search.Result {
	var out []search.Result
	for _, sec := range sections {
		out = append(out, sec.Rows[:sec.Shown]...)
	}
	return out
}

// resolveSections fetches hashes for the shown releases that came without
// one, removes any still without a hash, and folds duplicate resolved hashes.
func (s cliSearch) resolveSections(ctx context.Context, sections []search.Section) int {
	shown := shownRows(sections)
	resolved := map[string]string{}
	r := search.NewResolver()
	for batch := r.Next(shown, "", false); len(batch) > 0; batch = r.Next(shown, "", false) {
		hashes := make([]string, len(batch))
		errs := make([]error, len(batch))
		var wg sync.WaitGroup
		for i, row := range batch {
			wg.Go(func() { hashes[i], errs[i] = row.Resolve(ctx) })
		}
		wg.Wait()
		for i, row := range batch {
			r.Done(row, errs[i])
			key := row.Key()
			resolved[key] = hashes[i]
			for j := range shown {
				if shown[j].Key() == key {
					shown[j].Hash = hashes[i]
				}
			}
		}
	}

	unresolved := 0
	for si := range sections {
		sec := &sections[si]
		for i := 0; i < sec.Shown; {
			key := sec.Rows[i].Key()
			if h, ok := resolved[key]; ok {
				sec.Rows[i].Hash = h
			}
			if sec.Rows[i].Hash == "" {
				sec.Remove(key)
				unresolved++
				continue
			}
			i++
		}
	}

	type pos struct{ sec, row int }
	seen := map[string]pos{}
	for si := range sections {
		sec := &sections[si]
		for i := 0; i < sec.Shown; {
			cur := sec.Rows[i]
			firstPos, ok := seen[cur.Hash]
			if !ok {
				seen[cur.Hash] = pos{si, i}
				i++
				continue
			}
			first := &sections[firstPos.sec].Rows[firstPos.row]
			indexers := slices.Clone(first.Indexers)
			for _, name := range cur.Indexers {
				if !slices.Contains(indexers, name) {
					indexers = append(indexers, name)
				}
			}
			if cur.Seeders > first.Seeders {
				cur.Indexers = indexers
				*first = cur
			} else {
				first.Indexers = indexers
			}
			// By index: after the copy above, first may carry cur's key.
			sec.Rows = slices.Delete(sec.Rows, i, i+1)
			sec.Shown--
		}
	}
	return unresolved
}

// check asks TorBox which hashes are cached, paced by the badge checker.
// A failure leaves the badges unknown rather than failing the search.
func (s cliSearch) check(ctx context.Context, rows []search.Result) map[string]bool {
	out := map[string]bool{}
	if s.cached == nil {
		return out
	}
	hashes := make([]string, len(rows))
	for i, r := range rows {
		hashes[i] = r.Hash
	}
	c := search.NewChecker()
	c.Enqueue(hashes, time.Now())
	for {
		batch, wait := c.Next(time.Now())
		if batch == nil && wait == 0 {
			break
		}
		if batch == nil {
			if !sleepCtx(ctx, wait) {
				break
			}
			continue
		}
		got, err := s.cached.CheckCached(ctx, batch)
		switch {
		case err == nil:
			c.Done(batch, got, time.Now())
			for h, v := range got {
				out[h] = v
			}
		case torbox.IsRateLimited(err):
			c.Limited(batch, torbox.RetryAfter(err), time.Now())
			_, _ = fmt.Fprintf(s.errOut, "TorBox rate limit, retrying in %s\n", c.RetryingIn(time.Now()).Round(time.Second))
		default:
			c.Failed()
			_, _ = fmt.Fprintf(s.errOut, "TorBox cached check failed; badges show ?: %v\n", err)
		}
	}
	return out
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func titleYear(title string, year int) string {
	title = matcher.SanitizeDisplay(title)
	if year > 0 {
		return fmt.Sprintf("%s (%d)", title, year)
	}
	return title
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func init() {
	searchCmd.Flags().BoolVar(&searchRawFlag, "raw", false, "send the text to Prowlarr as typed, with no Trakt step or title filter")
	searchCmd.Flags().BoolVar(&searchJSONFlag, "json", false, "print JSON")
	searchCmd.Flags().StringVar(&searchSortFlag, "sort", "rank", "sort releases inside each group by rank, size or seeders")
	searchCmd.Flags().BoolVar(&searchCachedOnlyFlag, "cached-only", false, "print only releases cached on TorBox")
	searchCmd.Flags().BoolVar(&searchAllFlag, "all", false, "show and check every release, not just the top of each group")
	rootCmd.AddCommand(searchCmd)
}
