package search_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

func TestTally_Attrs(t *testing.T) {
	const gb = int64(1 << 30)
	secs := []search.Section{
		{
			Group: search.Group4K,
			Shown: 2,
			Rows: []search.Result{
				rankRow("Secret.Release.Title.2160p-GRP", 5*gb, 10, search.LangNone, false, "h1"),
				rankRow("Another.Secret.Title.2160p-GRP", 5*gb, 9, search.LangNone, false, "h2"),
				rankRow("Third.Secret.Title.2160p-GRP", 5*gb, 8, search.LangNone, false, "h3"),
			},
		},
		{
			Group: search.Group1080p,
			Shown: 1,
			Rows: []search.Result{
				rankRow("Fourth.Secret.Title.1080p-GRP", 2*gb, 10, search.LangNone, false, "h4"),
			},
		},
		{
			Group: search.Group720p,
			Late:  true,
			Shown: 1,
			Rows: []search.Result{
				rankRow("Late.Secret.Title.720p-GRP", 1*gb, 5, search.LangNone, false, "h5"),
			},
		},
	}

	tally := search.Tally{
		Received: 20,
		Merged:   15,
		Dropped: map[search.DropReason]int{
			search.DropTitle:   2,
			search.DropSeeders: 3,
		},
		Late: 1,
	}

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	log.Debug("search summary", tally.Attrs(secs)...)

	line := buf.String()
	assert.Contains(t, line, "received=20")
	assert.Contains(t, line, "merged=15")
	assert.Contains(t, line, `groups="4K 2/3 1080p 1/1"`)
	assert.Contains(t, line, "dropped_title=2")
	assert.Contains(t, line, "dropped_seeders=3")
	assert.Contains(t, line, "dropped_source=0")
	assert.Contains(t, line, "dropped_size=0")
	assert.Contains(t, line, "dropped_unresolved=0")
	assert.Contains(t, line, "late=1")
	assert.NotContains(t, line, "Secret")
}
