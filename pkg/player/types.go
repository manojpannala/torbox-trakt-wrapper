package player

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type MediaStream struct {
	URL        string
	Title      string
	Parsed     matcher.ParsedMedia
	ResumeSecs float64
	ExtraArgs  []string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type PlaybackProgress struct {
	TimePos    float64 `json:"time_pos"`
	PercentPos float64 `json:"percent_pos"`
	Duration   float64 `json:"duration"`
	Paused     bool    `json:"paused"`
}

type ScrobbleHandler interface {
	Start(ctx context.Context, media matcher.ParsedMedia, progress float64) error
	Pause(ctx context.Context, media matcher.ParsedMedia, progress float64) error
	Stop(ctx context.Context, media matcher.ParsedMedia, progress float64) (*trakt.ScrobbleResponse, error)
}

type Player interface {
	Play(ctx context.Context, media MediaStream) (*Session, error)
}

type Session struct {
	Cmd        *exec.Cmd
	SocketPath string
	Done       chan struct{}
	Err        error
	controller atomic.Pointer[Monitor]
}

func (s *Session) Wait() error {
	<-s.Done
	return s.Err
}

func (s *Session) Close() error {
	if c := s.controller.Load(); c != nil {
		c.Stop()
	}
	if s.Cmd != nil && s.Cmd.Process != nil {
		err := s.Cmd.Process.Kill()
		if err != nil && (strings.Contains(err.Error(), "already finished") || strings.Contains(err.Error(), "process already finished")) {
			return nil
		}
		return err
	}
	return nil
}
