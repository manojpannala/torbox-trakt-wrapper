package prowlarr_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search/prowlarr"
)

const (
	testKey   = "prowlarr-test-key-0123456789"
	sampleHex = "0123456789abcdef0123456789abcdef01234567"
)

// The shape of GET /api/v1/indexer, trimmed to what we read plus a few
// fields we ignore.
const indexersJSON = `[
 {"id":1,"name":"General","enable":true,"protocol":"torrent","redirect":false,
  "capabilities":{"movieSearchParams":["q"],"tvSearchParams":["q","season","ep"]}},
 {"id":7,"name":"MovieIDs","enable":true,"protocol":"torrent",
  "capabilities":{"movieSearchParams":["q","imdbId"],"tvSearchParams":[]}},
 {"id":8,"name":"TVIDs","enable":true,"protocol":"torrent",
  "capabilities":{"movieSearchParams":["q"],"tvSearchParams":["q","imdbId","season","ep"]}},
 {"id":3,"name":"Disabled","enable":false,"protocol":"torrent","capabilities":{}},
 {"id":4,"name":"Usenet","enable":true,"protocol":"usenet","capabilities":{}}
]`

// The shape of GET /api/v1/search. {{BASE}} is the test server's URL, as
// Prowlarr builds its links from the address it was reached on.
const releasesJSON = `[
 {"guid":"https://site.invalid/t/1","title":"Sample.Film.2014.1080p.BluRay.x264-GRP","size":2000,
  "seeders":40,"leechers":3,"indexer":"General","indexerId":1,"protocol":"torrent",
  "infoHash":"0123456789ABCDEF0123456789ABCDEF01234567","categories":[{"id":2000,"name":"Movies"}],
  "downloadUrl":"{{BASE}}/1/download?apikey=prowlarr-test-key-0123456789&link=abc&file=x",
  "magnetUrl":"{{BASE}}/1/download?apikey=prowlarr-test-key-0123456789&link=def&file=x"},
 {"guid":"https://site.invalid/t/2","title":"Sample.Film.2014.720p.WEB-DL-GRP","size":900,
  "seeders":12,"indexer":"General","indexerId":1,"protocol":"torrent","infoHash":null,
  "downloadUrl":"{{BASE}}/1/download?apikey=prowlarr-test-key-0123456789&link=ghi&file=y"},
 {"guid":"https://site.invalid/t/3","title":"Sample.Film.2014.2160p-GRP","size":9000,
  "seeders":null,"indexer":"General","indexerId":1,"protocol":"torrent","infoHash":"",
  "downloadUrl":"https://elsewhere.invalid/1/download?link=jkl"},
 {"guid":"nzb-1","title":"Sample.Film.2014.1080p-NZB","size":1,"indexer":"General","indexerId":1,
  "protocol":"usenet","downloadUrl":"{{BASE}}/1/download?link=mno"},
 {"guid":"https://site.invalid/t/5","title":"Sample.Film.2014.480p-GRP","size":500,
  "seeders":2,"indexer":"General","indexerId":1,"protocol":"torrent","infoHash":"not-a-hash"}
]`

type fake struct {
	*httptest.Server
	hits     atomic.Int32
	lastPath atomic.Value
	download http.HandlerFunc
}

func newFake(t *testing.T, urlBase string) *fake {
	t.Helper()
	f := &fake{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.lastPath.Store(r.URL.RequestURI())
		assert.Empty(t, r.URL.Query().Get("apikey"), "the key never goes in a URL")
		if r.Header.Get("X-Api-Key") != testKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch path := strings.TrimPrefix(r.URL.Path, urlBase); {
		case path == "/api/v1/indexer":
			_, _ = w.Write([]byte(indexersJSON))
		case path == "/api/v1/search":
			_, _ = w.Write([]byte(strings.ReplaceAll(releasesJSON, "{{BASE}}", f.URL+urlBase)))
		case strings.HasSuffix(path, "/download") && f.download != nil:
			f.download(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func client(t *testing.T, f *fake, urlBase string) *prowlarr.Client {
	t.Helper()
	c, err := prowlarr.New(f.URL+urlBase+"/", testKey)
	require.NoError(t, err)
	return c
}

func TestNew_PlainHTTPOnlyToThisMachine(t *testing.T) {
	for _, ok := range []string{"http://127.0.0.1:9696", "http://localhost:9696/prowlarr", "http://[::1]:9696", "http://127.8.0.1", "https://prowlarr.example.invalid"} {
		_, err := prowlarr.New(ok, testKey)
		assert.NoError(t, err, ok)
	}
	for _, bad := range []string{"http://192.168.1.5:9696", "http://prowlarr.example.invalid"} {
		_, err := prowlarr.New(bad, testKey)
		assert.ErrorIs(t, err, prowlarr.ErrInsecureURL, bad)
	}
	for _, bad := range []string{"", "127.0.0.1:9696", "ftp://127.0.0.1", "http://"} {
		_, err := prowlarr.New(bad, testKey)
		assert.Error(t, err, bad)
	}
}

func TestIndexers_KeepsEnabledTorrentIndexersAndTheirIDSupport(t *testing.T) {
	f := newFake(t, "")

	got, err := client(t, f, "").Indexers(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []search.Indexer{
		{ID: 1, Name: "General"},
		{ID: 7, Name: "MovieIDs", MovieIMDb: true},
		{ID: 8, Name: "TVIDs", TVIMDb: true},
	}, got)
}

func TestSearch_AsksOneIndexer(t *testing.T) {
	f := newFake(t, "")

	_, err := client(t, f, "").Search(context.Background(), search.Request{
		IndexerID: 7, Type: "movie", Query: "{ImdbId:tt0000001}", Categories: []int{2000},
	})

	require.NoError(t, err)
	assert.Equal(t, "/api/v1/search?categories=2000&indexerIds=7&limit=100&query=%7BImdbId%3Att0000001%7D&type=movie", f.lastPath.Load())
}

func TestSearch_MapsReleases(t *testing.T) {
	f := newFake(t, "")

	got, err := client(t, f, "").Search(context.Background(), search.Request{IndexerID: 1, Type: "search", Query: "x"})

	require.NoError(t, err)
	require.Len(t, got, 2, "no usable hash and no link of ours means the release is dropped; usenet is skipped")
	assert.Equal(t, sampleHex, got[0].Hash)
	assert.Equal(t, "Sample.Film.2014.1080p.BluRay.x264-GRP", got[0].Title)
	assert.Equal(t, int64(2000), got[0].Size)
	assert.Equal(t, 40, got[0].Seeders)
	assert.Equal(t, []string{"General"}, got[0].Indexers)
	assert.Equal(t, "1/https://site.invalid/t/1", got[0].Key())
	assert.Nil(t, got[0].Resolve, "a release with a hash needs no resolve")
	assert.True(t, got[1].Pending())
}

func TestResolve_ReadsTheMagnetFromTheRedirectWithTheKeyInTheHeader(t *testing.T) {
	f := newFake(t, "")
	f.download = func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "ghi", r.URL.Query().Get("link"))
		http.Redirect(w, r, "magnet:?xt=urn:btih:"+strings.ToUpper(sampleHex)+"&dn=x", http.StatusMovedPermanently)
	}
	rows, err := client(t, f, "").Search(context.Background(), search.Request{IndexerID: 1, Type: "search", Query: "x"})
	require.NoError(t, err)

	hash, err := rows[1].Resolve(context.Background())

	require.NoError(t, err)
	assert.Equal(t, sampleHex, hash)
}

func TestResolve_UnderAURLBase(t *testing.T) {
	f := newFake(t, "/prowlarr")
	f.download = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "magnet:?xt=urn:btih:"+sampleHex, http.StatusFound)
	}
	rows, err := client(t, f, "/prowlarr").Search(context.Background(), search.Request{IndexerID: 1, Type: "search", Query: "x"})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	hash, err := rows[1].Resolve(context.Background())

	require.NoError(t, err)
	assert.Equal(t, sampleHex, hash)
}

