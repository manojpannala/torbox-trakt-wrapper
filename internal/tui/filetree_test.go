package tui_test

import (
	"testing"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileTree_NavigationAndRender(t *testing.T) {
	parent := &tui.LibraryItem{
		ID:         1,
		RawName:    "Test.Series.Alpha.S01.1080p",
		CleanTitle: "Test Series Alpha S01",
		TorrentFiles: []torbox.TorrentFile{
			{ID: 101, Name: "Test.Series.Alpha.S01E01.mkv", Size: 1024 * 1024 * 500},
			{ID: 102, Name: "Test.Series.Alpha.S01E02.mkv", Size: 1024 * 1024 * 520},
			{ID: 103, Name: "Test.Series.Alpha.S01E03.mkv", Size: 1024 * 1024 * 510},
		},
	}

	engine := matcher.NewMatcher(nil, nil, nil)
	ft := tui.NewFileTreeModel(parent, engine)
	ft.Width = 80
	ft.Height = 24

	require.Len(t, ft.Items, 3)
	assert.Equal(t, 0, ft.Cursor)
	assert.Equal(t, 101, ft.SelectedItem().ID)

	ft.MoveDown()
	assert.Equal(t, 1, ft.Cursor)
	assert.Equal(t, 102, ft.SelectedItem().ID)

	ft.MoveDown()
	assert.Equal(t, 2, ft.Cursor)
	assert.Equal(t, 103, ft.SelectedItem().ID)

	ft.MoveDown()
	assert.Equal(t, 2, ft.Cursor)

	ft.MoveUp()
	assert.Equal(t, 1, ft.Cursor)
	assert.Equal(t, 102, ft.SelectedItem().ID)

	rendered := ft.Render(tui.DefaultTheme(), 80, 24)
	assert.Contains(t, rendered, "Test Series Alpha S01")
	assert.Contains(t, rendered, "Test Series Alpha S01E01")
}

func TestFileTree_Empty(t *testing.T) {
	ft := tui.NewFileTreeModel(nil, nil)
	assert.Empty(t, ft.Items)
	assert.Nil(t, ft.SelectedItem())

	rendered := ft.Render(tui.DefaultTheme(), 80, 24)
	assert.Contains(t, rendered, "No files available")
}
