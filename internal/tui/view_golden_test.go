package tui_test

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

// updateGolden rewrites the files under testdata instead of comparing against
// them: go test ./internal/tui -run ViewGolden -update
var updateGolden = flag.Bool("update", false, "rewrite the golden view files")

const (
	goldenWidth  = 120
	goldenHeight = 40
)

// Styling is stripped before comparison. Lip Gloss decides how much colour to
// emit from the surrounding terminal, so the escape sequences are not stable
// across environments; the layout is what these files guard.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func goldenTorrents() []torbox.Torrent {
	return []torbox.Torrent{
		{
			ID:            1,
			Name:          "Test.Feature.Alpha.2023.1080p.BluRay.x264.mkv",
			Size:          8 * 1024 * 1024 * 1024,
			DownloadState: "completed",
			Progress:      1.0,
		},
		{
			ID:            2,
			Name:          "Test.Feature.Beta.2022.2160p.WEB-DL.mkv",
			Size:          24 * 1024 * 1024 * 1024,
			DownloadState: "completed",
			Progress:      1.0,
		},
		{
			ID:            3,
			Name:          "Test.Series.Gamma.S01.1080p.WEB-DL",
			Size:          12 * 1024 * 1024 * 1024,
			DownloadState: "downloading",
			Progress:      0.375,
			Files: []torbox.TorrentFile{
				{ID: 31, Name: "Test.Series.Gamma.S01E01.1080p.mkv", Size: 3 * 1024 * 1024 * 1024},
				{ID: 32, Name: "Test.Series.Gamma.S01E02.1080p.mkv", Size: 3 * 1024 * 1024 * 1024},
				{ID: 33, Name: "Test.Series.Gamma.S01E03.1080p.mkv", Size: 3 * 1024 * 1024 * 1024},
			},
		},
	}
}

func goldenCatalog() tui.TraktCatalogLoadedMsg {
	return tui.TraktCatalogLoadedMsg{
		Movies: []trakt.WatchedMovie{
			{Plays: 1, Movie: trakt.Movie{Title: "Test Feature Alpha", Year: 2023}},
		},
		Playback: []trakt.PlaybackItem{
			{
				ID:       501,
				Type:     "movie",
				Progress: 42,
				Movie:    &trakt.Movie{Title: "Test Feature Beta", Year: 2022},
			},
		},
	}
}

func goldenModel(t *testing.T) tui.AppModel {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "test-api-key"
	cfg.Trakt.ClientID = "test-client-id"
	cfg.Trakt.AccessToken = "test-access-token"

	m := tui.NewAppModel(t.Context(), cfg)
	return step(t, m, tea.WindowSizeMsg{Width: goldenWidth, Height: goldenHeight})
}

func step(t *testing.T, m tui.AppModel, msg tea.Msg) tui.AppModel {
	t.Helper()
	next, _ := m.Update(msg)
	updated, ok := next.(tui.AppModel)
	require.True(t, ok, "Update returned %T, not tui.AppModel", next)
	return updated
}

func typeRunes(t *testing.T, m tui.AppModel, s string) tui.AppModel {
	t.Helper()
	for _, r := range s {
		m = step(t, m, keyRune(r))
	}
	return m
}

// keyRune builds the press of a printable key; keyPress builds a named one such
// as tea.KeyEsc, which carries no text.
func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func keyPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestAppModel_ViewGolden(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) tui.AppModel
	}{
		{
			name: "empty_library",
			build: func(t *testing.T) tui.AppModel {
				return step(t, goldenModel(t), tui.TorrentsLoadedMsg{})
			},
		},
		{
			name: "library_with_badges",
			build: func(t *testing.T) tui.AppModel {
				m := step(t, goldenModel(t), tui.TorrentsLoadedMsg{Torrents: goldenTorrents()})
				return step(t, m, goldenCatalog())
			},
		},
		{
			name: "library_filtered",
			build: func(t *testing.T) tui.AppModel {
				m := step(t, goldenModel(t), tui.TorrentsLoadedMsg{Torrents: goldenTorrents()})
				m = step(t, m, goldenCatalog())
				m = typeRunes(t, m, "/")
				return typeRunes(t, m, "Beta")
			},
		},
		{
			name: "status_error",
			build: func(t *testing.T) tui.AppModel {
				m := step(t, goldenModel(t), tui.TorrentsLoadedMsg{Torrents: goldenTorrents()})
				return step(t, m, tui.StatusMsg{Text: "Failed to resolve stream URL", IsErr: true})
			},
		},
		{
			name: "file_tree",
			build: func(t *testing.T) tui.AppModel {
				m := step(t, goldenModel(t), tui.TorrentsLoadedMsg{Torrents: goldenTorrents()})
				m = typeRunes(t, m, "jj")
				return typeRunes(t, m, "f")
			},
		},
		{
			name: "modal_help",
			build: func(t *testing.T) tui.AppModel {
				m := step(t, goldenModel(t), tui.TorrentsLoadedMsg{Torrents: goldenTorrents()})
				return typeRunes(t, m, "?")
			},
		},
		{
			name: "modal_add",
			build: func(t *testing.T) tui.AppModel {
				m := step(t, goldenModel(t), tui.TorrentsLoadedMsg{Torrents: goldenTorrents()})
				return typeRunes(t, m, "a")
			},
		},
		{
			name: "modal_delete",
			build: func(t *testing.T) tui.AppModel {
				m := step(t, goldenModel(t), tui.TorrentsLoadedMsg{Torrents: goldenTorrents()})
				return typeRunes(t, m, "d")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ansiEscape.ReplaceAllString(tt.build(t).View().Content, "")
			path := filepath.Join("testdata", tt.name+".golden")

			if *updateGolden {
				require.NoError(t, os.WriteFile(path, []byte(got), 0o600))
				return
			}

			want, err := os.ReadFile(path) // #nosec G304 -- path is built from the test table
			require.NoError(t, err, "run with -update to create %s", path)
			assert.Equal(t, string(want), got)
		})
	}
}
