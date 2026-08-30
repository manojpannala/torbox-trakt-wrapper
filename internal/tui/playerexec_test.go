package tui

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
)

type fakePlayer struct {
	got            player.MediaStream
	playErr        error
	blockUntilDone bool
}

func (f *fakePlayer) Play(ctx context.Context, media player.MediaStream) (*player.Session, error) {
	f.got = media
	if f.playErr != nil {
		return nil, f.playErr
	}
	done := make(chan struct{})
	if f.blockUntilDone {
		go func() {
			<-ctx.Done()
			close(done)
		}()
	} else {
		close(done)
	}
	return &player.Session{Done: done}, nil
}

func TestPlayerExec_PassesMediaThroughToThePlayer(t *testing.T) {
	fp := &fakePlayer{}
	tail := &outputTail{}
	e := &playerExec{
		ctx:    context.Background(),
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
	e := &playerExec{ctx: context.Background(), player: fp, tail: tail}
	e.SetStdout(io.Discard)

	require.NoError(t, e.Run())
	_, _ = fp.got.Stdout.Write([]byte("Error parsing option bogus (option not found)\n"))

	assert.Equal(t, "Error parsing option bogus (option not found)", tail.errorLine())
}

func TestPlayerExec_ReturnsPlayError(t *testing.T) {
	fp := &fakePlayer{playErr: errors.New("mpv not found")}
	e := &playerExec{ctx: context.Background(), player: fp, tail: &outputTail{}}
	e.SetStdout(io.Discard)

	assert.EqualError(t, e.Run(), "mpv not found")
}

func TestPlayerExec_CancellingTheContextEndsTheSession(t *testing.T) {
	fp := &fakePlayer{blockUntilDone: true}
	ctx, cancel := context.WithCancel(context.Background())
	e := &playerExec{ctx: ctx, player: fp, tail: &outputTail{}}
	e.SetStdout(io.Discard)

	done := make(chan error, 1)
	go func() { done <- e.Run() }()

	select {
	case <-done:
		t.Fatal("playback returned before the context was cancelled")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("cancelling the context did not end the player session")
	}
}
