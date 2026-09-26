package matcher

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatcher_Parse_MemoizesSecondCall(t *testing.T) {
	original := parseMediaFunc
	t.Cleanup(func() { parseMediaFunc = original })

	var calls atomic.Int32
	parseMediaFunc = func(name string) ParsedMedia {
		calls.Add(1)
		return original(name)
	}

	m := NewMatcher(nil, nil, nil)
	const name = "Test.Crime.Series.S01E05.Episode.Name.1080p.BluRay.x264-DEMAND.mkv"

	first := m.Parse(name)
	second := m.Parse(name)

	assert.Equal(t, first, second, "the cached value must equal the freshly parsed one")
	assert.EqualValues(t, 1, calls.Load(), "the second call must be a cache hit, not a re-parse")
}

func TestMatcher_Parse_ConcurrentAccessIsRaceFree(t *testing.T) {
	m := NewMatcher(nil, nil, nil)

	names := []string{
		"Test.Movie.Alpha.2023.1080p.BluRay.x264.mkv",
		"Test.Show.Beta.S01E01.720p.WEB-DL.mkv",
		"Test.Show.Beta.S01E02.720p.WEB-DL.mkv",
		"[Test-Group] Test Anime Gamma - 07 [1080p].mkv",
	}

	const goroutines = 8
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				name := names[(g+i)%len(names)]
				_ = m.Parse(name)
			}
		}(g)
	}
	wg.Wait()

	require.Len(t, m.parseMemo, len(names), "every distinct name should have exactly one memo entry")
}
