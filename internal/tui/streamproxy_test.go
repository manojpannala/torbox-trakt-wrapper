package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/stream"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
)

func TestStreamItem_TheResolvedLinkCanBeRenewed(t *testing.T) {
	rec := stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/torrents/requestdl": respond(200, `{"success":true,"data":"https://cdn.example/a.mkv"}`),
	})
	m := testModel(t)
	item := &LibraryItem{ID: 7, Category: TabTorrents, CleanTitle: "Some Film", TorrentFiles: []torbox.TorrentFile{{ID: 2}}}

	msg := m.streamItemCmd(item, 0)()

	resolved, ok := msg.(StreamURLResolvedMsg)
	require.True(t, ok, "got %T", msg)
	assert.Equal(t, "https://cdn.example/a.mkv", resolved.URL)
	require.NotNil(t, resolved.Renew)
	link, err := resolved.Renew(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/a.mkv", link)
	urls := rec.all()
	require.Len(t, urls, 2)
	assert.Equal(t, urls[0], urls[1], "renewing asks for the same file again")
	assert.Contains(t, urls[1], "torrent_id=7")
	assert.Contains(t, urls[1], "file_id=2")
}

func TestStreamFile_TheResolvedLinkCanBeRenewed(t *testing.T) {
	rec := stubAPI(t, "TORBOX_BASE_URL", map[string]func(http.ResponseWriter){
		"/usenet/requestdl": respond(200, `{"success":true,"data":"https://cdn.example/b.mkv"}`),
	})
	m := testModel(t)
	parent := &LibraryItem{ID: 9, Category: TabUsenet}

	msg := m.streamFileCmd(parent, 3, "Episode", parent.Parsed, 0)()

	resolved, ok := msg.(StreamURLResolvedMsg)
	require.True(t, ok, "got %T", msg)
	require.NotNil(t, resolved.Renew)
	_, err := resolved.Renew(context.Background())
	require.NoError(t, err)
	urls := rec.all()
	require.Len(t, urls, 2)
	assert.Contains(t, urls[1], "/usenet/requestdl")
	assert.Contains(t, urls[1], "file_id=3")
}

func TestNewPlayerExec_UsesTheProxyUnlessItIsTurnedOff(t *testing.T) {
	renew := func(context.Context) (string, error) { return "https://cdn.example/fresh.mkv", nil }
	msg := StreamURLResolvedMsg{URL: "https://cdn.example/a.mkv", Renew: renew}

	on := testModel(t)
	off := testModelWith(t, func(c *config.Config) { c.Player.StreamProxy = false })

	assert.NotNil(t, on.newPlayerExec(msg).renew)
	assert.Nil(t, off.newPlayerExec(msg).renew)
	assert.Equal(t, "https://cdn.example/a.mkv", off.newPlayerExec(msg).media.URL)
}

func TestPlayerExec_PlaysThroughTheLocalProxyWhenTheLinkCanBeRenewed(t *testing.T) {
	fp := &fakePlayer{}
	e := &playerExec{
		ctx:    context.Background(),
		player: fp,
		tail:   &outputTail{},
		media:  player.MediaStream{URL: "https://cdn.example/a.mkv?token=signed"},
		renew:  func(context.Context) (string, error) { return "https://cdn.example/b.mkv", nil },
	}
	e.SetStdout(io.Discard)
	e.SetStderr(io.Discard)

	require.NoError(t, e.Run())

	assert.True(t, strings.HasPrefix(fp.got.URL, "http://127.0.0.1:"), "got %q", fp.got.URL)
	assert.NotContains(t, fp.got.URL, "signed")
}

// fetchingPlayer reads from the URL it's given, the way mpv would, then exits.
type fetchingPlayer struct{}

func (fetchingPlayer) Play(_ context.Context, media player.MediaStream) (*player.Session, error) {
	resp, err := http.Get(media.URL)
	if err == nil {
		_ = resp.Body.Close()
	}
	done := make(chan struct{})
	close(done)
	return &player.Session{Done: done}, nil
}

func TestPlayerExec_ReportsALinkThatExpiredAndCouldNotBeRenewed(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(cdn.Close)
	e := &playerExec{
		ctx:    context.Background(),
		player: fetchingPlayer{},
		tail:   &outputTail{},
		media:  player.MediaStream{URL: cdn.URL + "/a.mkv"},
		renew:  func(context.Context) (string, error) { return "", errors.New("torbox is down") },
	}
	e.SetStdout(io.Discard)
	e.SetStderr(io.Discard)

	assert.ErrorIs(t, e.Run(), stream.ErrRenewFailed)
}

func TestPlaybackFinished_SaysTheLinkExpired(t *testing.T) {
	tail := &outputTail{}
	_, _ = tail.Write([]byte("[ffmpeg] https: HTTP error 403 Forbidden\n"))

	msg := playbackFinished("mpv", tail)(fmt.Errorf("playing: %w", stream.ErrRenewFailed))

	assert.Equal(t, PlaybackFinishedMsg{Text: "Stream link expired and couldn't be renewed", IsErr: true}, msg)
}
