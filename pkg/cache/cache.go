// Package cache persists API listings between runs so the TUI can render
// before the network answers.
package cache

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

// Key names a cacheable payload. The set is closed: adding a key is a
// deliberate assertion that the payload holds nothing signed or secret.
type Key string

const (
	TorBoxTorrents Key = "torbox-torrents"
	TorBoxUsenet   Key = "torbox-usenet"
	TorBoxWebDL    Key = "torbox-webdl"
	TraktCatalog   Key = "trakt-catalog"
)

var allKeys = []Key{TorBoxTorrents, TorBoxUsenet, TorBoxWebDL, TraktCatalog}

type Entry[T any] struct {
	Value    T
	StoredAt time.Time
	Fresh    bool
}

type Store struct {
	dir     string
	ttl     time.Duration
	version string
	log     *slog.Logger
}

type envelope struct {
	Version  string          `json:"version"`
	StoredAt int64           `json:"stored_at"`
	Payload  json.RawMessage `json:"payload"`
}

func New(dir string, ttl time.Duration, version string, log *slog.Logger) *Store {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Store{dir: dir, ttl: ttl, version: version, log: log}
}

func (s *Store) path(key Key) string {
	return filepath.Join(s.dir, string(key)+".json")
}

func Read[T any](s *Store, key Key) (Entry[T], bool) {
	var e Entry[T]
	if s == nil {
		return e, false
	}

	raw, err := os.ReadFile(s.path(key)) // #nosec G304 -- key comes from a closed set
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.log.Debug("cache read failed", "key", key, "err", err)
		}
		return e, false
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		s.log.Debug("cache envelope corrupt", "key", key, "err", err)
		return e, false
	}
	if env.Version != s.version {
		s.log.Debug("cache version mismatch", "key", key, "have", env.Version, "want", s.version)
		return e, false
	}
	if err := json.Unmarshal(env.Payload, &e.Value); err != nil {
		s.log.Debug("cache payload corrupt", "key", key, "err", err)
		return Entry[T]{}, false
	}

	e.StoredAt = time.Unix(env.StoredAt, 0)
	e.Fresh = s.ttl > 0 && time.Since(e.StoredAt) < s.ttl
	return e, true
}

func Write[T any](s *Store, key Key, v T) {
	if s == nil {
		return
	}

	payload, err := json.Marshal(v)
	if err != nil {
		s.log.Debug("cache marshal failed", "key", key, "err", err)
		return
	}
	data, err := json.Marshal(envelope{Version: s.version, StoredAt: time.Now().Unix(), Payload: payload})
	if err != nil {
		return
	}

	if err = config.EnsureSecureDir(s.dir); err != nil {
		s.log.Debug("cache dir unavailable", "dir", s.dir, "err", err)
		return
	}

	// A unique temp name per writer: a fixed one lets two processes clobber
	// each other and rename a torn file into place.
	tmp, err := os.CreateTemp(s.dir, string(key)+"-*.tmp")
	if err != nil {
		s.log.Debug("cache temp file failed", "key", key, "err", err)
		return
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmpName)
		s.log.Debug("cache write failed", "key", key, "err", errors.Join(werr, cerr))
		return
	}

	if err := os.Rename(tmpName, s.path(key)); err != nil {
		_ = os.Remove(tmpName)
		s.log.Debug("cache rename failed", "key", key, "err", err)
	}
}

func (s *Store) Invalidate(key Key) {
	if s == nil {
		return
	}
	if err := os.Remove(s.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.log.Debug("cache invalidate failed", "key", key, "err", err)
	}
}

func (s *Store) Clear() {
	if s == nil {
		return
	}
	for _, k := range allKeys {
		s.Invalidate(k)
	}
	leftovers, _ := filepath.Glob(filepath.Join(s.dir, "*.tmp"))
	for _, f := range leftovers {
		_ = os.Remove(f)
	}
}