func TestResolve_NeverFollowsARedirectThatIsNotAMagnet(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		assert.Empty(t, r.Header.Get("X-Api-Key"))
	}))
	defer other.Close()
	f := newFake(t, "")
	f.download = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/file.torrent", http.StatusFound)
	}
	rows, err := client(t, f, "").Search(context.Background(), search.Request{IndexerID: 1, Type: "search", Query: "x"})
	require.NoError(t, err)

	_, err = rows[1].Resolve(context.Background())

	assert.ErrorIs(t, err, search.ErrNotMagnet)
	assert.Zero(t, elsewhere.Load(), "the redirect target is never fetched")
}

func TestResolve_ATorrentBodyIsNotAMagnet(t *testing.T) {
	f := newFake(t, "")
	f.download = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write([]byte("d8:announce0:e"))
	}
	rows, err := client(t, f, "").Search(context.Background(), search.Request{IndexerID: 1, Type: "search", Query: "x"})
	require.NoError(t, err)

	_, err = rows[1].Resolve(context.Background())

	assert.ErrorIs(t, err, search.ErrNotMagnet)
}

func TestResolve_AGrabLimitIsRateLimited(t *testing.T) {
	f := newFake(t, "")
	f.download = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}
	rows, err := client(t, f, "").Search(context.Background(), search.Request{IndexerID: 1, Type: "search", Query: "x"})
	require.NoError(t, err)

	_, err = rows[1].Resolve(context.Background())

	assert.ErrorIs(t, err, search.ErrRateLimited)
	assert.Equal(t, "limited", prowlarr.Describe(err))
}

func TestErrors_ARejectedKey(t *testing.T) {
	f := newFake(t, "")
	c, err := prowlarr.New(f.URL, "wrong-key")
	require.NoError(t, err)

	_, err = c.Indexers(context.Background())

	assert.ErrorIs(t, err, search.ErrUnauthorized)
	assert.Equal(t, "Prowlarr rejected the API key", prowlarr.Describe(err))
}

func TestErrors_NothingListening(t *testing.T) {
	f := newFake(t, "")
	c := client(t, f, "")
	host := strings.TrimPrefix(f.URL, "http://")
	f.Close()

	_, err := c.Search(context.Background(), search.Request{IndexerID: 1, Type: "search", Query: "x"})

	assert.ErrorIs(t, err, search.ErrUnreachable)
	assert.Equal(t, "can't reach Prowlarr at "+host+". Is it running?", prowlarr.Describe(err))
	assert.NotContains(t, err.Error(), "query=", "the request URL is left out of the error")
}

func TestErrors_ACancelledSearchReportsTheCancel(t *testing.T) {
	f := newFake(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client(t, f, "").Search(ctx, search.Request{IndexerID: 1, Type: "search", Query: "x"})

	assert.True(t, errors.Is(err, context.Canceled), err)
}

func TestErrors_TheKeyNeverAppears(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(testKey))
	}))
	defer srv.Close()
	c, err := prowlarr.New(srv.URL, testKey)
	require.NoError(t, err)

	_, err = c.Search(context.Background(), search.Request{IndexerID: 1, Type: "search", Query: "x"})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), testKey)
	assert.NotContains(t, prowlarr.Describe(err), testKey)
}
