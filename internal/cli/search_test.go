package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

const (
	searchKey = "prowlarr-cli-test-key"
	cliHashA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cliHashB  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// searchServers fakes Trakt, Prowlarr and TorBox for one search: MovieIDs
// answers the ID search, General the text search with a duplicate of A, a
// hashless B to resolve and a different film the filter drops.
func searchServers(t *testing.T) (cfgPath string, checked *[]string) {
	t.Helper()
	for _, name := range []string{"TORBOX_API_KEY", "PROWLARR_API_KEY", "TRAKT_CLIENT_ID", "TRAKT_CLIENT_SECRET", "TRAKT_ACCESS_TOKEN", "TRAKT_REFRESH_TOKEN"} {
		t.Setenv(name, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	traktSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/imdb/tt0000001":
			_, _ = w.Write([]byte(`[{"type":"movie","movie":{"title":"Sample Film","year":2014,"ids":{"trakt":1,"imdb":"tt0000001"}}}]`))
		case "/search/movie,show":
			_, _ = w.Write([]byte(`[{"type":"movie","movie":{"title":"Sample Film","year":2014,"ids":{"trakt":1,"imdb":"tt0000001"}}},
				{"type":"show","show":{"title":"Sample Film: The Series","year":2019,"ids":{"trakt":2,"imdb":"tt0000002"}}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(traktSrv.Close)
	t.Setenv("TRAKT_BASE_URL", traktSrv.URL)

	var prowlarr *httptest.Server
	prowlarr = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.Query().Get("apikey"), "the key never goes in a URL")
		if r.Header.Get("X-Api-Key") != searchKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/v1/indexer":
			_, _ = w.Write([]byte(`[
			 {"id":1,"name":"General","enable":true,"protocol":"torrent","capabilities":{"movieSearchParams":["q"]}},
			 {"id":7,"name":"MovieIDs","enable":true,"protocol":"torrent","capabilities":{"movieSearchParams":["q","imdbId"]}}]`))
		case r.URL.Path == "/api/v1/search" && r.URL.Query().Get("type") == "movie":
			_, _ = w.Write([]byte(`[{"guid":"a7","title":"Sample.Film.2014.1080p.BluRay-GRP","size":2000000000,"seeders":40,
				"indexer":"MovieIDs","indexerId":7,"protocol":"torrent","infoHash":"` + strings.ToUpper(cliHashA) + `"}]`))
		case r.URL.Path == "/api/v1/search" && r.URL.Query().Get("indexerIds") == "1":
			_, _ = w.Write([]byte(`[
			 {"guid":"a1","title":"Sample Film 2014 1080p BluRay","size":2000000000,"seeders":5,
			  "indexer":"General","indexerId":1,"protocol":"torrent","infoHash":"` + cliHashA + `"},
			 {"guid":"b1","title":"Sample Film (2014) 2160p WEB-DL","size":9000000000,"seeders":70,
			  "indexer":"General","indexerId":1,"protocol":"torrent",
			  "downloadUrl":"` + prowlarr.URL + `/1/download?apikey=` + searchKey + `&link=b"},
			 {"guid":"c1","title":"Other Film 2014 1080p","size":1000,"seeders":90,
			  "indexer":"General","indexerId":1,"protocol":"torrent","infoHash":"cccccccccccccccccccccccccccccccccccccccc"}]`))
		case r.URL.Path == "/1/download":
			w.Header().Set("Location", "magnet:?xt=urn:btih:"+cliHashB+"&dn=x")
			w.WriteHeader(http.StatusMovedPermanently)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(prowlarr.Close)

	checked = &[]string{}
	torbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrents/checkcached" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct{ Hashes []string }
		_ = json.Unmarshal(body, &req)
		*checked = append(*checked, req.Hashes...)
		_, _ = w.Write([]byte(`{"success":true,"data":{"` + cliHashA + `":{"name":"x","size":1,"hash":"` + cliHashA + `"}}}`))
	}))
	t.Cleanup(torbox.Close)
	t.Setenv("TORBOX_BASE_URL", torbox.URL)

	cfgPath = filepath.Join(t.TempDir(), "config.toml")
	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "dummy-api-key"
	cfg.Trakt.ClientID = "dummy-client-id"
	cfg.Search.ProwlarrURL = prowlarr.URL
	cfg.Search.ProwlarrAPIKey = searchKey
	require.NoError(t, cfg.SaveToFile(cfgPath))
	return cfgPath, checked
}

func TestCLI_Search_TitlesListTheirIMDbIDs(t *testing.T) {
	cfgPath, checked := searchServers(t)

	out, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json=false", "--sort", "cached", "sample", "film")
	require.NoError(t, err)
	assert.Regexp(t, `tt0000001\s+movie\s+Sample Film \(2014\)`, out)
	assert.Regexp(t, `tt0000002\s+show\s+Sample Film: The Series \(2019\)`, out)
	assert.Empty(t, *checked, "a title search asks TorBox nothing")
}

func TestCLI_Search_ReleasesCarryBadgesAndMagnets(t *testing.T) {
	cfgPath, checked := searchServers(t)

	out, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json=false", "--sort", "cached", "tt0000001")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 4, "two releases, each with its magnet line:\n%s", out)
	assert.Regexp(t, `^●\s+1080p\s+.*\s+40\s+\S*MovieIDs\S*\s+Sample\.Film\.2014\.1080p\.BluRay-GRP$`, lines[0], "A is cached, sorts first, and keeps its best-seeded name")
	assert.Contains(t, lines[0], "General", "A's duplicate folds in its indexer")
	assert.Equal(t, "magnet:?xt=urn:btih:"+cliHashA, strings.TrimSpace(lines[1]))
	assert.Regexp(t, `^○\s+2160p\s+.*\s+70\s+General\s+Sample Film \(2014\) 2160p WEB-DL$`, lines[2], "B is resolved from its download link")
	assert.Equal(t, "magnet:?xt=urn:btih:"+cliHashB, strings.TrimSpace(lines[3]))
	assert.NotContains(t, out, "Other Film", "the title filter drops a different film")
	assert.ElementsMatch(t, []string{cliHashA, cliHashB}, *checked, "one badge batch for both hashes")
}

func TestCLI_Search_JSONSaysCachedOrNot(t *testing.T) {
	cfgPath, _ := searchServers(t)

	out, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json", "--sort", "seeders", "tt0000001")
	require.NoError(t, err)

	var rows []struct {
		Title  string `json:"title"`
		Hash   string `json:"hash"`
		Cached *bool  `json:"cached"`
		Magnet string `json:"magnet"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &rows), out)
	require.Len(t, rows, 2)
	assert.Equal(t, cliHashB, rows[0].Hash, "--sort seeders puts the 70-seeder B first")
	require.NotNil(t, rows[0].Cached)
	assert.False(t, *rows[0].Cached)
	require.NotNil(t, rows[1].Cached)
	assert.True(t, *rows[1].Cached)
	assert.Equal(t, "magnet:?xt=urn:btih:"+cliHashA, rows[1].Magnet)
	assert.NotContains(t, out, searchKey)
}

func TestCLI_Search_OffWithoutProwlarr(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("PROWLARR_API_KEY", "")
	require.NoError(t, config.DefaultConfig().SaveToFile(cfgPath))

	_, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json=false", "--sort", "cached", "tt0000001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search is off")
}

func TestCLI_Search_RejectsAnUnknownSort(t *testing.T) {
	cfgPath, _ := searchServers(t)

	_, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json=false", "--sort", "name", "tt0000001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--sort must be one of")
}
