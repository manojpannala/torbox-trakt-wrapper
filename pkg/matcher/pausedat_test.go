package matcher_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

func TestMatch_CarriesPausedAtForTheResumePrompt(t *testing.T) {
	paused := time.Date(2026, 9, 9, 21, 30, 0, 0, time.UTC)

	playback := []trakt.PlaybackItem{
		{
			ID: 101, Progress: 41.0, Type: "movie", PausedAt: paused,
			Movie: &trakt.Movie{Title: "Test Dream Heist", Year: 2010},
		},
		{
			ID: 201, Progress: 72.0, Type: "episode", PausedAt: paused,
			Show:    &trakt.Show{Title: "Test Crime Series"},
			Episode: &trakt.Episode{Season: 1, Number: 4},
		},
	}
	m := matcher.NewMatcher(nil, nil, playback)

	movie := m.MatchFile("Test.Dream.Heist.2010.1080p.mkv")
	assert.Equal(t, 41.0, movie.ProgressPercent)
	assert.Equal(t, paused, movie.PausedAt, "the modal says how long ago you stopped")

	episode := m.MatchFile("Test.Crime.Series.S01E04.1080p.mkv")
	assert.Equal(t, 72.0, episode.ProgressPercent)
	assert.Equal(t, paused, episode.PausedAt)
}
