package search

import (
	"errors"
	"sort"
	"time"
)

// The resolver's budget per search. Every resolve makes the backend fetch
// from the indexer's site, so few run at once and few run in total.
const (
	ResolveParallel = 2
	ResolveUpFront  = 10
	ResolveBudget   = 20
	ResolveTimeout  = 10 * time.Second
)

// Resolver picks which pending releases to resolve for one search. Like
// Checker it does no I/O and is not safe for concurrent use.
type Resolver struct {
	started  int
	inFlight int
	tried    map[string]bool
	limited  map[int]bool
}

func NewResolver() *Resolver {
	return &Resolver{tried: map[string]bool{}, limited: map[int]bool{}}
}

// Next returns the releases to resolve now. The ResolveUpFront best-seeded
// go without asking; want, the row under the cursor, may take the total to
// ResolveBudget; force, an Enter on want, resolves it whatever the budget.
func (r *Resolver) Next(rows []Result, want string, force bool) []Result {
	var out []Result
	start := func(row Result) {
		r.tried[row.Key()] = true
		r.started++
		r.inFlight++
		out = append(out, row)
	}

	var candidates []Result
	for _, row := range rows {
		if row.Pending() && !r.tried[row.Key()] && !r.limited[row.IndexerID] {
			candidates = append(candidates, row)
		}
	}
	if want != "" {
		for _, row := range candidates {
			if row.Key() == want && (force || (r.inFlight < ResolveParallel && r.started < ResolveBudget)) {
				start(row)
				break
			}
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Seeders > candidates[j].Seeders })
	for _, row := range candidates {
		if r.inFlight >= ResolveParallel || r.started >= ResolveUpFront {
			break
		}
		if !r.tried[row.Key()] {
			start(row)
		}
	}
	return out
}

// Done ends one resolve. A rate-limited indexer gets no more resolves in
// this search.
func (r *Resolver) Done(row Result, err error) {
	r.inFlight--
	if errors.Is(err, ErrRateLimited) {
		r.limited[row.IndexerID] = true
	}
}

// Limited reports whether an indexer stopped this search's resolves.
func (r *Resolver) Limited(indexerID int) bool {
	return r.limited[indexerID]
}

// Busy reports whether any resolve is still out.
func (r *Resolver) Busy() bool {
	return r.inFlight > 0
}
