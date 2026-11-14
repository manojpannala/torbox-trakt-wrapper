package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

func TestResolveSections_ABetterSeededDuplicateKeepsEveryIndexer(t *testing.T) {
	hash := strings.Repeat("a", 40)
	resolve := func(context.Context) (string, error) { return hash, nil }
	sections := []search.Section{{Rows: []search.Result{
		{Title: "Sample.Film.2014.1080p.x265-GRP", Seeders: 5, Indexers: []string{"One"}, IndexerID: 1, GUID: "x", Resolve: resolve},
		{Title: "Sample.Film.2014.1080p-GRP", Seeders: 50, Indexers: []string{"Two"}, IndexerID: 2, GUID: "y", Resolve: resolve},
	}, Shown: 2}}

	unresolved := cliSearch{}.resolveSections(context.Background(), sections)

	assert.Zero(t, unresolved)
	shown := sections[0].Rows[:sections[0].Shown]
	require.Len(t, shown, 1)
	assert.Equal(t, 50, shown[0].Seeders)
	assert.ElementsMatch(t, []string{"One", "Two"}, shown[0].Indexers)
}
