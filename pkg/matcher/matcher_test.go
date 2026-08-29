package matcher_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

func TestMatcher_MovieMatching(t *testing.T) {
	movies := []trakt.WatchedMovie{
		{
			Plays:         1,
			LastWatchedAt: time.Now(),
			Movie: trakt.Movie{
				Title: "Test Movie Alpha",
				Year:  2023,
				IDs:   trakt.IDs{Trakt: 615024},
			},
		},
		{
			Plays:         2,
			LastWatchedAt: time.Now(),
			Movie: trakt.Movie{
				Title: "Test Sequel Gamma: Part Two",
				Year:  2024,
				IDs:   trakt.IDs{Trakt: 644558},
			},
		},
		{
			Plays:         1,
			LastWatchedAt: time.Now(),
			Movie: trakt.Movie{
				Title: "Test Indie Delta",
				Year:  2020,
				IDs:   trakt.IDs{Trakt: 501234},
			},
		},
	}

	playback := []trakt.PlaybackItem{
		{
			ID:       101,
			Progress: 45.5,
			Type:     "movie",
			Movie: &trakt.Movie{
				Title: "Test Dream Heist",
				Year:  2010,
				IDs:   trakt.IDs{Trakt: 16662},
			},
		},
	}

	m := matcher.NewMatcher(movies, nil, playback)

	res1 := m.MatchFile("Test.Movie.Alpha.2023.2160p.UHD.Remux.mkv")
	assert.Equal(t, matcher.StatusWatched, res1.Status)
	assert.Equal(t, "✓", res1.Badge)
	assert.Equal(t, 615024, res1.TraktID)
	assert.Equal(t, 1, res1.WatchedPlays)

	res2 := m.MatchFile("Test.Sequel.Gamma.Part.Two.2024.1080p.WEB-DL.mkv")
	assert.Equal(t, matcher.StatusWatched, res2.Status)
	assert.Equal(t, "✓", res2.Badge)
	assert.Equal(t, 644558, res2.TraktID)

	res3 := m.MatchFile("Test.Indie.Delta.2019.1080p.BluRay.mkv")
	assert.Equal(t, matcher.StatusWatched, res3.Status)
	assert.Equal(t, "✓", res3.Badge)
	assert.Equal(t, 501234, res3.TraktID)

	res4 := m.MatchFile("Test.Dream.Heist.2010.1080p.BluRay.mkv")
	assert.Equal(t, matcher.StatusInProgress, res4.Status)
	assert.Equal(t, "◐", res4.Badge)
	assert.Equal(t, 45.5, res4.ProgressPercent)
	assert.Equal(t, int64(101), res4.PlaybackID)

	res5 := m.MatchFile("Test.Avatar.Planet.2022.2160p.mkv")
	assert.Equal(t, matcher.StatusUnwatched, res5.Status)
	assert.Equal(t, "", res5.Badge)
}

func TestMatcher_TVShowMatching(t *testing.T) {
	shows := []trakt.WatchedShow{
		{
			Plays: 10,
			Show: trakt.Show{
				Title: "Test Crime Series",
				Year:  2008,
				IDs:   trakt.IDs{Trakt: 1388},
			},
			Seasons: []trakt.WatchedSeason{
				{
					Number: 1,
					Episodes: []trakt.WatchedEpisode{
						{Number: 1, Plays: 2},
						{Number: 2, Plays: 1},
						{Number: 3, Plays: 1},
					},
				},
			},
		},
	}

	playback := []trakt.PlaybackItem{
		{
			ID:       201,
			Progress: 72.0,
			Type:     "episode",
			Show: &trakt.Show{
				Title: "Test Crime Series",
				IDs:   trakt.IDs{Trakt: 1388},
			},
			Episode: &trakt.Episode{
				Season: 1,
				Number: 4,
			},
		},
	}

	m := matcher.NewMatcher(nil, shows, playback)

	res1 := m.MatchFile("Test.Crime.Series.S01E01.Pilot.1080p.mkv")
	assert.Equal(t, matcher.StatusWatched, res1.Status)
	assert.Equal(t, "✓", res1.Badge)
	assert.Equal(t, 2, res1.WatchedPlays)

	res2 := m.MatchFile("Test.Crime.Series.S01E04.Episode.Four.1080p.mkv")
	assert.Equal(t, matcher.StatusInProgress, res2.Status)
	assert.Equal(t, "◐", res2.Badge)
	assert.Equal(t, 72.0, res2.ProgressPercent)
	assert.Equal(t, int64(201), res2.PlaybackID)

	res3 := m.MatchFile("Test.Crime.Series.S01E05.Episode.Five.1080p.mkv")
	assert.Equal(t, matcher.StatusUnwatched, res3.Status)
	assert.Equal(t, "", res3.Badge)
}

func TestMatcher_MatchTorrentFilesAndFolderAggregation(t *testing.T) {
	shows := []trakt.WatchedShow{
		{
			Show: trakt.Show{Title: "Test Office SciFi", Year: 2022},
			Seasons: []trakt.WatchedSeason{
				{
					Number: 1,
					Episodes: []trakt.WatchedEpisode{
						{Number: 1, Plays: 1},
						{Number: 2, Plays: 1},
						{Number: 3, Plays: 1},
					},
				},
			},
		},
	}

	playback := []trakt.PlaybackItem{
		{
			Progress: 55.0,
			Show:     &trakt.Show{Title: "Test Office SciFi"},
			Episode:  &trakt.Episode{Season: 1, Number: 4},
		},
	}

	m := matcher.NewMatcher(nil, shows, playback)

	files := []torbox.TorrentFile{
		{Name: "Test.Office.SciFi.S01E01.mkv"},
		{Name: "Test.Office.SciFi.S01E02.mkv"},
		{Name: "Test.Office.SciFi.S01E03.mkv"},
		{Name: "Test.Office.SciFi.S01E04.mkv"},
		{Name: "Test.Office.SciFi.S01E05.mkv"},
	}

	results := m.MatchTorrentFiles(files)
	require.Len(t, results, 5)

	assert.Equal(t, matcher.StatusWatched, results[0].Status)
	assert.Equal(t, matcher.StatusWatched, results[1].Status)
	assert.Equal(t, matcher.StatusWatched, results[2].Status)
	assert.Equal(t, matcher.StatusInProgress, results[3].Status)
	assert.Equal(t, matcher.StatusUnwatched, results[4].Status)

	folderStatus := matcher.AggregateFolderStatus(results)
	assert.Equal(t, 5, folderStatus.TotalItems)
	assert.Equal(t, 3, folderStatus.WatchedItems)
	assert.Equal(t, 1, folderStatus.InProgressItems)
	assert.Equal(t, matcher.StatusInProgress, folderStatus.Status)
	assert.Equal(t, "◐", folderStatus.Badge)
	assert.Equal(t, "[3/5]", folderStatus.Summary)

	completeResults := results[:3]
	completeStatus := matcher.AggregateFolderStatus(completeResults)
	assert.Equal(t, matcher.StatusWatched, completeStatus.Status)
	assert.Equal(t, "✓", completeStatus.Badge)
	assert.Equal(t, "[3/3]", completeStatus.Summary)
}
