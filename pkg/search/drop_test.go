package search_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

const (
	mb = int64(1 << 20)
	gb = int64(1 << 30)
)

func dropRow(name string, size int64, seeders int, hash string, pack bool) search.Result {
	return search.Result{
		Title:   name,
		Hash:    hash,
		Size:    size,
		Seeders: seeders,
		Pack:    pack,
		Media:   matcher.ParseMedia(name),
	}
}

func TestDrop_SeedersAndLibrary(t *testing.T) {
	row := dropRow("Sample.Film.2014.1080p.BluRay.x264-GRP", 2*gb, 0, "abc123", false)

	kept, dropped := search.Drop([]search.Result{row}, nil)
	assert.Empty(t, kept)
	assert.Equal(t, 1, dropped[search.DropSeeders])

	kept, dropped = search.Drop([]search.Result{row}, func(h string) bool { return h == "abc123" })
	require.Len(t, kept, 1)
	assert.Empty(t, dropped)
}

func TestDrop_Source(t *testing.T) {
	bad := []string{
		"Sample.Film.2014.CAM.x264-GRP",
		"Sample.Film.2014.HDCAM.x264-GRP",
		"Sample.Film.2014.TS.x264-GRP",
		"Sample.Film.2014.TC.x264-GRP",
		"Sample.Film.2014.SCR.x264-GRP",
		"Sample.Film.2014.Screener.x264-GRP",
	}
	for _, name := range bad {
		kept, dropped := search.Drop([]search.Result{dropRow(name, 2*gb, 10, "", false)}, nil)
		assert.Empty(t, kept, name)
		assert.Equal(t, 1, dropped[search.DropSource], name)
	}

	kept, dropped := search.Drop([]search.Result{
		dropRow("Sample.Film.2014.1080p.DTS-GRP", 2*gb, 10, "", false),
	}, nil)
	require.Len(t, kept, 1)
	assert.Empty(t, dropped)
}

func TestDrop_SizeFloors(t *testing.T) {
	tests := []struct {
		name     string
		row      search.Result
		wantKeep bool
	}{
		{"4K film 900MB", dropRow("Sample.Film.2014.2160p-GRP", 900*mb, 10, "", false), false},
		{"4K film 1GB", dropRow("Sample.Film.2014.2160p-GRP", 1*gb, 10, "", false), true},
		{"4K film unknown size", dropRow("Sample.Film.2014.2160p-GRP", 0, 10, "", false), true},
		{"4K episode 400MB", dropRow("Sample.Show.S01E02.2160p-GRP", 400*mb, 10, "", false), true},
		{"4K episode 200MB", dropRow("Sample.Show.S01E02.2160p-GRP", 200*mb, 10, "", false), false},
		{"4K pack 900MB", dropRow("Sample.Show.S01.2160p-GRP", 900*mb, 10, "", true), false},
		{"1080p film 250MB", dropRow("Sample.Film.2014.1080p-GRP", 250*mb, 10, "", false), false},
		{"720p film 140MB", dropRow("Sample.Film.2014.720p-GRP", 140*mb, 10, "", false), false},
		{"480p film 10MB", dropRow("Sample.Film.2014.480p-GRP", 10*mb, 10, "", false), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept, dropped := search.Drop([]search.Result{tt.row}, nil)
			if tt.wantKeep {
				require.Len(t, kept, 1)
				assert.Empty(t, dropped)
			} else {
				assert.Empty(t, kept)
				assert.Equal(t, 1, dropped[search.DropSize])
			}
		})
	}
}

func TestDrop_OrderAndCounts(t *testing.T) {
	rows := []search.Result{
		dropRow("Sample.Film.2014.CAM.x264-GRP", 2*gb, 0, "h1", false),
		dropRow("Sample.Film.2014.1080p.First-GRP", 2*gb, 10, "h2", false),
		dropRow("Sample.Film.2014.HDCAM.x264-GRP", 2*gb, 5, "h3", false),
		dropRow("Sample.Film.2014.1080p-GRP", 100*mb, 5, "h4", false),
		dropRow("Sample.Film.2014.1080p.Second-GRP", 2*gb, 10, "h5", false),
	}

	kept, dropped := search.Drop(rows, nil)
	require.Len(t, kept, 2)
	assert.Equal(t, "h2", kept[0].Hash)
	assert.Equal(t, "h5", kept[1].Hash)
	assert.Equal(t, map[search.DropReason]int{
		search.DropSeeders: 1,
		search.DropSource:  1,
		search.DropSize:    1,
	}, dropped)
}
