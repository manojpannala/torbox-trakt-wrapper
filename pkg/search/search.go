// Package search finds releases for a chosen work through a Searcher backend
// and ranks them. It knows nothing about any one backend.
package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

type Kind int

const (
	Movie Kind = iota
	Show
)

// Work is what the releases are for: a Trakt hit, a library row, or a typed
// IMDb ID. Season and Episode are zero for a whole show or season.
type Work struct {
	Kind    Kind
	Title   string
	Year    int
	IMDbID  string
	Season  int
	Episode int
}

// Indexer is one source a backend can search on its own.
type Indexer struct {
	ID        int
	Name      string
	MovieIMDb bool
	TVIMDb    bool
}

// Request is one search against one indexer.
type Request struct {
	IndexerID  int
	Indexer    string
	Type       string
	Query      string
	Categories []int
	// ByID results are exact, so they skip the title filter.
	ByID bool
	// Raw results skip the title filter too: the user typed the query.
	Raw bool
}

// Result is one release. Resolve is set when the backend could not give a
// hash but can fetch one; it is never serialised.
type Result struct {
	Title     string              `json:"title"`
	Hash      string              `json:"hash,omitempty"`
	Size      int64               `json:"size"`
	Seeders   int                 `json:"seeders"`
	Indexers  []string            `json:"indexers"`
	IndexerID int                 `json:"-"`
	GUID      string              `json:"-"`
	Pack      bool                `json:"pack,omitempty"`
	Media     matcher.ParsedMedia `json:"-"`

	Resolve func(ctx context.Context) (string, error) `json:"-"`
}

// Key identifies a release across searches, even before it has a hash.
func (r Result) Key() string {
	return fmt.Sprintf("%d/%s", r.IndexerID, r.GUID)
}

// Pending reports a release that still needs resolving before it can be
// badge-checked or added.
func (r Result) Pending() bool {
	return r.Hash == "" && r.Resolve != nil
}

// Searcher is a search backend.
type Searcher interface {
	Indexers(ctx context.Context) ([]Indexer, error)
	Search(ctx context.Context, req Request) ([]Result, error)
}

var (
	ErrUnauthorized = errors.New("search: the backend rejected the API key")
	ErrRateLimited  = errors.New("search: the indexer is rate limited")
	ErrUnreachable  = errors.New("search: the backend is unreachable")
	// ErrNotMagnet means the download link answered with something other
	// than a magnet redirect, such as a .torrent file.
	ErrNotMagnet = errors.New("search: the release did not resolve to a magnet")
)

// SearchTimeout bounds one request to one indexer, so a slow indexer only
// delays its own results.
const SearchTimeout = 60 * time.Second

const (
	catMovies = 2000
	catTV     = 5000
)

// Plan lists the requests a search for w sends: an ID search to every
// indexer that supports one, and a text search to every indexer.
func Plan(indexers []Indexer, w Work) []Request {
	var reqs []Request
	for _, ix := range indexers {
		if w.IMDbID != "" {
			if w.Kind == Movie && ix.MovieIMDb {
				reqs = append(reqs, Request{
					IndexerID: ix.ID, Indexer: ix.Name, Type: "movie",
					Query: "{ImdbId:" + w.IMDbID + "}", Categories: []int{catMovies}, ByID: true,
				})
			}
			if w.Kind == Show && ix.TVIMDb {
				q := "{ImdbId:" + w.IMDbID + "}"
				if w.Season > 0 {
					q += fmt.Sprintf("{Season:%d}", w.Season)
				}
				if w.Episode > 0 {
					q += fmt.Sprintf("{Episode:%d}", w.Episode)
				}
				reqs = append(reqs, Request{
					IndexerID: ix.ID, Indexer: ix.Name, Type: "tvsearch",
					Query: q, Categories: []int{catTV}, ByID: true,
				})
			}
		}
		if text := textQuery(w); text != "" {
			cat := catMovies
			if w.Kind == Show {
				cat = catTV
			}
			reqs = append(reqs, Request{
				IndexerID: ix.ID, Indexer: ix.Name, Type: "search",
				Query: text, Categories: []int{cat},
			})
		}
	}
	return reqs
}

// PlanRaw sends text to every indexer as typed, with no category and no filter.
func PlanRaw(indexers []Indexer, text string) []Request {
	reqs := make([]Request, 0, len(indexers))
	for _, ix := range indexers {
		reqs = append(reqs, Request{IndexerID: ix.ID, Indexer: ix.Name, Type: "search", Query: strings.TrimSpace(text), Raw: true})
	}
	return reqs
}

func textQuery(w Work) string {
	title := queryTitle(w.Title)
	if title == "" {
		return ""
	}
	switch {
	case w.Kind == Movie && w.Year > 0:
		return fmt.Sprintf("%s %d", title, w.Year)
	case w.Kind == Show && w.Season > 0 && w.Episode > 0:
		return fmt.Sprintf("%s S%02dE%02d", title, w.Season, w.Episode)
	case w.Kind == Show && w.Season > 0:
		return fmt.Sprintf("%s S%02d", title, w.Season)
	default:
		return title
	}
}

// queryTitle drops the punctuation release names never carry, so an
// indexer's term matching sees the same words.
func queryTitle(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r == '\'' || r == '\u2019':
		case r == '&':
			b.WriteString(" and ")
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
