package search

import (
	"regexp"
	"strconv"
	"strings"
)

// Query is what the user typed: either free text for a title search, or an
// IMDb ID with an optional season and episode.
type Query struct {
	Text    string
	IMDbID  string
	Season  int
	Episode int
}

var (
	idQuery      = regexp.MustCompile(`^(tt\d{7,})(?:\s+[Ss](\d{1,2})(?:[Ee](\d{1,3}))?)?$`)
	episodeInput = regexp.MustCompile(`^[Ss](\d{1,2})(?:[Ee](\d{1,3}))?$`)
)

func ParseQuery(s string) Query {
	s = strings.TrimSpace(s)
	m := idQuery.FindStringSubmatch(strings.ToLower(s))
	if m == nil {
		return Query{Text: s}
	}
	q := Query{IMDbID: m[1]}
	q.Season, _ = strconv.Atoi(m[2])
	q.Episode, _ = strconv.Atoi(m[3])
	return q
}

// ParseEpisode reads the season prompt: "S02", "S02E05", or blank for the
// whole show. ok is false for anything else.
func ParseEpisode(s string) (season, episode int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, true
	}
	m := episodeInput.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	season, _ = strconv.Atoi(m[1])
	episode, _ = strconv.Atoi(m[2])
	return season, episode, season > 0
}

// Work is the work an ID query names. kind ("movie" or "show"), title and
// year come from a title lookup; with no lookup, a season in the query makes
// it a show. A movie drops any season the user typed.
func (q Query) Work(kind, title string, year int) Work {
	w := Work{Kind: Movie, Title: title, Year: year, IMDbID: q.IMDbID, Season: q.Season, Episode: q.Episode}
	if kind == "show" || (kind == "" && q.Season > 0) {
		w.Kind = Show
	} else {
		w.Season, w.Episode = 0, 0
	}
	return w
}
