package search

import (
	"slices"
	"sort"
	"strings"
)

type Badge int

const (
	BadgeUnknown Badge = iota
	BadgeResolving
	BadgeCached
	BadgeUncached
	BadgeInLibrary
)

func (b Badge) String() string {
	switch b {
	case BadgeResolving:
		return "…"
	case BadgeCached:
		return "●"
	case BadgeUncached:
		return "○"
	case BadgeInLibrary:
		return "■"
	default:
		return "?"
	}
}

type SortKey int

const (
	SortRank SortKey = iota
	SortSize
	SortSeeders
)

var sortNames = []string{"rank", "size", "seeders"}

func (k SortKey) String() string {
	if k < 0 || int(k) >= len(sortNames) {
		return sortNames[0]
	}
	return sortNames[k]
}

func (k SortKey) Next() SortKey {
	return (k + 1) % SortKey(len(sortNames))
}

// ParseSortKey reads a --sort value; ok is false for an unknown name.
func ParseSortKey(s string) (SortKey, bool) {
	if s == "cached" || s == "resolution" {
		return SortRank, true
	}
	i := slices.Index(sortNames, s)
	return SortKey(max(i, 0)), i >= 0
}

// Merge folds releases that share a hash into one row, keeping the best
// seeded copy and every indexer that listed it. Unresolved rows stay apart.
func Merge(rows []Result) []Result {
	out := make([]Result, 0, len(rows))
	at := make(map[string]int, len(rows))
	for _, r := range rows {
		id := r.Hash
		if id == "" {
			id = "\x00" + r.Key()
		}
		i, seen := at[id]
		if !seen {
			r.Indexers = slices.Clone(r.Indexers)
			at[id] = len(out)
			out = append(out, r)
			continue
		}
		indexers := out[i].Indexers
		for _, name := range r.Indexers {
			if !slices.Contains(indexers, name) {
				indexers = append(indexers, name)
			}
		}
		if r.Seeders > out[i].Seeders {
			r.Indexers = indexers
			out[i] = r
		} else {
			out[i].Indexers = indexers
		}
	}
	return out
}

// Prefs is what a search knows about what the user wants.
type Prefs struct {
	Language string   // Work.Language; "" skips the language key
	Episode  bool     // a single-episode search (Show with Episode > 0)
	Groups   []string // preferred_groups, compared case-insensitively
}

// Less reports whether a ranks above b. Keys in turn, each only breaking
// ties in the ones before it: language, episode, preferred group, stream
// tier, and seeders.
func (p Prefs) Less(a, b Result) bool {
	if p.Language != "" {
		if ta, tb := langTier(a.Lang, p.Language), langTier(b.Lang, p.Language); ta != tb {
			return ta < tb
		}
	}
	if p.Episode {
		if ea, eb := isSingleEpisode(a), isSingleEpisode(b); ea != eb {
			return ea
		}
	}
	if len(p.Groups) > 0 {
		if ga, gb := p.hasGroup(a), p.hasGroup(b); ga != gb {
			return ga
		}
	}
	if sa, sb := streamTier(a), streamTier(b); sa != sb {
		return sa < sb
	}
	return a.Seeders > b.Seeders
}

func langTier(l Lang, want string) int {
	if l != LangNone && l != LangMulti && strings.EqualFold(l.Code(), want) {
		return 0
	}
	if l == LangNone && strings.EqualFold(want, "en") {
		return 0
	}
	switch l {
	case LangMulti:
		return 1
	case LangNone:
		return 2
	default:
		return 3
	}
}

func (p Prefs) hasGroup(r Result) bool {
	if r.Media.ReleaseGroup == "" {
		return false
	}
	for _, g := range p.Groups {
		if strings.EqualFold(r.Media.ReleaseGroup, g) {
			return true
		}
	}
	return false
}

func sizeCeiling(r Result) int64 {
	if r.Pack {
		return 0
	}
	ep := isSingleEpisode(r)
	switch GroupOf(r.Media) {
	case Group4K:
		if ep {
			return 10 * gb
		}
		return 40 * gb
	case Group1080p:
		if ep {
			return 4 * gb
		}
		return 15 * gb
	case Group720p:
		if ep {
			return 2 * gb
		}
		return 6 * gb
	default:
		return 0
	}
}

