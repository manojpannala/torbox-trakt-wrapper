package matcher_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

func TestMatcher_CarriesTheMatchedWorksIMDbID(t *testing.T) {
	m := matcher.NewMatcher(
		[]trakt.WatchedMovie{{Plays: 1, Movie: trakt.Movie{Title: "Sample Film", Year: 2014, IDs: trakt.IDs{Trakt: 1, IMDB: "tt0000001"}}}},
		[]trakt.WatchedShow{{Plays: 1, Show: trakt.Show{Title: "Sample Show", Year: 2019, IDs: trakt.IDs{Trakt: 2, IMDB: "tt0000002"}}}},
		[]trakt.PlaybackItem{
			{ID: 1, Progress: 40, Type: "movie", Movie: &trakt.Movie{Title: "Paused Film", Year: 2020, IDs: trakt.IDs{Trakt: 3, IMDB: "tt0000003"}}},
			{ID: 2, Progress: 40, Type: "episode", Show: &trakt.Show{Title: "Paused Show", IDs: trakt.IDs{Trakt: 4, IMDB: "tt0000004"}},
				Episode: &trakt.Episode{Season: 1, Number: 2}},
		},
	)

	for name, want := range map[string]string{
		"Sample.Film.2014.1080p.BluRay.mkv":  "tt0000001",
		"Sample.Show.S02E05.1080p.WEB.mkv":   "tt0000002",
		"Paused.Film.2020.720p.mkv":          "tt0000003",
		"Paused.Show.S01E02.1080p.mkv":       "tt0000004",
		"Unknown.Film.2014.1080p.BluRay.mkv": "",
	} {
		assert.Equal(t, want, m.MatchFile(name).IMDbID, name)
	}
}
