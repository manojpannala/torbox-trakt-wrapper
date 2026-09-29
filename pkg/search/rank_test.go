package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

func TestMerge_FoldsTheSameHashAcrossIndexers(t *testing.T) {
	rows := []search.Result{
		{Title: "low", Hash: sampleHex, Seeders: 3, Indexers: []string{"A"}},
		{Title: "other", Hash: "ffffffffffffffffffffffffffffffffffffffff", Seeders: 1, Indexers: []string{"A"}},
		{Title: "high", Hash: sampleHex, Seeders: 9, Indexers: []string{"B"}},
		{Title: "again", Hash: sampleHex, Seeders: 1, Indexers: []string{"A"}},
	}

	got := search.Merge(rows)

	require.Len(t, got, 2)
	assert.Equal(t, "high", got[0].Title, "the best seeded copy wins")
	assert.Equal(t, 9, got[0].Seeders)
	assert.Equal(t, []string{"A", "B"}, got[0].Indexers)
	assert.Equal(t, "other", got[1].Title)
	assert.Equal(t, []string{"A"}, rows[0].Indexers, "the input is not modified")
}

func TestMerge_KeepsUnresolvedRowsApart(t *testing.T) {
	rows := []search.Result{
		{Title: "one", IndexerID: 1, GUID: "g1"},
		{Title: "two", IndexerID: 1, GUID: "g2"},
		{Title: "one again", IndexerID: 1, GUID: "g1"},
	}

	got := search.Merge(rows)

	require.Len(t, got, 2)
	assert.Equal(t, "one", got[0].Title)
}

func TestSort(t *testing.T) {
	rows := func() []search.Result {
		return []search.Result{
			{Title: "a", Hash: "a", Seeders: 5, Size: 10, Media: matcher.ParsedMedia{Resolution: "720p"}},
			{Title: "b", Hash: "b", Seeders: 50, Size: 30, Media: matcher.ParsedMedia{Resolution: "1080p"}},
			{Title: "c", Hash: "c", Seeders: 20, Size: 20, Media: matcher.ParsedMedia{Resolution: "4k"}},
			{Title: "d", Hash: "d", Seeders: 1, Size: 5},
		}
	}
	badges := map[string]search.Badge{"a": search.BadgeCached, "b": search.BadgeUncached, "c": search.BadgeUnknown, "d": search.BadgeInLibrary}
	badge := func(r search.Result) search.Badge { return badges[r.Hash] }
	order := func(key search.SortKey) string {
		r := rows()
		search.Sort(r, key, badge)
		s := ""
		for _, x := range r {
			s += x.Title
		}
		return s
	}

	assert.Equal(t, "adcb", order(search.SortCached), "cached and in-library first, unknown next, uncached last")
	assert.Equal(t, "bcad", order(search.SortSize))
	assert.Equal(t, "bcad", order(search.SortSeeders))
	assert.Equal(t, "cbad", order(search.SortResolution))
}

func TestSortKey_CyclesAndParses(t *testing.T) {
	assert.Equal(t, search.SortSize, search.SortCached.Next())
	assert.Equal(t, search.SortCached, search.SortResolution.Next())
	assert.Equal(t, "resolution", search.SortResolution.String())

	key, ok := search.ParseSortKey("seeders")
	assert.True(t, ok)
	assert.Equal(t, search.SortSeeders, key)
	_, ok = search.ParseSortKey("bogus")
	assert.False(t, ok)
}

func TestBadgeGlyphs(t *testing.T) {
	assert.Equal(t, "●", search.BadgeCached.String())
	assert.Equal(t, "○", search.BadgeUncached.String())
	assert.Equal(t, "■", search.BadgeInLibrary.String())
	assert.Equal(t, "?", search.BadgeUnknown.String())
	assert.Equal(t, "…", search.BadgeResolving.String())
}
