package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type Option func(*MPVPlayer)

func WithExecutable(exe string) Option {
	return func(p *MPVPlayer) {
		if exe != "" {
			p.executable = exe
		}
	}
}

func WithExtraArgs(args []string) Option {
	return func(p *MPVPlayer) {
		p.extraArgs = args
	}
}

func WithScrobbler(s ScrobbleHandler) Option {
	return func(p *MPVPlayer) {
		p.scrobbler = s
	}
}

func WithSocketDir(dir string) Option {
	return func(p *MPVPlayer) {
		p.socketDir = dir
	}
}

func WithIPCEnabled(enabled bool) Option {
	return func(p *MPVPlayer) {
		p.ipcEnabled = enabled
	}
}

type MPVPlayer struct {
	executable string
	extraArgs  []string
	socketDir  string
	ipcEnabled bool
	scrobbler  ScrobbleHandler
}

func NewMPVPlayer(opts ...Option) *MPVPlayer {
	socketDir := filepath.Join(config.GetConfigDir(), "sockets")
	if err := config.EnsureSecureDir(socketDir); err != nil {
		socketDir = os.TempDir()
	}

	p := &MPVPlayer{
		executable: "mpv",
		socketDir:  socketDir,
		ipcEnabled: true,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

func (p *MPVPlayer) Play(ctx context.Context, media MediaStream) (*Session, error) {
	if media.URL == "" {
		return nil, fmt.Errorf("empty media stream url")
	}

	var socketPath string
	var args []string

	if media.Title != "" {
		args = append(args, fmt.Sprintf("--force-media-title=%s", media.Title))
	}

	if p.ipcEnabled {
		socketPath = filepath.Join(p.socketDir, fmt.Sprintf("mpv-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
		_ = os.Remove(socketPath)
		args = append(args, fmt.Sprintf("--input-ipc-server=%s", socketPath))
	}

	if media.ResumeSecs > 0 {
		args = append(args, fmt.Sprintf("--start=%d", int(media.ResumeSecs)))
	}

	args = append(args, p.extraArgs...)
	args = append(args, media.ExtraArgs...)
	args = append(args, media.URL)

	cmd := exec.CommandContext(ctx, p.executable, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		if socketPath != "" {
			_ = os.Remove(socketPath)
		}
		return nil, fmt.Errorf("failed to start mpv: %w", err)
	}

	session := &Session{
		Cmd:        cmd,
		SocketPath: socketPath,
		Done:       make(chan struct{}),
	}

	if p.ipcEnabled && socketPath != "" {
		go func() {
			dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			client, err := DialIPC(dialCtx, socketPath, 5*time.Second)
			if err == nil {
				monitor := NewMonitor(client, media.Parsed, p.scrobbler, socketPath)
				session.controller = monitor
				monitor.Start()
			}
		}()
	}

	go func() {
		defer close(session.Done)
		defer func() {
			if socketPath != "" {
				_ = os.Remove(socketPath)
			}
		}()

		err := cmd.Wait()
		if session.controller != nil {
			session.controller.Stop()
		}
		session.Err = err
	}()

	return session, nil
}

type TraktScrobbler struct {
	client *trakt.Client
}

func NewTraktScrobbler(client *trakt.Client) *TraktScrobbler {
	return &TraktScrobbler{client: client}
}

func (s *TraktScrobbler) Start(ctx context.Context, media matcher.ParsedMedia, progress float64) error {
	req := s.buildScrobbleRequest(media, progress)
	_, err := s.client.StartScrobble(ctx, req)
	return err
}

func (s *TraktScrobbler) Pause(ctx context.Context, media matcher.ParsedMedia, progress float64) error {
	req := s.buildScrobbleRequest(media, progress)
	_, err := s.client.PauseScrobble(ctx, req)
	return err
}

func (s *TraktScrobbler) Stop(ctx context.Context, media matcher.ParsedMedia, progress float64) (*trakt.ScrobbleResponse, error) {
	req := s.buildScrobbleRequest(media, progress)
	return s.client.StopScrobble(ctx, req)
}

func (s *TraktScrobbler) buildScrobbleRequest(media matcher.ParsedMedia, progress float64) trakt.ScrobbleRequest {
	req := trakt.ScrobbleRequest{
		Progress: progress,
	}

	if media.Type == matcher.MediaTypeEpisode {
		season := media.Season
		if season == 0 {
			season = 1
		}
		req.Show = &trakt.Show{
			Title: media.CleanTitle,
		}
		req.Episode = &trakt.Episode{
			Season: season,
			Number: media.Episode,
		}
	} else {
		req.Movie = &trakt.Movie{
			Title: media.CleanTitle,
			Year:  media.Year,
		}
	}

	return req
}