func streamTier(r Result) int {
	words := splitWords(strings.ReplaceAll(strings.ToLower(r.Media.OriginalName), "h.265", "h265"))
	if slices.Contains(words, "remux") {
		return 2
	}
	if c := sizeCeiling(r); c > 0 && r.Size > c {
		return 2
	}
	for _, w := range words {
		switch w {
		case "hevc", "x265", "h265", "av1":
			return 0
		}
	}
	return 1
}

// Section is one group on screen, or the Late results.
type Section struct {
	Group Group
	Late  bool
	Rows  []Result // every row after drops; Rows[:Shown] are on screen
	Shown int
}

// Arrange groups rows (already merged and dropped), ranks each group with
// p.Less (stable, so ties keep merged order) and shows the top cap of each.
// all lifts every cap. Empty groups and groups that are off are left out.
func Arrange(rows []Result, caps map[Group]int, all bool, p Prefs) []Section {
	buckets := make(map[Group][]Result)
	for _, r := range rows {
		g := GroupOf(r.Media)
		if caps[g] <= 0 {
			continue
		}
		buckets[g] = append(buckets[g], r)
	}
	var out []Section
	for g := Group4K; g <= GroupUnknown; g++ {
		bucket := buckets[g]
		if len(bucket) == 0 {
			continue
		}
		sort.SliceStable(bucket, func(i, j int) bool { return p.Less(bucket[i], bucket[j]) })
		shown := min(caps[g], len(bucket))
		if all {
			shown = len(bucket)
		}
		out = append(out, Section{Group: g, Rows: bucket, Shown: shown})
	}
	return out
}

// ArrangeLate builds the Late results section: rows ranked by Group, then
// p.Less, showing LateCap. It returns false when rows is empty.
func ArrangeLate(rows []Result, caps map[Group]int, p Prefs) (Section, bool) {
	var kept []Result
	for _, r := range rows {
		if caps[GroupOf(r.Media)] > 0 {
			kept = append(kept, r)
		}
	}
	if len(kept) == 0 {
		return Section{}, false
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if ga, gb := GroupOf(kept[i].Media), GroupOf(kept[j].Media); ga != gb {
			return ga < gb
		}
		return p.Less(kept[i], kept[j])
	})
	return Section{Late: true, Rows: kept, Shown: min(LateCap, len(kept))}, true
}

// More shows the next step rows of s (its cap, or LateCap when Late) and
// returns the rows it brought on screen, in order.
func (s *Section) More(step int) []Result {
	if step <= 0 || s.Shown >= len(s.Rows) {
		return nil
	}
	next := min(s.Shown+step, len(s.Rows))
	added := s.Rows[s.Shown:next]
	s.Shown = next
	return added
}

// Order puts s.Rows[:s.Shown] in key order; rows past Shown keep rank order
// so More still brings in the next best.
func (s *Section) Order(key SortKey, p Prefs) {
	shown := s.Rows[:min(s.Shown, len(s.Rows))]
	rankLess := func(a, b Result) bool {
		if s.Late {
			if ga, gb := GroupOf(a.Media), GroupOf(b.Media); ga != gb {
				return ga < gb
			}
		}
		return p.Less(a, b)
	}
	sort.SliceStable(shown, func(i, j int) bool {
		a, b := shown[i], shown[j]
		switch key {
		case SortSize:
			if a.Size != b.Size {
				return a.Size > b.Size
			}
		case SortSeeders:
			if a.Seeders != b.Seeders {
				return a.Seeders > b.Seeders
			}
		}
		return rankLess(a, b)
	})
}

// Remove takes the shown row with key off screen without pulling a
// replacement up (Shown drops by one). It reports whether it found it.
func (s *Section) Remove(key string) bool {
	limit := min(s.Shown, len(s.Rows))
	for i := range limit {
		if s.Rows[i].Key() == key {
			s.Rows = slices.Delete(s.Rows, i, i+1)
			s.Shown--
			return true
		}
	}
	return false
}
