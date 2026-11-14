package search_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

func rankRow(name string, size int64, seeders int, lang search.Lang, pack bool, id string) search.Result {
	return search.Result{
		Title:     name,
		Hash:      id,
		Size:      size,
		Seeders:   seeders,
		IndexerID: 1,
		GUID:      id,
		Pack:      pack,
		Lang:      lang,
		Media:     matcher.ParseMedia(name),
	}
}

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

func TestPrefsLess_Language(t *testing.T) {
	const g = int64(2 << 30)
	ta := rankRow("Sample.Film.2014.1080p.Tamil.x264-GRP", g, 10, search.LangTamil, false, "ta")
	multi := rankRow("Sample.Film.2014.1080p.Multi.x264-GRP", g, 10, search.LangMulti, false, "multi")
	none := rankRow("Sample.Film.2014.1080p.x264-GRP", g, 10, search.LangNone, false, "none")
	hi := rankRow("Sample.Film.2014.1080p.Hindi.x264-GRP", g, 10, search.LangHindi, false, "hi")
	en := rankRow("Sample.Film.2014.1080p.English.x264-GRP", g, 10, search.LangEnglish, false, "en")

	pTa := search.Prefs{Language: "ta"}
	assert.True(t, pTa.Less(ta, multi))
	assert.True(t, pTa.Less(multi, none))
	assert.True(t, pTa.Less(none, hi))
	assert.False(t, pTa.Less(hi, ta))

	pEn := search.Prefs{Language: "en"}
	assert.False(t, pEn.Less(none, en))
	assert.False(t, pEn.Less(en, none))
	assert.True(t, pEn.Less(none, hi))
	assert.True(t, pEn.Less(en, hi))

	taLow := rankRow("Sample.Film.2014.1080p.Tamil.x264-GRP", g, 5, search.LangTamil, false, "ta")
	hiHigh := rankRow("Sample.Film.2014.1080p.Hindi.x264-GRP", g, 500, search.LangHindi, false, "hi")
	assert.True(t, pTa.Less(taLow, hiHigh), "language beats seeders")
	assert.True(t, (search.Prefs{}).Less(hiHigh, taLow), "empty language skips language key so seeders decide")
}

func TestPrefsLess_EpisodeAndGroup(t *testing.T) {
	const g = int64(2 << 30)
	ep := rankRow("Sample.Show.S01E02.1080p.x264-GRP", g, 10, search.LangNone, false, "ep")
	pack := rankRow("Sample.Show.S01.1080p.x264-GRP", g, 10, search.LangNone, true, "pack")
	assert.True(t, (search.Prefs{Episode: true}).Less(ep, pack))
	assert.False(t, (search.Prefs{}).Less(ep, pack))

	grp := rankRow("Sample.Film.2014.1080p.x264-GRP", g, 10, search.LangNone, false, "grp")
	enc := rankRow("Sample.Film.2014.1080p.x264-ENC", g, 10, search.LangNone, false, "enc")
	pGrp := search.Prefs{Groups: []string{"grp"}}
	assert.True(t, pGrp.Less(grp, enc))
	assert.False(t, pGrp.Less(enc, grp))
}

