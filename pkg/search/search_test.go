package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

var (
	yts     = search.Indexer{ID: 1, Name: "YTS", MovieIMDb: true}
	general = search.Indexer{ID: 2, Name: "General"}
	tvIDs   = search.Indexer{ID: 3, Name: "TVIDs", TVIMDb: true}
)

func TestPlan_MovieSendsTheIDOnlyWhereItIsSupported(t *testing.T) {
	w := search.Work{Kind: search.Movie, Title: "Sample Film", Year: 2014, IMDbID: "tt0000001"}

	reqs := search.Plan([]search.Indexer{yts, general}, w)

	assert.Equal(t, []search.Request{
		{IndexerID: 1, Indexer: "YTS", Type: "movie", Query: "{ImdbId:tt0000001}", Categories: []int{2000}, ByID: true},
		{IndexerID: 1, Indexer: "YTS", Type: "search", Query: "Sample Film 2014", Categories: []int{2000}},
		{IndexerID: 2, Indexer: "General", Type: "search", Query: "Sample Film 2014", Categories: []int{2000}},
	}, reqs)
}

func TestPlan_EpisodeIDSearchCarriesSeasonAndEpisode(t *testing.T) {
	w := search.Work{Kind: search.Show, Title: "Sample Show", IMDbID: "tt0000002", Season: 2, Episode: 5}

	reqs := search.Plan([]search.Indexer{yts, tvIDs}, w)

	require.Len(t, reqs, 3, "YTS has no TV ID search, so it only gets the text search")
	assert.Equal(t, search.Request{IndexerID: 1, Indexer: "YTS", Type: "search", Query: "Sample Show S02E05", Categories: []int{5000}}, reqs[0])
	assert.Equal(t, "{ImdbId:tt0000002}{Season:2}{Episode:5}", reqs[1].Query)
	assert.Equal(t, "tvsearch", reqs[1].Type)
	assert.Equal(t, "Sample Show S02E05", reqs[2].Query)
}

func TestPlan_SeasonAndWholeShowText(t *testing.T) {
	season := search.Plan([]search.Indexer{general}, search.Work{Kind: search.Show, Title: "Sample Show", Season: 2})
	whole := search.Plan([]search.Indexer{general}, search.Work{Kind: search.Show, Title: "Sample Show", Year: 2008})

	assert.Equal(t, "Sample Show S02", season[0].Query)
	assert.Equal(t, "Sample Show", whole[0].Query, "a show's year is left out; release names rarely carry it")
}

func TestPlan_TextQueryDropsPunctuation(t *testing.T) {
	w := search.Work{Kind: search.Movie, Title: "Agent’s Mission: Part – One & Two", Year: 2020}

	reqs := search.Plan([]search.Indexer{general}, w)

	assert.Equal(t, "Agents Mission Part One and Two 2020", reqs[0].Query)
}

func TestPlan_NoTitleMeansNoTextSearch(t *testing.T) {
	w := search.Work{Kind: search.Movie, IMDbID: "tt0000001"}

	reqs := search.Plan([]search.Indexer{yts, general}, w)

	require.Len(t, reqs, 1)
	assert.True(t, reqs[0].ByID)
}

func TestPlanRaw_SendsTheTextAsTypedToEveryIndexer(t *testing.T) {
	reqs := search.PlanRaw([]search.Indexer{yts, general}, "  some words  ")

	assert.Equal(t, []search.Request{
		{IndexerID: 1, Indexer: "YTS", Type: "search", Query: "some words", Raw: true},
		{IndexerID: 2, Indexer: "General", Type: "search", Query: "some words", Raw: true},
	}, reqs)
}

func TestParseQuery(t *testing.T) {
	cases := map[string]search.Query{
		"sample film":        {Text: "sample film"},
		"  tt0903747 ":       {IMDbID: "tt0903747"},
		"tt0903747 S02E05":   {IMDbID: "tt0903747", Season: 2, Episode: 5},
		"TT0903747 s2":       {IMDbID: "tt0903747", Season: 2},
		"tt123":              {Text: "tt123"},
		"tt0903747 extra":    {Text: "tt0903747 extra"},
		"tt0903747 S02E05x":  {Text: "tt0903747 S02E05x"},
		"tt12345678 S01E100": {IMDbID: "tt12345678", Season: 1, Episode: 100},
	}
	for in, want := range cases {
		assert.Equal(t, want, search.ParseQuery(in), in)
	}
}

func TestParseEpisode(t *testing.T) {
	cases := []struct {
		in              string
		season, episode int
		ok              bool
	}{
		{"", 0, 0, true},
		{"S02", 2, 0, true},
		{"s02e05", 2, 5, true},
		{" S1E1 ", 1, 1, true},
		{"S00", 0, 0, false},
		{"2", 0, 0, false},
		{"S02E", 0, 0, false},
	}
	for _, c := range cases {
		season, episode, ok := search.ParseEpisode(c.in)
		assert.Equal(t, c.ok, ok, c.in)
		if c.ok {
			assert.Equal(t, [2]int{c.season, c.episode}, [2]int{season, episode}, c.in)
		}
	}
}

func TestQueryWork(t *testing.T) {
	q := search.ParseQuery("tt0000001 S02E05")

	assert.Equal(t, search.Work{Kind: search.Show, Title: "Sample Show", Year: 2019, IMDbID: "tt0000001", Season: 2, Episode: 5},
		q.Work("show", "Sample Show", 2019))
	assert.Equal(t, search.Work{Kind: search.Movie, Title: "Sample Film", Year: 2014, IMDbID: "tt0000001"},
		q.Work("movie", "Sample Film", 2014), "a movie drops the season")
	assert.Equal(t, search.Work{Kind: search.Show, IMDbID: "tt0000001", Season: 2, Episode: 5},
		q.Work("", "", 0), "without a lookup the season says show")
	assert.Equal(t, search.Work{Kind: search.Movie, IMDbID: "tt0000001"},
		search.ParseQuery("tt0000001").Work("", "", 0))
}

func TestResultKeyAndPending(t *testing.T) {
	r := search.Result{IndexerID: 4, GUID: "guid-1"}
	assert.Equal(t, "4/guid-1", r.Key())
	assert.False(t, r.Pending(), "no hash and no way to resolve is not pending")
}
