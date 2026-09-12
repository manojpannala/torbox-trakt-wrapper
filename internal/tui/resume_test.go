package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

func appWithEpisodePlayback() AppModel {
	playback := []trakt.PlaybackItem{{
		ID:       201,
		Progress: 72.0,
		Type:     "episode",
		Show:     &trakt.Show{Title: "Test Crime Series"},
		Episode:  &trakt.Episode{Season: 1, Number: 4},
	}}
	return AppModel{matcher: matcher.NewMatcher(nil, nil, playback)}
}

func TestResumePercentFor_FindsAnEpisodePosition(t *testing.T) {
	m := appWithEpisodePlayback()

	got := m.resumePercentFor(matcher.ParseMedia("Test.Crime.Series.S01E04.1080p.mkv"))

	assert.Equal(t, 72.0, got, "an episode inside a folder has never resumed")
}

func TestResumePercentFor_IsZeroForAnUnwatchedEpisode(t *testing.T) {
	m := appWithEpisodePlayback()

	got := m.resumePercentFor(matcher.ParseMedia("Test.Crime.Series.S01E09.1080p.mkv"))

	assert.Zero(t, got)
}
