// Package prowlarr searches through the user's own Prowlarr, one request per
// indexer, over its native JSON API.
package prowlarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/logging"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

const (
	indexersTimeout = 10 * time.Second
	resultLimit     = 100
	maxBody         = 16 << 20
)

// ErrInsecureURL rejects plain http to anything but this machine: the API
// key would cross the network in the clear.
var ErrInsecureURL = errors.New("prowlarr_url must use https unless it points at this machine")

// UnreachableError means no answer came back from Prowlarr at all.
type UnreachableError struct{ Host string }

func (e *UnreachableError) Error() string {
	return "can't reach Prowlarr at " + e.Host
}

func (e *UnreachableError) Is(target error) bool {
	return target == search.ErrUnreachable
}

// Client is a search.Searcher backed by Prowlarr.
type Client struct {
	base   *url.URL
	apiKey string
	http   *http.Client
	// download matches the path of Prowlarr's own download links.
	download *regexp.Regexp
}

var _ search.Searcher = (*Client)(nil)

// New checks rawURL and returns a client for it. The client never follows
// redirects: a download link's redirect is read, not fetched.
func New(rawURL, apiKey string) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("prowlarr_url %q is not an http(s) URL", logging.Redact(rawURL))
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return nil, ErrInsecureURL
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment, u.User = "", "", nil
	return &Client{
		base:     u,
		apiKey:   apiKey,
		download: regexp.MustCompile(`^` + regexp.QuoteMeta(u.Path) + `/\d+/download$`),
		http: &http.Client{
			Timeout: search.SearchTimeout + 5*time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Host is the host:port searches go to, for messages.
func (c *Client) Host() string {
	return c.base.Host
}

type indexerResource struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Enable       bool   `json:"enable"`
	Protocol     string `json:"protocol"`
	Capabilities struct {
		MovieSearchParams []string `json:"movieSearchParams"`
		TVSearchParams    []string `json:"tvSearchParams"`
	} `json:"capabilities"`
}

// Indexers lists the enabled torrent indexers.
func (c *Client) Indexers(ctx context.Context) ([]search.Indexer, error) {
	ctx, cancel := context.WithTimeout(ctx, indexersTimeout)
	defer cancel()

	var list []indexerResource
	if err := c.get(ctx, "/api/v1/indexer", nil, &list); err != nil {
		return nil, err
	}
	out := make([]search.Indexer, 0, len(list))
	for _, ix := range list {
		if !ix.Enable || ix.Protocol != "torrent" {
			continue
		}
		out = append(out, search.Indexer{
			ID:        ix.ID,
			Name:      ix.Name,
			MovieIMDb: slices.Contains(ix.Capabilities.MovieSearchParams, "imdbId"),
			TVIMDb:    slices.Contains(ix.Capabilities.TVSearchParams, "imdbId"),
		})
	}
	return out, nil
}

type release struct {
	GUID        string    `json:"guid"`
	Title       string    `json:"title"`
	Size        int64     `json:"size"`
	Seeders     *int      `json:"seeders"`
	Indexer     string    `json:"indexer"`
	IndexerID   int       `json:"indexerId"`
	InfoHash    string    `json:"infoHash"`
	Protocol    string    `json:"protocol"`
	DownloadURL proxyLink `json:"downloadUrl"`
}

// proxyLink is a Prowlarr download link with its apikey parameter removed
// while decoding, so the key is never held anywhere but the header.
type proxyLink string

func (l *proxyLink) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*l = proxyLink(stripKey(s))
	return nil
}

func stripKey(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Del("apikey")
	u.RawQuery = q.Encode()
	return u.String()
}

// Search runs one request against one indexer. Releases with no usable
// hash come back pending, with Resolve set, when Prowlarr can fetch one.
func (c *Client) Search(ctx context.Context, req search.Request) ([]search.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, search.SearchTimeout)
	defer cancel()

	q := url.Values{}
	q.Set("query", req.Query)
	q.Set("type", req.Type)
	q.Set("indexerIds", strconv.Itoa(req.IndexerID))
	q.Set("limit", strconv.Itoa(resultLimit))
	for _, cat := range req.Categories {
		q.Add("categories", strconv.Itoa(cat))
	}

	var releases []release
	if err := c.get(ctx, "/api/v1/search", q, &releases); err != nil {
		return nil, err
	}
	out := make([]search.Result, 0, len(releases))
	for _, r := range releases {
		if r.Protocol != "torrent" {
			continue
		}
		res := search.Result{
			Title:     r.Title,
			Hash:      search.NormalizeHash(r.InfoHash),
			Size:      r.Size,
			Indexers:  []string{r.Indexer},
			IndexerID: r.IndexerID,
			GUID:      r.GUID,
		}
		if r.Seeders != nil {
			res.Seeders = *r.Seeders
		}
		if res.Hash == "" {
			link := string(r.DownloadURL)
			if !c.ownsDownload(link) {
				continue
			}
			res.Resolve = func(ctx context.Context) (string, error) { return c.resolve(ctx, link) }
		}
		out = append(out, res)
	}
	return out, nil
}

// ownsDownload accepts only links back to this Prowlarr's download
// endpoint, so a result can't point the key-bearing request anywhere else.
func (c *Client) ownsDownload(link string) bool {
	u, err := url.Parse(link)
	return err == nil && u.User == nil &&
		u.Scheme == c.base.Scheme && strings.EqualFold(u.Host, c.base.Host) &&
		c.download.MatchString(u.Path)
}

func (c *Client) resolve(ctx context.Context, link string) (string, error) {
	if !c.ownsDownload(link) {
		return "", search.ErrNotMagnet
	}
	ctx, cancel := context.WithTimeout(ctx, search.ResolveTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stripKey(link), nil)
	if err != nil {
		return "", search.ErrNotMagnet
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", c.transportErr(ctx)
	}
	_ = resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound:
		loc := resp.Header.Get("Location")
		if strings.HasPrefix(loc, "magnet:") {
			if h := search.HashFromMagnet(loc); h != "" {
				return h, nil
			}
		}
		return "", search.ErrNotMagnet
	case http.StatusUnauthorized:
		return "", search.ErrUnauthorized
	case http.StatusTooManyRequests:
		return "", search.ErrRateLimited
	default:
		return "", search.ErrNotMagnet
	}
}

func (c *Client) get(ctx context.Context, path string, q url.Values, target any) error {
	u := *c.base
	u.Path += path
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("building prowlarr request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "torbox-trakt-wrapper/"+config.Version)

	resp, err := c.http.Do(req)
	if err != nil {
		return c.transportErr(ctx)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return search.ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return search.ErrRateLimited
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("prowlarr %s answered %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(target); err != nil {
		return fmt.Errorf("decoding prowlarr %s: %w", path, err)
	}
	return nil
}

// transportErr reports a request that got no response. It never wraps the
// http error, whose text carries the request URL.
func (c *Client) transportErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return &UnreachableError{Host: c.base.Host}
}

// Describe words an error from this package for the user.
func Describe(err error) string {
	var unreachable *UnreachableError
	switch {
	case errors.As(err, &unreachable):
		return unreachable.Error() + ". Is it running?"
	case errors.Is(err, search.ErrUnauthorized):
		return "Prowlarr rejected the API key"
	case errors.Is(err, search.ErrRateLimited):
		return "limited"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, ErrInsecureURL):
		return err.Error()
	default:
		return logging.Redact(err.Error())
	}
}
