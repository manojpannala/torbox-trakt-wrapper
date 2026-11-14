package search

import "github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"

type DropReason int

const (
	DropTitle DropReason = iota
	DropSeeders
	DropSource
	DropSize
	DropUnresolved
)

var dropReasonNames = []string{"title", "seeders", "source", "size", "unresolved"}

func (d DropReason) String() string {
	if d < 0 || int(d) >= len(dropReasonNames) {
		return "unknown"
	}
	return dropReasonNames[d]
}

const (
	mb = int64(1 << 20)
	gb = int64(1 << 30)
)

var badSourceWords = map[string]bool{
	"cam":      true,
	"hdcam":    true,
	"ts":       true,
	"tc":       true,
	"scr":      true,
	"screener": true,
}

func isSingleEpisode(r Result) bool {
	return !r.Pack && r.Media.Type == matcher.MediaTypeEpisode && r.Media.EpisodeEnd <= r.Media.Episode
}

func sizeFloor(r Result) int64 {
	ep := isSingleEpisode(r)
	switch GroupOf(r.Media) {
	case Group4K:
		if ep {
			return 300 * mb
		}
		return 1 * gb
	case Group1080p:
		if ep {
			return 100 * mb
		}
		return 300 * mb
	case Group720p:
		if ep {
			return 50 * mb
		}
		return 150 * mb
	default:
		return 0
	}
}

func hasBadSource(name string) bool {
	for _, w := range splitWords(name) {
		if badSourceWords[w] {
			return true
		}
	}
	return false
}

// Drop removes rows nobody wants, before ranking. inLibrary(hash) rows are
// never dropped. Order of checks: seeders, source, size; a row counts once,
// under the first reason that hits.
func Drop(rows []Result, inLibrary func(hash string) bool) (kept []Result, dropped map[DropReason]int) {
	kept = make([]Result, 0, len(rows))
	dropped = map[DropReason]int{}
	for _, r := range rows {
		if inLibrary != nil && r.Hash != "" && inLibrary(r.Hash) {
			kept = append(kept, r)
			continue
		}
		if r.Seeders == 0 {
			dropped[DropSeeders]++
			continue
		}
		if hasBadSource(r.Media.OriginalName) {
			dropped[DropSource]++
			continue
		}
		if floor := sizeFloor(r); floor > 0 && r.Size > 0 && r.Size < floor {
			dropped[DropSize]++
			continue
		}
		kept = append(kept, r)
	}
	return kept, dropped
}
