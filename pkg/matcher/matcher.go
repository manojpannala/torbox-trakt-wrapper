package matcher

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

var (
	nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9\s]+`)
	whitespaceRegex      = regexp.MustCompile(`\s+`)
)

type Matcher struct {
	mu                sync.RWMutex
	moviesByTitleYear map[string]*trakt.WatchedMovie
	moviesByTitle     map[string][]*trakt.WatchedMovie
	showsByTitle      map[string]*trakt.WatchedShow
	playbackByMovie   map[string]*trakt.PlaybackItem
	playbackByEpisode map[string]*trakt.PlaybackItem
	playbackByTraktID map[int]*trakt.PlaybackItem
}

func NewMatcher(movies []trakt.WatchedMovie, shows []trakt.WatchedShow, playback []trakt.PlaybackItem) *Matcher {
	m := &Matcher{}
	m.UpdateCatalog(movies, shows, playback)
	return m
}

func (m *Matcher) UpdateCatalog(movies []trakt.WatchedMovie, shows []trakt.WatchedShow, playback []trakt.PlaybackItem) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.moviesByTitleYear = make(map[string]*trakt.WatchedMovie, len(movies))
	m.moviesByTitle = make(map[string][]*trakt.WatchedMovie, len(movies))
	m.showsByTitle = make(map[string]*trakt.WatchedShow, len(shows))
	m.playbackByMovie = make(map[string]*trakt.PlaybackItem, len(playback))
	m.playbackByEpisode = make(map[string]*trakt.PlaybackItem, len(playback))
	m.playbackByTraktID = make(map[int]*trakt.PlaybackItem, len(playback))

	for i := range movies {
		movie := &movies[i]
		normTitle := NormalizeTitle(movie.Movie.Title)
		if normTitle == "" {
			continue
		}

		if movie.Movie.Year > 0 {
			key := fmt.Sprintf("%s:%d", normTitle, movie.Movie.Year)
			m.moviesByTitleYear[key] = movie
		}

		m.moviesByTitle[normTitle] = append(m.moviesByTitle[normTitle], movie)
	}

	for i := range shows {
		show := &shows[i]
		normTitle := NormalizeTitle(show.Show.Title)
		if normTitle == "" {
			continue
		}
		m.showsByTitle[normTitle] = show
	}

	for i := range playback {
		item := &playback[i]
		if item.Movie != nil {
			normTitle := NormalizeTitle(item.Movie.Title)
			if item.Movie.Year > 0 {
				key := fmt.Sprintf("%s:%d", normTitle, item.Movie.Year)
				m.playbackByMovie[key] = item
			}
			m.playbackByMovie[normTitle] = item
			if item.Movie.IDs.Trakt > 0 {
				m.playbackByTraktID[item.Movie.IDs.Trakt] = item
			}
		} else if item.Show != nil && item.Episode != nil {
			normShow := NormalizeTitle(item.Show.Title)
			key := fmt.Sprintf("%s:%d:%d", normShow, item.Episode.Season, item.Episode.Number)
			m.playbackByEpisode[key] = item
			if item.Episode.IDs.Trakt > 0 {
				m.playbackByTraktID[item.Episode.IDs.Trakt] = item
			}
		}
	}
}

func (m *Matcher) MatchFile(filename string) MatchResult {
	parsed := ParseMedia(filename)
	return m.MatchParsed(parsed)
}

func (m *Matcher) MatchParsed(parsed ParsedMedia) MatchResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := MatchResult{
		Parsed: parsed,
		Status: StatusUnwatched,
		Badge:  "",
	}

	normTitle := NormalizeTitle(parsed.CleanTitle)
	if normTitle == "" {
		return result
	}

	if parsed.Type == MediaTypeEpisode {
		m.matchEpisode(&result, normTitle, parsed)
	} else {
		m.matchMovie(&result, normTitle, parsed)
	}

	return result
}

func (m *Matcher) matchMovie(result *MatchResult, normTitle string, parsed ParsedMedia) {
	var matchedMovie *trakt.WatchedMovie

	if parsed.Year > 0 {
		key := fmt.Sprintf("%s:%d", normTitle, parsed.Year)
		if movie, ok := m.moviesByTitleYear[key]; ok {
			matchedMovie = movie
		} else {
			for diff := -1; diff <= 1; diff += 2 {
				tolKey := fmt.Sprintf("%s:%d", normTitle, parsed.Year+diff)
				if movie, ok := m.moviesByTitleYear[tolKey]; ok {
					matchedMovie = movie
					break
				}
			}
		}
	}

	if matchedMovie == nil {
		if list, ok := m.moviesByTitle[normTitle]; ok && len(list) == 1 {
			matchedMovie = list[0]
		}
	}

	if matchedMovie != nil {
		result.Status = StatusWatched
		result.Badge = "✓"
		result.WatchedPlays = matchedMovie.Plays
		result.TraktID = matchedMovie.Movie.IDs.Trakt
		result.TraktTitle = matchedMovie.Movie.Title
		result.TraktYear = matchedMovie.Movie.Year
		result.MatchedMovie = matchedMovie
	}

	var pb *trakt.PlaybackItem
	if parsed.Year > 0 {
		key := fmt.Sprintf("%s:%d", normTitle, parsed.Year)
		pb = m.playbackByMovie[key]
	}
	if pb == nil {
		pb = m.playbackByMovie[normTitle]
	}

	if pb != nil && pb.Progress > 0 && pb.Progress < 90 {
		result.Status = StatusInProgress
		result.Badge = "◐"
		result.ProgressPercent = pb.Progress
		result.PlaybackID = pb.ID
		if result.TraktID == 0 && pb.Movie != nil {
			result.TraktID = pb.Movie.IDs.Trakt
			result.TraktTitle = pb.Movie.Title
			result.TraktYear = pb.Movie.Year
		}
	}
}

func (m *Matcher) matchEpisode(result *MatchResult, normTitle string, parsed ParsedMedia) {
	show, ok := m.showsByTitle[normTitle]
	if !ok {
		for k, s := range m.showsByTitle {
			if stripArticles(k) == stripArticles(normTitle) {
				show = s
				break
			}
		}
	}

	if show != nil {
		result.TraktTitle = show.Show.Title
		result.TraktYear = show.Show.Year
		result.TraktID = show.Show.IDs.Trakt
		result.MatchedShow = show

		seasonNum := parsed.Season
		if seasonNum == 0 {
			seasonNum = 1
		}

		for _, season := range show.Seasons {
			if season.Number == seasonNum {
				for _, ep := range season.Episodes {
					if ep.Number == parsed.Episode {
						result.Status = StatusWatched
						result.Badge = "✓"
						result.WatchedPlays = ep.Plays
						result.MatchedEpisode = &ep
						break
					}
				}
				break
			}
		}
	}

	seasonNum := parsed.Season
	if seasonNum == 0 {
		seasonNum = 1
	}
	key := fmt.Sprintf("%s:%d:%d", normTitle, seasonNum, parsed.Episode)
	if pb, ok := m.playbackByEpisode[key]; ok && pb.Progress > 0 && pb.Progress < 90 {
		result.Status = StatusInProgress
		result.Badge = "◐"
		result.ProgressPercent = pb.Progress
		result.PlaybackID = pb.ID
	}
}

func (m *Matcher) MatchTorrentFiles(files []torbox.TorrentFile) []MatchResult {
	results := make([]MatchResult, len(files))
	for i, f := range files {
		results[i] = m.MatchFile(f.Name)
	}
	return results
}

func AggregateFolderStatus(results []MatchResult) FolderStatus {
	var total, watched, inProgress int

	for _, res := range results {
		total++
		switch res.Status {
		case StatusWatched:
			watched++
		case StatusInProgress:
			inProgress++
		}
	}

	status := FolderStatus{
		TotalItems:      total,
		WatchedItems:    watched,
		InProgressItems: inProgress,
		Status:          StatusUnwatched,
		Badge:           "",
		Summary:         fmt.Sprintf("[%d/%d]", watched, total),
	}

	if total == 0 {
		return status
	}

	if watched == total {
		status.Status = StatusWatched
		status.Badge = "✓"
	} else if watched > 0 || inProgress > 0 {
		status.Status = StatusInProgress
		status.Badge = "◐"
	}

	return status
}

func NormalizeTitle(title string) string {
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, "&", " and ")
	s = nonAlphanumericRegex.ReplaceAllString(s, " ")
	s = whitespaceRegex.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func stripArticles(s string) string {
	s = strings.TrimPrefix(s, "the ")
	s = strings.TrimPrefix(s, "a ")
	s = strings.TrimPrefix(s, "an ")
	return strings.TrimSpace(s)
}
