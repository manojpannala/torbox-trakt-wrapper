// Package stream hands the player a localhost address for a TorBox download
// link, so the signed link behind it can be swapped for a fresh one when it
// expires mid-stream.
package stream

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/logging"
)

const (
	// maxRenewals caps how often one playback asks TorBox for a new link.
	maxRenewals = 5
	// defaultMinRenewGap stops a link that is refused as soon as it is issued
	// from being renewed in a loop.
	defaultMinRenewGap = 10 * time.Second
	renewTimeout       = 15 * time.Second
)

// ErrRenewFailed means the link expired during playback and no new one could
// be had, so the player stopped early.
var ErrRenewFailed = errors.New("stream link expired and could not be renewed")

// Renewer asks TorBox for a new download link for the same file.
type Renewer func(ctx context.Context) (string, error)

// forwardedHeaders are the upstream response headers the player needs to
// size, seek and resume the stream.
var forwardedHeaders = []string{
	"Accept-Ranges",
	"Content-Length",
	"Content-Range",
	"Content-Type",
	"ETag",
	"Last-Modified",
}

type Proxy struct {
	renew       Renewer
	client      *http.Client
	logger      *slog.Logger
	path        string
	listener    net.Listener
	server      *http.Server
	minRenewGap time.Duration

	mu          sync.Mutex
	upstream    string
	generation  uint64
	renewals    int
	lastRenewal time.Time
	renewFailed bool
}

type Option func(*Proxy)

// WithLogger sends the proxy's logs to logger. Links are never logged.
func WithLogger(logger *slog.Logger) Option {
	return func(p *Proxy) {
		if logger != nil {
			p.logger = logger
		}
	}
}

// WithHTTPClient replaces the client used to reach TorBox.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Proxy) {
		if client != nil {
			p.client = client
		}
	}
}

// Start serves upstream on a random loopback port, behind a random path so
// other local users can't find it. When TorBox refuses the link as expired,
// renew is asked for a new one; a nil renew never renews.
func Start(upstream string, renew Renewer, opts ...Option) (*Proxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &Proxy{
		renew:       renew,
		client:      defaultClient(),
		logger:      slog.New(slog.DiscardHandler),
		path:        "/" + rand.Text(),
		listener:    listener,
		minRenewGap: defaultMinRenewGap,
		upstream:    upstream,
	}
	for _, opt := range opts {
		opt(p)
	}
	p.server = &http.Server{
		Handler:           p,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(p.logger.Handler(), slog.LevelDebug),
	}
	go func() { _ = p.server.Serve(listener) }()
	return p, nil
}

// URL is the address to give the player in place of the TorBox link.
func (p *Proxy) URL() string {
	return "http://" + p.listener.Addr().String() + p.path
}

// RenewFailed reports whether the link expired and couldn't be renewed, which
// explains a player that stopped early.
func (p *Proxy) RenewFailed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.renewFailed
}

// Close stops the proxy and ends any stream still being served.
func (p *Proxy) Close() error {
	return p.server.Close()
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != p.path {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	upstream, generation := p.current()
	resp, err := p.fetch(r, upstream)
	if err == nil && linkRefused(resp.StatusCode) && p.renew != nil {
		refused := resp.StatusCode
		_ = resp.Body.Close()
		fresh, ok := p.renewAfter(r.Context(), generation)
		if !ok {
			w.WriteHeader(refused)
			return
		}
		resp, err = p.fetch(r, fresh)
		if err == nil && linkRefused(resp.StatusCode) {
			p.logger.Warn("stream renewed link was refused too", "status", resp.StatusCode)
			p.mu.Lock()
			p.renewFailed = true
			p.mu.Unlock()
		}
	}
	if err != nil {
		p.logger.Warn("stream upstream request failed", "err", withoutURL(err))
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for _, key := range forwardedHeaders {
		if v := resp.Header.Get(key); v != "" {
			w.Header().Set(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodGet {
		_, _ = io.Copy(w, resp.Body)
	}
}

func (p *Proxy) current() (string, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.upstream, p.generation
}

// renewAfter returns a link newer than generation, asking TorBox for one
// unless another request already has. Requests that find the link expired
// together wait on the lock and share the one renewal. It reports false once
// the renewals are used up or TorBox won't give a new link.
func (p *Proxy) renewAfter(ctx context.Context, generation uint64) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.generation != generation {
		return p.upstream, true
	}
	if p.renewFailed {
		return "", false
	}
	if p.renewals >= maxRenewals || (!p.lastRenewal.IsZero() && time.Since(p.lastRenewal) < p.minRenewGap) {
		p.logger.Warn("stream link renewal failed", "err", "renewed too often", "renewals", p.renewals)
		p.renewFailed = true
		return "", false
	}
	p.renewals++
	p.lastRenewal = time.Now()

	// The player may drop this request while the renewal runs; the requests
	// waiting behind it still want the new link.
	renewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), renewTimeout)
	defer cancel()
	link, err := p.renew(renewCtx)
	if err == nil && link == "" {
		err = errors.New("empty link")
	}
	if err != nil {
		p.logger.Warn("stream link renewal failed", "err", logging.Redact(withoutURL(err)), "renewals", p.renewals)
		p.renewFailed = true
		return "", false
	}
	p.upstream = link
	p.generation++
	p.logger.Info("stream link renewed", "renewals", p.renewals)
	return link, true
}

func (p *Proxy) fetch(r *http.Request, upstream string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream, nil)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"Range", "If-Range"} {
		if v := r.Header.Get(key); v != "" {
			req.Header.Set(key, v)
		}
	}
	return p.client.Do(req) // #nosec G704 -- upstream is the TorBox link the user chose to play
}

// linkRefused reports the answers TorBox's CDN gives a signed link that has
// expired.
func linkRefused(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusGone:
		return true
	}
	return false
}

// defaultClient never asks for compression, which would make Go drop
// Content-Length and Content-Range, and never times out a whole response,
// because a stream can run for hours.
func defaultClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{Transport: transport}
}

// withoutURL drops the link an *url.Error carries: a signed link is a
// credential.
func withoutURL(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err.Error()
	}
	return err.Error()
}
