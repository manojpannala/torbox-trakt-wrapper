package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

var testPausedAt = time.Date(2026, 9, 9, 21, 30, 0, 0, time.UTC)

func appWithEpisodePlayback() AppModel {
	playback := []trakt.PlaybackItem{{
		ID:       201,
		Progress: 72.0,
		Type:     "episode",
		PausedAt: testPausedAt,
		Show:     &trakt.Show{Title: "Test Crime Series"},
		Episode:  &trakt.Episode{Season: 1, Number: 4},
	}}
	return AppModel{matcher: matcher.NewMatcher(nil, nil, playback)}
}

func TestResumeFor_FindsAnEpisodePosition(t *testing.T) {
	m := appWithEpisodePlayback()

	percent, pausedAt := m.resumeFor(matcher.ParseMedia("Test.Crime.Series.S01E04.1080p.mkv"))

	assert.Equal(t, 72.0, percent, "an episode inside a folder has never resumed")
	assert.Equal(t, testPausedAt, pausedAt)
}

func TestResumeFor_IsZeroForAnUnwatchedEpisode(t *testing.T) {
	m := appWithEpisodePlayback()

	percent, pausedAt := m.resumeFor(matcher.ParseMedia("Test.Crime.Series.S01E09.1080p.mkv"))

	assert.Zero(t, percent)
	assert.True(t, pausedAt.IsZero())
}
