package player

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
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

// WithKeepOpen overrides mpv's keep-open setting for the launch. Accepts
// "yes", "no" or "always"; anything else leaves the user's mpv.conf alone.
func WithKeepOpen(value string) Option {
	return func(p *MPVPlayer) {
		switch value {
		case "yes", "no", "always":
			p.keepOpen = value
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(p *MPVPlayer) {
		if logger != nil {
			p.logger = logger
		}
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
	keepOpen   string
	scrobbler  ScrobbleHandler
	logger     *slog.Logger
}

func NewMPVPlayer(opts ...Option) *MPVPlayer {
	socketDir := filepath.Join(config.GetConfigDir(), "sockets")
	if err := config.EnsureSecureDir(socketDir); err != nil {
		// MkdirTemp is 0700; os.TempDir is world-readable and the ipc
		// socket accepts arbitrary mpv commands.
		if tmp, tmpErr := os.MkdirTemp("", "tt-wrapper-sockets-"); tmpErr == nil {
			socketDir = tmp
		} else {
			socketDir = os.TempDir()
		}
	}

	p := &MPVPlayer{
		executable: "mpv",
		socketDir:  socketDir,
		ipcEnabled: true,
		logger:     slog.New(slog.DiscardHandler),
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

	if media.ResumeAtPercent > 0 {
		args = append(args, fmt.Sprintf("--start=%.1f%%", media.ResumeAtPercent))
	}

	if p.keepOpen != "" {
		args = append(args, fmt.Sprintf("--keep-open=%s", p.keepOpen))
	}

	args = append(args, p.extraArgs...)
	args = append(args, media.ExtraArgs...)

	p.logger.Debug("launching player",
		"executable", p.executable,
		"args", args,
		"host", streamHost(media.URL))

	// -- stops mpv parsing an api-supplied url as an option.
	cmd := exec.CommandContext(ctx, p.executable, append(args, "--", media.URL)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if media.Stdin != nil {
		cmd.Stdin = media.Stdin
	}
	if media.Stdout != nil {
		cmd.Stdout = media.Stdout
	}
	if media.Stderr != nil {
		cmd.Stderr = media.Stderr
	}

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
				monitor := NewMonitor(client, media.Parsed, p.scrobbler, socketPath, p.logger)
				session.controller.Store(monitor)
				monitor.Start(ctx)
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
		if c := session.controller.Load(); c != nil {
			c.Stop()
		}
		session.Err = err
	}()

	return session, nil
}

type TraktScrobbler struct {
	client *trakt.Client
	logger *slog.Logger

	mu  sync.Mutex
	ids map[string]trakt.IDs
}

type ScrobblerOption func(*TraktScrobbler)

// WithScrobblerLogger records failed title lookups, which are otherwise silent.
func WithScrobblerLogger(logger *slog.Logger) ScrobblerOption {
	return func(s *TraktScrobbler) {
		if logger != nil {
			s.logger = logger
		}
	}
}

func NewTraktScrobbler(client *trakt.Client, opts ...ScrobblerOption) *TraktScrobbler {
	s := &TraktScrobbler{client: client, logger: slog.New(slog.DiscardHandler)}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func buildScrobbleRequest(media matcher.ParsedMedia, progress float64, ids trakt.IDs) trakt.ScrobbleRequest {
	req := trakt.ScrobbleRequest{Progress: progress}

	if media.Type == matcher.MediaTypeEpisode {
		season := media.Season
		if season == 0 {
			season = 1
		}
		req.Show = &trakt.Show{Title: media.CleanTitle, IDs: ids}
		req.Episode = &trakt.Episode{Season: season, Number: media.Episode}
	} else {
		req.Movie = &trakt.Movie{Title: media.CleanTitle, Year: media.Year, IDs: ids}
	}

	return req
}

// resolve finds media's Trakt id, because a release is usually named
// differently from Trakt's canonical title and scrobbling by title alone 404s.
// A hit or a genuine miss is memoised; a failed lookup is not, so a blip does
// not poison the title for the life of the process.
func (s *TraktScrobbler) resolve(ctx context.Context, media matcher.ParsedMedia) trakt.IDs {
	key := fmt.Sprintf("%s|%s|%d", media.Type, matcher.NormalizeTitle(media.CleanTitle), media.Year)

	s.mu.Lock()
	cached, ok := s.ids[key]
	s.mu.Unlock()
	if ok {
		return cached
	}

	ids, err := s.searchIDs(ctx, media)
	if err != nil {
		s.logger.Debug("trakt title lookup failed", "title", media.CleanTitle, "err", err)
		return trakt.IDs{}
	}

	s.mu.Lock()
	if s.ids == nil {
		s.ids = make(map[string]trakt.IDs)
	}
	s.ids[key] = ids
	s.mu.Unlock()

	return ids
}

func (s *TraktScrobbler) searchIDs(ctx context.Context, media matcher.ParsedMedia) (trakt.IDs, error) {
	if media.Type == matcher.MediaTypeEpisode {
		shows, err := s.client.SearchShows(ctx, media.CleanTitle, media.Year)
		if err != nil {
			return trakt.IDs{}, err
		}
		for _, show := range shows {
			if sameItem(media, show.Title, show.Year) {
				return show.IDs, nil
			}
		}
		return trakt.IDs{}, nil
	}

	movies, err := s.client.SearchMovies(ctx, media.CleanTitle, media.Year)
	if err != nil {
		return trakt.IDs{}, err
	}
	for _, movie := range movies {
		if sameItem(media, movie.Title, movie.Year) {
			return movie.IDs, nil
		}
	}
	return trakt.IDs{}, nil
}

// sameItem keeps a search hit only when it is unambiguously the same thing;
// scrobbling the wrong item is worse than not scrobbling.
func sameItem(media matcher.ParsedMedia, title string, year int) bool {
	if matcher.NormalizeTitle(title) != matcher.NormalizeTitle(media.CleanTitle) {
		return false
	}
	return media.Year == 0 || year == 0 || media.Year == year
}

func (s *TraktScrobbler) Start(ctx context.Context, media matcher.ParsedMedia, progress float64) error {
	if s.client == nil {
		return nil
	}
	req := buildScrobbleRequest(media, progress, s.resolve(ctx, media))
	_, err := s.client.StartScrobble(ctx, req)
	return err
}

func (s *TraktScrobbler) Pause(ctx context.Context, media matcher.ParsedMedia, progress float64) error {
	if s.client == nil {
		return nil
	}
	req := buildScrobbleRequest(media, progress, s.resolve(ctx, media))
	_, err := s.client.PauseScrobble(ctx, req)
	return err
}

func (s *TraktScrobbler) Stop(ctx context.Context, media matcher.ParsedMedia, progress float64) (*trakt.ScrobbleResponse, error) {
	if s.client == nil {
		return nil, nil
	}
	req := buildScrobbleRequest(media, progress, s.resolve(ctx, media))
	return s.client.StopScrobble(ctx, req)
}

func streamHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
