package cache_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/cache"
)

const version = "v1.2.3"

func newStore(t *testing.T, ttl time.Duration) (*cache.Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	return cache.New(dir, ttl, version, nil), dir
}

func writeEnvelope(t *testing.T, dir string, key cache.Key, ver string, storedAt time.Time, payload any) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	p, err := json.Marshal(payload)
	require.NoError(t, err)
	data, err := json.Marshal(map[string]any{
		"version":   ver,
		"stored_at": storedAt.Unix(),
		"payload":   json.RawMessage(p),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, string(key)+".json"), data, 0o600))
}

func TestReadAfterWriteIsFresh(t *testing.T) {
	s, _ := newStore(t, time.Hour)
	cache.Write(s, cache.TorBoxTorrents, []string{"a", "b"})

	e, ok := cache.Read[[]string](s, cache.TorBoxTorrents)

	require.True(t, ok)
	assert.Equal(t, []string{"a", "b"}, e.Value)
	assert.True(t, e.Fresh)
	assert.WithinDuration(t, time.Now(), e.StoredAt, 2*time.Second)
}

func TestStaleEntryIsStillReturned(t *testing.T) {
	s, dir := newStore(t, 15*time.Minute)
	writeEnvelope(t, dir, cache.TorBoxUsenet, version, time.Now().Add(-time.Hour), []string{"old"})

	e, ok := cache.Read[[]string](s, cache.TorBoxUsenet)

	require.True(t, ok, "a stale hit is still worth rendering")
	assert.False(t, e.Fresh)
	assert.Equal(t, []string{"old"}, e.Value)
}

func TestMissingKeyIsAMiss(t *testing.T) {
	s, _ := newStore(t, time.Hour)

	_, ok := cache.Read[[]string](s, cache.TorBoxWebDL)

	assert.False(t, ok)
}

func TestCorruptFileIsAMiss(t *testing.T) {
	s, dir := newStore(t, time.Hour)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "torbox-torrents.json"), []byte("{not json"), 0o600))

	_, ok := cache.Read[[]string](s, cache.TorBoxTorrents)

	assert.False(t, ok)
}

func TestWrongPayloadShapeIsAMiss(t *testing.T) {
	s, dir := newStore(t, time.Hour)
	writeEnvelope(t, dir, cache.TorBoxTorrents, version, time.Now(), map[string]int{"x": 1})

	_, ok := cache.Read[[]string](s, cache.TorBoxTorrents)

	assert.False(t, ok)
}

func TestVersionMismatchIsAMiss(t *testing.T) {
	s, dir := newStore(t, time.Hour)
	writeEnvelope(t, dir, cache.TraktCatalog, "v0.0.1", time.Now(), []string{"x"})

	_, ok := cache.Read[[]string](s, cache.TraktCatalog)

	assert.False(t, ok, "a file from another release may decode into partly-zeroed structs")
}

func TestZeroTTLIsNeverFresh(t *testing.T) {
	s, _ := newStore(t, 0)
	cache.Write(s, cache.TorBoxTorrents, []string{"a"})

	e, ok := cache.Read[[]string](s, cache.TorBoxTorrents)

	require.True(t, ok)
	assert.False(t, e.Fresh)
}

func TestEmptyPayloadOverwrites(t *testing.T) {
	s, _ := newStore(t, time.Hour)
	cache.Write(s, cache.TorBoxTorrents, []string{"deleted-later"})
	cache.Write(s, cache.TorBoxTorrents, []string{})

	e, ok := cache.Read[[]string](s, cache.TorBoxTorrents)

	require.True(t, ok)
	assert.Empty(t, e.Value, "an empty library must not resurrect deleted items")
}

func TestPermissions(t *testing.T) {
	s, dir := newStore(t, time.Hour)
	cache.Write(s, cache.TorBoxTorrents, []string{"a"})

	fi, err := os.Stat(filepath.Join(dir, "torbox-torrents.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())

	di, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), di.Mode().Perm())
}

func TestConcurrentWritersNeverLeaveATornFile(t *testing.T) {
	s, dir := newStore(t, time.Hour)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cache.Write(s, cache.TorBoxTorrents, []string{fmt.Sprintf("writer-%d", n)})
		}(i)
	}
	wg.Wait()

	e, ok := cache.Read[[]string](s, cache.TorBoxTorrents)
	require.True(t, ok, "the file on disk must always be a complete envelope")
	require.Len(t, e.Value, 1)

	leftovers, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	require.NoError(t, err)
	assert.Empty(t, leftovers)
}

func TestInvalidateRemovesOneKey(t *testing.T) {
	s, _ := newStore(t, time.Hour)
	cache.Write(s, cache.TorBoxTorrents, []string{"a"})
	cache.Write(s, cache.TraktCatalog, []string{"b"})

	s.Invalidate(cache.TorBoxTorrents)

	_, torrents := cache.Read[[]string](s, cache.TorBoxTorrents)
	_, trakt := cache.Read[[]string](s, cache.TraktCatalog)
	assert.False(t, torrents)
	assert.True(t, trakt)
}

func TestClearRemovesEverythingIncludingLeftoverTempFiles(t *testing.T) {
	s, dir := newStore(t, time.Hour)
	for _, k := range []cache.Key{cache.TorBoxTorrents, cache.TorBoxUsenet, cache.TorBoxWebDL, cache.TraktCatalog} {
		cache.Write(s, k, []string{"x"})
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "torbox-torrents-123.tmp"), []byte("x"), 0o600))

	s.Clear()

	left, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, left)
}

func TestNilStoreIsSafe(t *testing.T) {
	var s *cache.Store

	assert.NotPanics(t, func() {
		cache.Write(s, cache.TorBoxTorrents, []string{"a"})
		_, ok := cache.Read[[]string](s, cache.TorBoxTorrents)
		assert.False(t, ok)
		s.Invalidate(cache.TorBoxTorrents)
		s.Clear()
	})
}

func TestAccountVersion_DiffersPerAccountAndRelease(t *testing.T) {
	base := cache.AccountVersion("v1.2.3", "torbox:key-A")

	assert.Equal(t, base, cache.AccountVersion("v1.2.3", "torbox:key-A"))
	assert.NotEqual(t, base, cache.AccountVersion("v1.2.3", "torbox:key-B"))
	assert.NotEqual(t, base, cache.AccountVersion("v1.2.4", "torbox:key-A"))
	assert.NotContains(t, base, "key-A")
}

func TestAccountVersion_KeepsAnotherAccountsDataOut(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	storeA := cache.New(dir, time.Hour, cache.AccountVersion(version, "torbox:key-A"), nil)
	cache.Write(storeA, cache.TorBoxUsenet, []string{"a"})

	storeB := cache.New(dir, time.Hour, cache.AccountVersion(version, "torbox:key-B"), nil)
	_, ok := cache.Read[[]string](storeB, cache.TorBoxUsenet)
	assert.False(t, ok, "account B must not see account A's cached data")

	storeA2 := cache.New(dir, time.Hour, cache.AccountVersion(version, "torbox:key-A"), nil)
	e, ok := cache.Read[[]string](storeA2, cache.TorBoxUsenet)
	require.True(t, ok)
	assert.Equal(t, []string{"a"}, e.Value)
}

func TestUnwritableDirectoryIsNotFatal(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	s := cache.New(filepath.Join(blocker, "cache"), time.Hour, version, nil)

	assert.NotPanics(t, func() { cache.Write(s, cache.TorBoxTorrents, []string{"a"}) })
	_, ok := cache.Read[[]string](s, cache.TorBoxTorrents)
	assert.False(t, ok)
}
