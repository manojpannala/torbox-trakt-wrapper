package matcher

import (
	"fmt"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type MediaType string

const (
	MediaTypeMovie   MediaType = "movie"
	MediaTypeEpisode MediaType = "episode"
	MediaTypeUnknown MediaType = "unknown"
)

type WatchStatus int

const (
	StatusUnwatched WatchStatus = iota
	StatusInProgress
	StatusWatched
)

type ParsedMedia struct {
	OriginalName string    `json:"original_name"`
	CleanTitle   string    `json:"clean_title"`
	Type         MediaType `json:"type"`
	Year         int       `json:"year,omitempty"`
	Season       int       `json:"season,omitempty"`
	Episode      int       `json:"episode,omitempty"`
	EpisodeEnd   int       `json:"episode_end,omitempty"`
	Resolution   string    `json:"resolution,omitempty"`
	Source       string    `json:"source,omitempty"`
	Codec        string    `json:"codec,omitempty"`
	Audio        string    `json:"audio,omitempty"`
	HDR          string    `json:"hdr,omitempty"`
	ReleaseGroup string    `json:"release_group,omitempty"`
}

func (p ParsedMedia) DisplayTitle() string {
	if p.Type == MediaTypeEpisode {
		if p.EpisodeEnd > p.Episode {
			return fmt.Sprintf("%s S%02dE%02d-E%02d", p.CleanTitle, p.Season, p.Episode, p.EpisodeEnd)
		}
		if p.Season > 0 && p.Episode > 0 {
			return fmt.Sprintf("%s S%02dE%02d", p.CleanTitle, p.Season, p.Episode)
		}
		if p.Episode > 0 {
			return fmt.Sprintf("%s E%02d", p.CleanTitle, p.Episode)
		}
	}
	if p.Year > 0 {
		return fmt.Sprintf("%s (%d)", p.CleanTitle, p.Year)
	}
	return p.CleanTitle
}

type MatchResult struct {
	Parsed          ParsedMedia           `json:"parsed"`
	Status          WatchStatus           `json:"status"`
	Badge           string                `json:"badge"`
	ProgressPercent float64               `json:"progress_percent"`
	WatchedPlays    int                   `json:"watched_plays"`
	TraktID         int                   `json:"trakt_id,omitempty"`
	TraktTitle      string                `json:"trakt_title,omitempty"`
	TraktYear       int                   `json:"trakt_year,omitempty"`
	MatchedMovie    *trakt.WatchedMovie   `json:"matched_movie,omitempty"`
	MatchedShow     *trakt.WatchedShow    `json:"matched_show,omitempty"`
	MatchedEpisode  *trakt.WatchedEpisode `json:"matched_episode,omitempty"`
	PlaybackID      int64                 `json:"playback_id,omitempty"`
}

type FolderStatus struct {
	TotalItems      int         `json:"total_items"`
	WatchedItems    int         `json:"watched_items"`
	InProgressItems int         `json:"in_progress_items"`
	Status          WatchStatus `json:"status"`
	Badge           string      `json:"badge"`
	Summary         string      `json:"summary"`
}
