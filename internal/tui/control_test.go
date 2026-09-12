package tui_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
)

type apiCall struct {
	Path string
	Body map[string]any
}

type apiRecorder struct {
	mu    sync.Mutex
	calls []apiCall
}

func (r *apiRecorder) record(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	var decoded map[string]any
	_ = json.Unmarshal(body, &decoded)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, apiCall{Path: req.URL.Path, Body: decoded})
}

func (r *apiRecorder) pathsHit() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, c := range r.calls {
		out = append(out, c.Path)
	}
	return out
}

// torboxStub points the TorBox client at a recorder for the duration of a test.
func torboxStub(t *testing.T) *apiRecorder {
	t.Helper()
	rec := &apiRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("TORBOX_BASE_URL", server.URL)
	return rec
}

func appWithTorrent(t *testing.T, state string) tui.AppModel {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "test-api-key"

	app := tui.NewAppModel(context.Background(), cfg)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.(tui.AppModel).Update(tui.TorrentsLoadedMsg{Torrents: []torbox.Torrent{
		{ID: 77, Name: "Test.Feature.Alpha.2023.1080p.mkv", DownloadState: state, Progress: 0.5},
	}})
	return m.(tui.AppModel)
}

func press(t *testing.T, m tui.AppModel, key rune) tea.Cmd {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
	_ = next
	return cmd
}

// drain runs a command and every command a tea.Batch fans out to.
func drain(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			drain(c)
		}
	}
}

func TestAppModel_PPausesAnActiveTorrent(t *testing.T) {
	rec := torboxStub(t)
	app := appWithTorrent(t, "downloading")

	drain(press(t, app, 'p'))

	require.Contains(t, rec.pathsHit(), "/torrents/controltorrent",
		"the help modal documents `p`, so it must reach the control endpoint")
	assert.Equal(t, "pause", rec.calls[0].Body["operation"])
	assert.Equal(t, float64(77), rec.calls[0].Body["torrent_id"])
}

func TestAppModel_PResumesAPausedTorrent(t *testing.T) {
	rec := torboxStub(t)
	app := appWithTorrent(t, "paused")

	drain(press(t, app, 'p'))

	require.NotEmpty(t, rec.calls, "pressing p on a paused torrent must call the API")
	assert.Equal(t, "resume", rec.calls[0].Body["operation"])
}
