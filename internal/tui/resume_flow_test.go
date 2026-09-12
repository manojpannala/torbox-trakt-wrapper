package tui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

// collect runs a command and every command a tea.Batch fans out to.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, collect(c)...)
		}
		return out
	case nil:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

func resumeAppWith(t *testing.T, progress float64) tui.AppModel {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":"https://stream.invalid/a.mkv?sig=x"}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("TORBOX_BASE_URL", server.URL)

	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "test-api-key"

	app := tui.NewAppModel(context.Background(), cfg)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	var playback []trakt.PlaybackItem
	if progress > 0 {
		playback = []trakt.PlaybackItem{{
			ID: 101, Progress: progress, Type: "movie",
			PausedAt: time.Now().Add(-72 * time.Hour),
			Movie:    &trakt.Movie{Title: "Test Feature Alpha", Year: 2023},
		}}
	}
	m, _ = m.(tui.AppModel).Update(tui.TraktCatalogLoadedMsg{Playback: playback})
	m, _ = m.(tui.AppModel).Update(tui.TorrentsLoadedMsg{Torrents: []torbox.Torrent{
		{ID: 1, Name: "Test.Feature.Alpha.2023.1080p.BluRay.x264.mkv", DownloadState: "completed"},
	}})
	return m.(tui.AppModel)
}

func enter(m tui.AppModel) (tui.AppModel, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(tui.AppModel), cmd
}

func key(m tui.AppModel, r rune) (tui.AppModel, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	return next.(tui.AppModel), cmd
}

func resumeOf(t *testing.T, msgs []tea.Msg) float64 {
	t.Helper()
	for _, msg := range msgs {
		if resolved, ok := msg.(tui.StreamURLResolvedMsg); ok {
			return resolved.ResumeAtPercent
		}
	}
	t.Fatalf("no stream was resolved; got %#v", msgs)
	return 0
}

func TestResumePrompt_AppearsForAnInProgressItem(t *testing.T) {
	m, cmd := enter(resumeAppWith(t, 41))

	assert.Nil(t, cmd, "the link must not be resolved until the user decides")
	view := strings.Join(strings.Fields(m.View().Content), " ")
	assert.Contains(t, view, "Resume Playback")
	assert.Contains(t, view, "41%")
}

func TestResumePrompt_IsSkippedWhenThereIsNoPosition(t *testing.T) {
	m, cmd := enter(resumeAppWith(t, 0))

	assert.NotContains(t, m.View().Content, "Resume Playback", "no position means no keypress")
	assert.Equal(t, float64(0), resumeOf(t, collect(cmd)))
}

func TestResumePrompt_ResumeCarriesTheStoredPosition(t *testing.T) {
	m, _ := enter(resumeAppWith(t, 41))
	_, cmd := key(m, 'r')

	assert.Equal(t, 41.0, resumeOf(t, collect(cmd)))
}

func TestResumePrompt_StartOverPlaysFromZero(t *testing.T) {
	m, _ := enter(resumeAppWith(t, 41))
	m2, cmd := key(m, 's')

	assert.Equal(t, float64(0), resumeOf(t, collect(cmd)))
	assert.NotContains(t, m2.View().Content, "Resume Playback")
}

func TestResumePrompt_EscapeCancelsWithoutStreaming(t *testing.T) {
	m, _ := enter(resumeAppWith(t, 41))
	m2, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.Nil(t, cmd, "cancelling must not resolve a link")
	assert.NotContains(t, m2.(tui.AppModel).View().Content, "Resume Playback")
}

func TestResumePrompt_AppearsForAnEpisodeInsideAFolder(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "test-api-key"

	app := tui.NewAppModel(context.Background(), cfg)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.(tui.AppModel).Update(tui.TraktCatalogLoadedMsg{
		Playback: []trakt.PlaybackItem{{
			ID: 201, Progress: 72, Type: "episode",
			PausedAt: time.Now().Add(-2 * time.Hour),
			Show:     &trakt.Show{Title: "Test Series Gamma"},
			Episode:  &trakt.Episode{Season: 1, Number: 2},
		}},
	})
	m, _ = m.(tui.AppModel).Update(tui.TorrentsLoadedMsg{Torrents: []torbox.Torrent{{
		ID: 3, Name: "Test.Series.Gamma.S01.1080p.WEB-DL", DownloadState: "completed",
		Files: []torbox.TorrentFile{
			{ID: 31, Name: "Test.Series.Gamma.S01E01.1080p.mkv"},
			{ID: 32, Name: "Test.Series.Gamma.S01E02.1080p.mkv"},
		},
	}}})

	// open the file tree, move to E02, play it
	app2, _ := key(m.(tui.AppModel), 'f')
	app2, _ = key(app2, 'j')
	app2, cmd := enter(app2)

	assert.Nil(t, cmd, "a folder episode must prompt like any other item")
	view := strings.Join(strings.Fields(app2.View().Content), " ")
	assert.Contains(t, view, "Resume Playback")
	assert.Contains(t, view, "72%")
}
