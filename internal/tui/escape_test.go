package tui_test

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
)

// hostileName is what an attacker can call a torrent on a public tracker.
// ESC[2J clears the screen, OSC 52 writes to the clipboard on several
// terminals, and the cursor moves let the UI be redrawn to impersonate a
// different item.
const hostileName = "Movie\x1b[2J\x1b]52;c;aGF4\x07\x1b[10;1HFAKE.2023.1080p.mkv"

func viewWithHostileTorrent(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "test-api-key"

	app := tui.NewAppModel(context.Background(), cfg)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.(tui.AppModel).Update(tui.TorrentsLoadedMsg{Torrents: []torbox.Torrent{
		{ID: 1, Name: hostileName, DownloadState: "completed"},
	}})
	return m.(tui.AppModel).View().Content
}

func TestView_DoesNotRenderTerminalEscapesFromATorrentName(t *testing.T) {
	out := viewWithHostileTorrent(t)

	assert.NotContains(t, out, "\x1b[2J", "a torrent name could clear the screen")
	assert.NotContains(t, out, "\x1b]52;", "a torrent name could write to the clipboard")
	assert.NotContains(t, out, "\x1b[10;1H", "a torrent name could reposition the cursor")
	assert.NotContains(t, out, "\x07", "a torrent name could ring the bell")
}

func TestView_KeepsTheHarmlessPartOfAHostileName(t *testing.T) {
	assert.Contains(t, viewWithHostileTorrent(t), "Movie", "sanitising must not blank the row")
}
