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
	"time"
)

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
	upstream string
	client   *http.Client
	logger   *slog.Logger
	path     string
	listener net.Listener
	server   *http.Server
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
// other local users can't find it.
func Start(upstream string, _ Renewer, opts ...Option) (*Proxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &Proxy{
		upstream: upstream,
		client:   defaultClient(),
		logger:   slog.New(slog.DiscardHandler),
		path:     "/" + rand.Text(),
		listener: listener,
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

	resp, err := p.fetch(r, p.upstream)
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