func TestPrefsLess_StreamAndSeeders(t *testing.T) {
	const gb = int64(1 << 30)
	var p search.Prefs

	hevc := rankRow("Sample.Film.2014.1080p.x265-GRP", 5*gb, 10, search.LangNone, false, "hevc")
	avc := rankRow("Sample.Film.2014.1080p.x264-GRP", 5*gb, 10, search.LangNone, false, "avc")
	remux := rankRow("Sample.Film.2014.1080p.BluRay.Remux.x265-GRP", 5*gb, 10, search.LangNone, false, "remux")
	tenBit := rankRow("Sample.Film.2014.1080p.10bit.x265-GRP", 5*gb, 10, search.LangNone, false, "10bit")
	bigFilm := rankRow("Sample.Film.2014.1080p.x265-GRP", 16*gb, 10, search.LangNone, false, "big")
	okFilm := rankRow("Sample.Film.2014.1080p.x264-GRP", 10*gb, 10, search.LangNone, false, "ok")
	bigEp := rankRow("Sample.Show.S01E02.2160p.x265-GRP", 11*gb, 10, search.LangNone, false, "bigep")
	okEp := rankRow("Sample.Show.S01E02.2160p.x264-GRP", 9*gb, 10, search.LangNone, false, "okep")
	bigPack := rankRow("Sample.Show.S01.2160p.x265-GRP", 80*gb, 10, search.LangNone, true, "pack")
	plain4K := rankRow("Sample.Show.S01.2160p.x264-GRP", 20*gb, 10, search.LangNone, true, "plain4k")

	assert.True(t, p.Less(hevc, avc))
	assert.True(t, p.Less(avc, remux))
	assert.True(t, p.Less(tenBit, avc), "10bit.x265 counts as HEVC")
	assert.True(t, p.Less(okFilm, bigFilm), "16 GB 1080p film sinks below 10 GB x264")
	assert.True(t, p.Less(okEp, bigEp), "11 GB 4K episode sinks below 9 GB x264")
	assert.True(t, p.Less(bigPack, plain4K), "80 GB pack has no ceiling")

	s50 := rankRow("Sample.Film.2014.1080p.x264-GRP", 5*gb, 50, search.LangNone, false, "s50")
	s40 := rankRow("Sample.Film.2014.1080p.x264-GRP", 5*gb, 40, search.LangNone, false, "s40")
	assert.True(t, p.Less(s50, s40))
}

func TestArrange_AndArrangeLate(t *testing.T) {
	const gb = int64(1 << 30)
	rows := []search.Result{
		rankRow("Sample.Film.2014.480p-GRP", 1*gb, 90, search.LangNone, false, "480"),
		rankRow("Sample.Film.2014.720p-GRP", 1*gb, 50, search.LangNone, false, "720"),
		rankRow("Sample.Film.2014.1080p-GRP", 2*gb, 40, search.LangNone, false, "1080"),
		rankRow("Sample.Film.2014.2160p-GRP", 5*gb, 30, search.LangNone, false, "4k"),
	}
	for i := range 12 {
		rows = append(rows, rankRow("Sample.Film.2014.1080p-GRP", 2*gb, i+1, search.LangNone, false, fmt.Sprintf("1080-%d", i)))
	}

	secs := search.Arrange(rows, search.DefaultCaps(), false, search.Prefs{})
	require.Len(t, secs, 3)
	assert.Equal(t, search.Group4K, secs[0].Group)
	assert.Equal(t, 1, secs[0].Shown)
	assert.Equal(t, search.Group1080p, secs[1].Group)
	assert.Equal(t, 10, secs[1].Shown)
	assert.Len(t, secs[1].Rows, 13)
	assert.Equal(t, search.Group720p, secs[2].Group)
	assert.Equal(t, 1, secs[2].Shown)

	allSecs := search.Arrange(rows, search.DefaultCaps(), true, search.Prefs{})
	assert.Equal(t, 13, allSecs[1].Shown)

	_, ok := search.ArrangeLate(nil, search.DefaultCaps(), search.Prefs{})
	assert.False(t, ok)

	var lateRows []search.Result
	for i := range 12 {
		lateRows = append(lateRows, rankRow("Sample.Film.2014.1080p-GRP", 2*gb, i+1, search.LangNone, false, fmt.Sprintf("late-%d", i)))
	}
	lateRows = append(lateRows, rankRow("Sample.Film.2014.2160p-GRP", 5*gb, 1, search.LangNone, false, "late-4k"))
	late, ok := search.ArrangeLate(lateRows, search.DefaultCaps(), search.Prefs{})
	require.True(t, ok)
	assert.True(t, late.Late)
	assert.Equal(t, search.LateCap, late.Shown)
	assert.Equal(t, "late-4k", late.Rows[0].Hash)
}

