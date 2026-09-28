package cli_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

// streamSetup stubs TorBox with one torrent and points the player at a script
// that records the URL it was asked to play.
func streamSetup(t *testing.T, streamProxy bool) (cfgPath, playedFile string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/torrents/mylist"):
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":42,"name":"Some.Film.2024.1080p","files":[{"id":1,"size":100}]}]}`))
		case strings.HasSuffix(r.URL.Path, "/torrents/requestdl"):
			_, _ = w.Write([]byte(`{"success":true,"data":"https://cdn.example/some-film.mkv?token=signed"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("TORBOX_BASE_URL", server.URL)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // the player makes its socket dir here

	dir := t.TempDir()
	playedFile = filepath.Join(dir, "played")
	script := filepath.Join(dir, "fake-mpv")
	body := "#!/bin/sh\nfor arg; do last=$arg; done\nprintf '%s' \"$last\" > '" + playedFile + "'\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o700)) // #nosec G306

	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "dummy-api-key"
	cfg.Player.Command = script
	cfg.Player.EnableIPC = false
	cfg.Player.StreamProxy = streamProxy
	cfgPath = filepath.Join(dir, "config.toml")
	require.NoError(t, cfg.SaveToFile(cfgPath))
	return cfgPath, playedFile
}

func TestCLI_StreamPlaysThroughTheLocalProxy(t *testing.T) {
	cfgPath, playedFile := streamSetup(t, true)

	_, err := executeCommand("--config", cfgPath, "stream", "42")
	require.NoError(t, err)

	played, err := os.ReadFile(playedFile) // #nosec G304
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(played), "http://127.0.0.1:"), "got %q", played)
	assert.NotContains(t, string(played), "signed")
}

func TestCLI_StreamHandsTheLinkStraightToThePlayerWhenTheProxyIsOff(t *testing.T) {
	cfgPath, playedFile := streamSetup(t, false)

	_, err := executeCommand("--config", cfgPath, "stream", "42")
	require.NoError(t, err)

	played, err := os.ReadFile(playedFile) // #nosec G304
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/some-film.mkv?token=signed", string(played))
}
