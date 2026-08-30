package tui_test

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

func TestAppModel_InitAndUpdate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "test-api-key"
	cfg.Trakt.ClientID = "test-client-id"
	cfg.Trakt.AccessToken = "test-access-token"

	app := tui.NewAppModel(context.Background(), cfg)
	cmd := app.Init()
	assert.NotNil(t, cmd)

	m, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	appModel, ok := m.(tui.AppModel)
	require.True(t, ok)

	torrents := []torbox.Torrent{
		{
			ID:            1,
			Name:          "Test.Movie.Alpha.2023.1080p.mkv",
			Size:          1024 * 1024 * 1024,
			DownloadState: "completed",
			Progress:      1.0,
		},
		{
			ID:            2,
			Name:          "Test.Series.Beta.S01E01.1080p.mkv",
			Size:          500 * 1024 * 1024,
			DownloadState: "downloading",
			Progress:      0.5,
		},
	}

	m, _ = appModel.Update(tui.TorrentsLoadedMsg{Torrents: torrents})
	appModel = m.(tui.AppModel)

	viewStr := appModel.View()
	assert.Contains(t, viewStr, "TORBOX TRAKT WRAPPER")
	assert.Contains(t, viewStr, "Test Movie Alpha")
	assert.Contains(t, viewStr, "Test Series Beta")

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyTab})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tui.UsenetLoadedMsg{
		Usenet: []torbox.UsenetItem{
			{
				ID:            10,
				Name:          "Test.Usenet.Gamma.2024.1080p.mkv",
				Size:          2 * 1024 * 1024 * 1024,
				DownloadState: "cached",
				Progress:      1.0,
			},
		},
	})
	appModel = m.(tui.AppModel)
	assert.Contains(t, appModel.View(), "Test Usenet Gamma")

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyTab})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tui.WebDLLoadedMsg{
		WebDL: []torbox.WebDLItem{
			{
				ID:            20,
				Name:          "Test.WebDL.Delta.2024.720p.mkv",
				Size:          700 * 1024 * 1024,
				DownloadState: "completed",
				Progress:      1.0,
			},
		},
	})
	appModel = m.(tui.AppModel)
	assert.Contains(t, appModel.View(), "Test Delta")

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A', 'l', 'p', 'h', 'a'}})
	appModel = m.(tui.AppModel)
	viewStr = appModel.View()
	assert.Contains(t, viewStr, "Test Movie Alpha")

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	appModel = m.(tui.AppModel)
	assert.Contains(t, appModel.View(), "Keyboard Shortcuts")

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	appModel = m.(tui.AppModel)
	assert.Contains(t, appModel.View(), "Add Download")

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	appModel = m.(tui.AppModel)
	assert.Contains(t, appModel.View(), "Confirm Deletion")

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	appModel = m.(tui.AppModel)

	traktCatalog := tui.TraktCatalogLoadedMsg{
		Movies: []trakt.WatchedMovie{
			{
				Plays: 1,
				Movie: trakt.Movie{Title: "Test Movie Alpha", Year: 2023},
			},
		},
	}
	m, _ = appModel.Update(traktCatalog)
	appModel = m.(tui.AppModel)

	viewStr = appModel.View()
	assert.Contains(t, viewStr, "✓")

	m, _ = appModel.Update(tui.StatusMsg{Text: "Operation failed", IsErr: true})
	appModel = m.(tui.AppModel)
	assert.Contains(t, appModel.View(), "Operation failed")

	m, _ = appModel.Update(tui.DeviceCodeGeneratedMsg{
		Code: &trakt.DeviceCodeResponse{
			UserCode:        "ABCD-1234",
			VerificationURL: "https://trakt.tv/activate",
			ExpiresIn:       600,
			Interval:        5,
		},
	})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tui.TokenPollErrorMsg{Err: errors.New("pairing denied")})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tui.TokenPollSuccessMsg{
		Token: &trakt.TokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			CreatedAt:    1700000000,
			ExpiresIn:    7200,
		},
	})
	_ = m.(tui.AppModel)
}

func TestAppModel_FileTreeToggle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	app := tui.NewAppModel(context.Background(), cfg)

	m, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	appModel := m.(tui.AppModel)

	torrents := []torbox.Torrent{
		{
			ID:            1,
			Name:          "Test.Series.Zeta.S01.1080p",
			Size:          2 * 1024 * 1024 * 1024,
			DownloadState: "completed",
			Progress:      1.0,
			Files: []torbox.TorrentFile{
				{ID: 11, Name: "Test.Series.Zeta.S01E01.mkv", Size: 1024 * 1024 * 1024},
				{ID: 12, Name: "Test.Series.Zeta.S01E02.mkv", Size: 1024 * 1024 * 1024},
			},
		},
	}

	m, _ = appModel.Update(tui.TorrentsLoadedMsg{Torrents: torrents})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	appModel = m.(tui.AppModel)
	assert.Contains(t, appModel.View(), "Files:")

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	appModel = m.(tui.AppModel)

	m, _ = appModel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	appModel = m.(tui.AppModel)
	assert.NotContains(t, appModel.View(), "Files:")
}
