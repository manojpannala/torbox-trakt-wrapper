package search

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

// maxLeftoverWords is how much of a cut-down site prefix, like "pany" left
// from "www.site.company -", may sit before the title.
const maxLeftoverWords = 2

// packSuffix finds the season-pack markers the release parser leaves on the
// title, in normalised form: "s02", "s01 s05", "season 2", "complete".
var packSuffix = regexp.MustCompile(`^(.*?)\s+(?:s(\d{1,2})(?:\s+s(\d{1,2}))?|season\s+(\d{1,2})(?:\s+(\d{1,2}))?|(?:complete\s+)?series|complete)$`)

// Prepare parses each release for req, drops text results that aren't for
// w, marks season packs, and cleans the name for display.
func Prepare(req Request, w Work, rows []Result) []Result {
	out := make([]Result, 0, len(rows))
	for _, r := range rows {
		r.Media = matcher.ParseMedia(r.Title)
		r.Title = matcher.SanitizeDisplay(r.Title)
		if !req.ByID && !req.Raw {
			keep, pack := Keep(w, r.Media)
			if !keep {
				continue
			}
			r.Pack = pack
		}
		out = append(out, r)
	}
	return out
}

// Keep decides whether a text-search release is for w, and whether it is a
// season pack.
func Keep(w Work, p matcher.ParsedMedia) (keep, pack bool) {
	want := normalize(w.Title)
	got := normalize(p.CleanTitle)
	if want == "" {
		return false, false
	}

	if w.Kind == Movie {
		if p.Type == matcher.MediaTypeEpisode || !titleMatches(want, got) {
			return false, false
		}
		if w.Year > 0 && (p.Year < w.Year-1 || p.Year > w.Year+1) {
			return false, false
		}
		return true, false
	}

	if p.Type == matcher.MediaTypeEpisode {
		if !titleMatches(want, got) {
			return false, false
		}
		if w.Season > 0 && p.Season != w.Season {
			return false, false
		}
		if w.Episode > 0 && p.Episode != w.Episode {
			return false, false
		}
		return true, false
	}

	title, first, last, ok := seasonPack(got)
	if !ok || !titleMatches(want, title) {
		return false, false
	}
	if w.Season > 0 && first > 0 && (w.Season < first || w.Season > last) {
		return false, false
	}
	return true, true
}

// seasonPack splits a normalised pack title into the show title and the
// seasons it covers; first is 0 for a complete-series pack.
func seasonPack(got string) (title string, first, last int, ok bool) {
	m := packSuffix.FindStringSubmatch(got)
	if m == nil {
		return "", 0, 0, false
	}
	switch {
	case m[2] != "":
		first, _ = strconv.Atoi(m[2])
		last = first
		if m[3] != "" {
			last, _ = strconv.Atoi(m[3])
		}
	case m[4] != "":
		first, _ = strconv.Atoi(m[4])
		last = first
		if m[5] != "" {
			last, _ = strconv.Atoi(m[5])
		}
	}
	return m[1], first, last, true
}

// titleMatches accepts got when it ends with want as whole words, after at
// most maxLeftoverWords of leftover prefix.
func titleMatches(want, got string) bool {
	if got == want {
		return true
	}
	prefix, ok := strings.CutSuffix(got, " "+want)
	return ok && len(strings.Fields(prefix)) <= maxLeftoverWords
}

func normalize(title string) string {
	s := matcher.NormalizeTitle(title)
	for _, article := range []string{"the ", "a ", "an "} {
		s = strings.TrimPrefix(s, article)
	}
	return s
}