func TestSection_MoreOrderRemove(t *testing.T) {
	const gb = int64(1 << 30)
	var rows []search.Result
	for i := range 25 {
		rows = append(rows, rankRow("Sample.Film.2014.1080p-GRP", 2*gb, 100-i, search.LangNone, false, fmt.Sprintf("r-%d", i)))
	}
	secs := search.Arrange(rows, search.DefaultCaps(), false, search.Prefs{})
	require.Len(t, secs, 1)
	sec := &secs[0]
	assert.Equal(t, 10, sec.Shown)

	got1 := sec.More(10)
	require.Len(t, got1, 10)
	assert.Equal(t, "r-10", got1[0].Hash)
	assert.Equal(t, "r-19", got1[9].Hash)

	got2 := sec.More(10)
	require.Len(t, got2, 5)
	assert.Equal(t, "r-20", got2[0].Hash)
	assert.Equal(t, "r-24", got2[4].Hash)

	assert.Empty(t, sec.More(10))

	// Test Order only touches Rows[:Shown] and restores rank order.
	orderRows := []search.Result{
		rankRow("Sample.Film.2014.1080p-GRP", 1*gb, 50, search.LangNone, false, "small-high"),
		rankRow("Sample.Film.2014.1080p-GRP", 5*gb, 40, search.LangNone, false, "big-mid"),
		rankRow("Sample.Film.2014.1080p-GRP", 9*gb, 10, search.LangNone, false, "huge-low"),
	}
	osec := search.Arrange(orderRows, map[search.Group]int{search.Group1080p: 2}, false, search.Prefs{})[0]
	osec.Order(search.SortSize, search.Prefs{})
	assert.Equal(t, "big-mid", osec.Rows[0].Hash)
	assert.Equal(t, "small-high", osec.Rows[1].Hash)
	assert.Equal(t, "huge-low", osec.Rows[2].Hash, "row past Shown never moves")

	osec.Order(search.SortRank, search.Prefs{})
	assert.Equal(t, "small-high", osec.Rows[0].Hash)
	assert.Equal(t, "big-mid", osec.Rows[1].Hash)
	assert.Equal(t, "huge-low", osec.Rows[2].Hash)

	assert.True(t, osec.Remove(osec.Rows[0].Key()))
	assert.Equal(t, 1, osec.Shown)
	assert.Equal(t, "big-mid", osec.Rows[0].Hash)
	assert.Equal(t, "huge-low", osec.Rows[1].Hash, "replacement is not pulled up")
	assert.False(t, osec.Remove("1/missing"))
}

func TestSortKey_CyclesAndParses(t *testing.T) {
	assert.Equal(t, search.SortSize, search.SortRank.Next())
	assert.Equal(t, search.SortSeeders, search.SortSize.Next())
	assert.Equal(t, search.SortRank, search.SortSeeders.Next())
	assert.Equal(t, "rank", search.SortRank.String())
	assert.Equal(t, "size", search.SortSize.String())
	assert.Equal(t, "seeders", search.SortSeeders.String())

	for _, name := range []string{"rank", "cached", "resolution"} {
		key, ok := search.ParseSortKey(name)
		assert.True(t, ok, name)
		assert.Equal(t, search.SortRank, key, name)
	}
	key, ok := search.ParseSortKey("seeders")
	assert.True(t, ok)
	assert.Equal(t, search.SortSeeders, key)
	_, ok = search.ParseSortKey("best")
	assert.False(t, ok)
}

func TestBadgeGlyphs(t *testing.T) {
	assert.Equal(t, "●", search.BadgeCached.String())
	assert.Equal(t, "○", search.BadgeUncached.String())
	assert.Equal(t, "■", search.BadgeInLibrary.String())
	assert.Equal(t, "?", search.BadgeUnknown.String())
	assert.Equal(t, "…", search.BadgeResolving.String())
}
