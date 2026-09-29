package search

import (
	"slices"
	"sort"
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
	SortCached SortKey = iota
	SortSize
	SortSeeders
	SortResolution
)

var sortNames = []string{"cached", "size", "seeders", "resolution"}

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

// Sort orders rows in place by key, most wanted first. badge reports each
// row's cached state, for SortCached.
func Sort(rows []Result, key SortKey, badge func(Result) Badge) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch key {
		case SortSize:
			if a.Size != b.Size {
				return a.Size > b.Size
			}
		case SortResolution:
			if ra, rb := resolutionRank(a.Media.Resolution), resolutionRank(b.Media.Resolution); ra != rb {
				return ra > rb
			}
		case SortCached:
			if ca, cb := cachedRank(badge(a)), cachedRank(badge(b)); ca != cb {
				return ca < cb
			}
		}
		return a.Seeders > b.Seeders
	})
}

func cachedRank(b Badge) int {
	switch b {
	case BadgeCached, BadgeInLibrary:
		return 0
	case BadgeUncached:
		return 2
	default:
		return 1
	}
}

func resolutionRank(res string) int {
	switch res {
	case "2160p", "4k", "uhd":
		return 4
	case "1080p", "1080i":
		return 3
	case "720p":
		return 2
	case "576p", "480p":
		return 1
	default:
		return 0
	}
}
