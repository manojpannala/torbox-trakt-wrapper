package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

func keep(w search.Work, release string) (bool, bool) {
	return search.Keep(w, matcher.ParseMedia(release))
}

func TestKeep_Movie(t *testing.T) {
	w := search.Work{Kind: search.Movie, Title: "The Sample Film", Year: 2014}
	cases := map[string]bool{
		"The.Sample.Film.2014.1080p.BluRay.x264-GRP":    true,
		"Sample Film 2014 2160p UHD BluRay x265-GRP":    true,
		"Sample.Film.2015.720p.WEB-DL-GRP":              true,
		"Sample.Film.2016.720p.WEB-DL-GRP":              false,
		"Sample.Film.Returns.2014.1080p.WEB-DL-GRP":     false,
		"Other.Sample.Film.Story.2014.1080p-GRP":        false,
		"Sample.Film.S01E01.1080p.WEB-DL-GRP":           false,
		"www.Site.company - Sample Film (2014) 1080p":   true,
		"pany  Sample Film (2014) Tamil 1080p HQ HDRip": true,
		"one two three Sample Film (2014) 1080p":        false,
	}
	for release, want := range cases {
		got, pack := keep(w, release)
		assert.Equal(t, want, got, release)
		assert.False(t, pack, release)
	}
}

func TestKeep_MovieWithoutAKnownYearAcceptsAnyYear(t *testing.T) {
	w := search.Work{Kind: search.Movie, Title: "Sample Film"}

	got, _ := keep(w, "Sample.Film.1999.1080p-GRP")

	assert.True(t, got)
}

func TestKeep_Episode(t *testing.T) {
	w := search.Work{Kind: search.Show, Title: "Sample Show", Season: 2, Episode: 5}
	cases := []struct {
		release    string
		keep, pack bool
	}{
		{"Sample.Show.S02E05.1080p.WEB-DL-GRP", true, false},
		{"Sample Show S02E06 1080p WEB-DL-GRP", false, false},
		{"Sample.Show.S03E05.1080p-GRP", false, false},
		{"Sample.Show.S02.1080p.BluRay.x264-GRP", true, true},
		{"Sample Show Season 2 Complete 1080p", true, true},
		{"Sample.Show.S01-S05.Complete.1080p", true, true},
		{"Sample.Show.S03-S05.Complete.1080p", false, false},
		{"Sample Show Complete Series 1080p", true, true},
		{"Sample.Show.S04.1080p-GRP", false, false},
		{"Other.Sample.Show.Here.S02E05.1080p-GRP", false, false},
		{"Sample.Show.2014.1080p.BluRay-GRP", false, false},
	}
	for _, c := range cases {
		got, pack := keep(w, c.release)
		assert.Equal(t, c.keep, got, c.release)
		assert.Equal(t, c.pack, pack, c.release)
	}
}

func TestKeep_SeasonKeepsItsEpisodesAndPacks(t *testing.T) {
	w := search.Work{Kind: search.Show, Title: "Sample Show", Season: 2}

	ep, _ := keep(w, "Sample.Show.S02E09.1080p-GRP")
	other, _ := keep(w, "Sample.Show.S01E09.1080p-GRP")
	pack, isPack := keep(w, "Sample.Show.S02.1080p-GRP")

	assert.True(t, ep)
	assert.False(t, other)
	assert.True(t, pack)
	assert.True(t, isPack)
}

func TestKeep_WholeShowKeepsEverythingForIt(t *testing.T) {
	w := search.Work{Kind: search.Show, Title: "Sample Show"}

	ep, _ := keep(w, "Sample.Show.S07E01.1080p-GRP")
	pack, _ := keep(w, "Sample.Show.S03.1080p-GRP")
	other, _ := keep(w, "Different.Show.S03.1080p-GRP")

	assert.True(t, ep)
	assert.True(t, pack)
	assert.False(t, other)
}

func TestKeep_NoTitleKeepsNothing(t *testing.T) {
	got, _ := keep(search.Work{Kind: search.Movie}, "Anything.2014.1080p-GRP")

	assert.False(t, got)
}

func TestPrepare_FiltersTextResultsButNotIDOrRawResults(t *testing.T) {
	w := search.Work{Kind: search.Movie, Title: "Sample Film", Year: 2014}
	rows := []search.Result{
		{Title: "Sample.Film.2014.1080p.BluRay.x264-GRP"},
		{Title: "Unrelated.Name.2014.1080p-GRP"},
	}

	text := search.Prepare(search.Request{}, w, rows)
	byID := search.Prepare(search.Request{ByID: true}, w, rows)
	raw := search.Prepare(search.Request{Raw: true}, w, rows)

	require.Len(t, text, 1)
	assert.Equal(t, "1080p", text[0].Media.Resolution)
	assert.Len(t, byID, 2)
	assert.Len(t, raw, 2)
}

func TestPrepare_CleansReleaseNamesForDisplay(t *testing.T) {
	rows := []search.Result{{Title: "Sample.Film.2014\x1b[31m.1080p-GRP"}}

	got := search.Prepare(search.Request{Raw: true}, search.Work{}, rows)

	assert.NotContains(t, got[0].Title, "\x1b")
}

func TestPrepare_MarksPacks(t *testing.T) {
	w := search.Work{Kind: search.Show, Title: "Sample Show", Season: 2, Episode: 5}

	got := search.Prepare(search.Request{}, w, []search.Result{{Title: "Sample.Show.S02.1080p-GRP"}})

	require.Len(t, got, 1)
	assert.True(t, got[0].Pack)
}
