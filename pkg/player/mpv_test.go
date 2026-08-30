package player_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

func TestMPVPlayer_Options(t *testing.T) {
	customDir, err := os.MkdirTemp("", "mpv-sock-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(customDir)
	}()

	scrobbler := &mockScrobbler{}

	p := player.NewMPVPlayer(
		player.WithExecutable("/usr/bin/mpv"),
		player.WithExtraArgs([]string{"--vo=gpu-next", "--hwdec=auto"}),
		player.WithSocketDir(customDir),
		player.WithIPCEnabled(false),
		player.WithScrobbler(scrobbler),
	)
	assert.NotNil(t, p)
}

func TestMPVPlayer_EmptyURLValidation(t *testing.T) {
	p := player.NewMPVPlayer()
	_, err := p.Play(context.Background(), player.MediaStream{URL: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty media stream url")
}

func TestMPVPlayer_PlayAndSessionLifecycle(t *testing.T) {
	p := player.NewMPVPlayer(
		player.WithExecutable("true"),
		player.WithIPCEnabled(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream := player.MediaStream{
		URL:        "https://example.com/stream.mkv",
		Title:      "Test Movie Alpha",
		ResumeSecs: 120,
	}

	session, err := p.Play(ctx, stream)
	require.NoError(t, err)
	assert.NotNil(t, session)

	err = session.Wait()
	require.NoError(t, err)

	err = session.Close()
	require.NoError(t, err)
}

func TestMPVPlayer_FailedStart(t *testing.T) {
	p := player.NewMPVPlayer(
		player.WithExecutable("/nonexistent/binary/mpv"),
		player.WithIPCEnabled(false),
	)

	_, err := p.Play(context.Background(), player.MediaStream{URL: "https://example.com/file.mkv"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start mpv")
}

func TestTraktScrobbler_StartPauseStop(t *testing.T) {
	var startHit, pauseHit, stopHit bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/scrobble/start":
			startHit = true
			_, _ = w.Write([]byte(`{"action":"start","progress":10}`))
		case "/scrobble/pause":
			pauseHit = true
			_, _ = w.Write([]byte(`{"action":"pause","progress":50}`))
		case "/scrobble/stop":
			stopHit = true
			_, _ = w.Write([]byte(`{"action":"scrobble","progress":95}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := trakt.NewClient("cid", "csecret", trakt.WithBaseURL(server.URL))
	scrobbler := player.NewTraktScrobbler(client)

	mediaMovie := matcher.ParsedMedia{
		CleanTitle: "Test Movie Alpha",
		Year:       2023,
		Type:       matcher.MediaTypeMovie,
	}

	err := scrobbler.Start(context.Background(), mediaMovie, 10.0)
	require.NoError(t, err)
	assert.True(t, startHit)

	err = scrobbler.Pause(context.Background(), mediaMovie, 50.0)
	require.NoError(t, err)
	assert.True(t, pauseHit)

	resp, err := scrobbler.Stop(context.Background(), mediaMovie, 95.0)
	require.NoError(t, err)
	assert.True(t, stopHit)
	assert.Equal(t, "scrobble", resp.Action)

	mediaEpisode := matcher.ParsedMedia{
		CleanTitle: "Test Crime Series",
		Season:     2,
		Episode:    5,
		Type:       matcher.MediaTypeEpisode,
	}
	err = scrobbler.Start(context.Background(), mediaEpisode, 5.0)
	require.NoError(t, err)
}

func TestMPVPlayer_KeepOpenOverride(t *testing.T) {
	run := func(keepOpen string) string {
		p := player.NewMPVPlayer(
			player.WithExecutable("echo"),
			player.WithIPCEnabled(false),
			player.WithKeepOpen(keepOpen),
		)

		var out bytes.Buffer
		session, err := p.Play(context.Background(), player.MediaStream{
			URL:    "https://example.com/stream.mkv",
			Stdout: &out,
		})
		require.NoError(t, err)
		require.NoError(t, session.Wait())
		return out.String()
	}

	assert.Contains(t, run("no"), "--keep-open=no")
	assert.Contains(t, run("always"), "--keep-open=always")
	assert.NotContains(t, run(""), "--keep-open", "an unset value leaves mpv.conf alone")
	assert.NotContains(t, run("maybe"), "--keep-open", "an invalid value leaves mpv.conf alone")
}
