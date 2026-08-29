package tui

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
)

type fakePlayer struct {
	got     player.MediaStream
	playErr error
}

func (f *fakePlayer) Play(_ context.Context, media player.MediaStream) (*player.Session, error) {
	f.got = media
	if f.playErr != nil {
		return nil, f.playErr
	}
	done := make(chan struct{})
	close(done)
	return &player.Session{Done: done}, nil
}

func TestPlayerExec_PassesMediaThroughToThePlayer(t *testing.T) {
	fp := &fakePlayer{}
	tail := &outputTail{}
	e := &playerExec{
		player: fp,
		tail:   tail,
		media: player.MediaStream{
			URL:        "https://example.invalid/a.mkv",
			Title:      "Some Show",
			ResumeSecs: 42,
		},
	}
	e.SetStdout(io.Discard)
	e.SetStderr(io.Discard)

	require.NoError(t, e.Run())

	assert.Equal(t, "https://example.invalid/a.mkv", fp.got.URL)
	assert.Equal(t, "Some Show", fp.got.Title)
	assert.Equal(t, float64(42), fp.got.ResumeSecs)
	assert.NotNil(t, fp.got.Stdout, "player must receive the tee'd stdout")
	assert.NotNil(t, fp.got.Stderr, "player must receive the tee'd stderr")
}

func TestPlayerExec_TeesOutputIntoTheTail(t *testing.T) {
	fp := &fakePlayer{}
	tail := &outputTail{}
	e := &playerExec{player: fp, tail: tail}
	e.SetStdout(io.Discard)

	require.NoError(t, e.Run())
	_, _ = fp.got.Stdout.Write([]byte("Error parsing option bogus (option not found)\n"))

	assert.Equal(t, "Error parsing option bogus (option not found)", tail.errorLine())
}

func TestPlayerExec_ReturnsPlayError(t *testing.T) {
	fp := &fakePlayer{playErr: errors.New("mpv not found")}
	e := &playerExec{player: fp, tail: &outputTail{}}
	e.SetStdout(io.Discard)

	assert.EqualError(t, e.Run(), "mpv not found")
}
