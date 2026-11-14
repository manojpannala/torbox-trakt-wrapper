package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
			if r.URL.Query().Get("query") == "control codes" {
				_, _ = w.Write([]byte(`[{"type":"movie","movie":{"title":"Sample\u001b[2J Film\u009b","year":2014,"ids":{"trakt":3,"imdb":"tt0000003\u0007"}}}]`))
				return
			}
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

func TestCLI_Search_TitlesDropControlCharacters(t *testing.T) {
	cfgPath, _ := searchServers(t)

	out, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json=false", "--sort", "cached", "control", "codes")
	require.NoError(t, err)
	assert.Regexp(t, `tt0000003\s+movie\s+Sample\[2J Film \(2014\)`, out)
	assert.NotContains(t, out, "\x1b")
	assert.NotContains(t, out, "\x07")
	assert.NotContains(t, out, "\u009b")
}

func TestCLI_Search_ReleasesCarryBadgesAndMagnets(t *testing.T) {
	cfgPath, checked := searchServers(t)

	out, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json=false", "--sort", "cached", "tt0000001")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 6, "two section headers plus two releases with magnet lines:\n%s", out)
	assert.Equal(t, "── 4K · 1 of 1 ──", lines[0])
	assert.Regexp(t, `^○\s+-\s+.*\s+70\s+General\s+Sample Film \(2014\) 2160p WEB-DL$`, lines[1], "B is resolved from its download link and ranks in 4K")
	assert.Equal(t, "magnet:?xt=urn:btih:"+cliHashB, strings.TrimSpace(lines[2]))
	assert.Equal(t, "── 1080p · 1 of 1 ──", lines[3])
	assert.Regexp(t, `^●\s+-\s+.*\s+40\s+\S*MovieIDs\S*\s+Sample\.Film\.2014\.1080p\.BluRay-GRP$`, lines[4], "A is cached and keeps its best-seeded name")
	assert.Contains(t, lines[4], "General", "A's duplicate folds in its indexer")
	assert.Equal(t, "magnet:?xt=urn:btih:"+cliHashA, strings.TrimSpace(lines[5]))
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

	_, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json=false", "--sort", "best", "tt0000001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--sort must be one of rank, size, seeders")
}

func TestCLI_Search_GroupedHeadersCapsAllDropsCachedOnlyJSONAndLog(t *testing.T) {
	for _, name := range []string{"TORBOX_API_KEY", "PROWLARR_API_KEY", "TRAKT_CLIENT_ID", "TRAKT_CLIENT_SECRET", "TRAKT_ACCESS_TOKEN", "TRAKT_REFRESH_TOKEN"} {
		t.Setenv(name, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	traktSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"type":"movie","movie":{"title":"Sample Film","year":2014,"language":"ta","ids":{"trakt":1,"imdb":"tt0000001"}}}]`))
	}))
	t.Cleanup(traktSrv.Close)
	t.Setenv("TRAKT_BASE_URL", traktSrv.URL)

	h4k1 := strings.Repeat("1", 40)
	h4k2 := strings.Repeat("2", 40)
	h1080 := strings.Repeat("3", 40)
	hDead := strings.Repeat("4", 40)
	hCam := strings.Repeat("5", 40)

	var prowlarrHits atomic.Int32
	var checked []string
	prowlarrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prowlarrHits.Add(1)
		switch r.URL.Path {
		case "/api/v1/indexer":
			_, _ = w.Write([]byte(`[{"id":1,"name":"General","enable":true,"protocol":"torrent","capabilities":{"movieSearchParams":["q"]}}]`))
		case "/api/v1/search":
			_, _ = fmt.Fprintf(w, `[
				{"guid":"g1","title":"Sample.Film.2014.2160p.Tamil.x265-GRP","size":5000000000,"seeders":30,"indexer":"General","indexerId":1,"protocol":"torrent","infoHash":"%s"},
				{"guid":"g2","title":"Sample.Film.2014.2160p.Hindi.x264-ENC","size":5000000000,"seeders":80,"indexer":"General","indexerId":1,"protocol":"torrent","infoHash":"%s"},
				{"guid":"g3","title":"Sample.Film.2014.1080p.Tamil.x265-GRP","size":2000000000,"seeders":20,"indexer":"General","indexerId":1,"protocol":"torrent","infoHash":"%s"},
				{"guid":"g4","title":"Sample.Film.2014.1080p.Dead-GRP","size":2000000000,"seeders":0,"indexer":"General","indexerId":1,"protocol":"torrent","infoHash":"%s"},
				{"guid":"g5","title":"Sample.Film.2014.1080p.CAM-GRP","size":2000000000,"seeders":50,"indexer":"General","indexerId":1,"protocol":"torrent","infoHash":"%s"}
			]`, h4k1, h4k2, h1080, hDead, hCam)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(prowlarrSrv.Close)

	torboxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct{ Hashes []string }
		_ = json.Unmarshal(body, &req)
		checked = append(checked, req.Hashes...)
		_, _ = w.Write([]byte(`{"success":true,"data":{"` + h1080 + `":{"name":"x","size":1,"hash":"` + h1080 + `"}}}`))
	}))
	t.Cleanup(torboxSrv.Close)
	t.Setenv("TORBOX_BASE_URL", torboxSrv.URL)

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "dummy-api-key"
	cfg.Trakt.ClientID = "dummy-client-id"
	cfg.Search.ProwlarrURL = prowlarrSrv.URL
	cfg.Search.ProwlarrAPIKey = searchKey
	cfg.Search.Resolutions = map[string]int{"2160p": 1, "1080p": 5}
	require.NoError(t, cfg.SaveToFile(cfgPath))

	t.Cleanup(func() {
		_, _ = executeCommand("--config", cfgPath, "--verbose=false", "version")
	})

	// 1. Capped 4K (1 of 2) before 1080p (1 of 1); 0-seeder and CAM dropped; Tamil ranks above Hindi.
	out, err := executeCommand("--config", cfgPath, "--verbose", "search", "--raw=false", "--json=false", "--sort", "rank", "--cached-only=false", "--all=false", "tt0000001")
	require.NoError(t, err)
	idx4K := strings.Index(out, "── 4K · 1 of 2 ──")
	idx1080 := strings.Index(out, "── 1080p · 1 of 1 ──")
	require.NotEqual(t, -1, idx4K, out)
	require.NotEqual(t, -1, idx1080, out)
	assert.Less(t, idx4K, idx1080)
	assert.Contains(t, out, "2160p.Tamil")
	assert.NotContains(t, out, "2160p.Hindi")
	assert.NotContains(t, out, "Dead-GRP")
	assert.NotContains(t, out, "CAM-GRP")
	assert.ElementsMatch(t, []string{h4k1, h1080}, checked)

	// Summary log written with no release title.
	logBytes, err := os.ReadFile(filepath.Join(stateDir, "torbox-trakt-wrapper", "tt-wrapper.log")) // #nosec G304
	require.NoError(t, err)
	logText := string(logBytes)
	assert.Contains(t, logText, "search summary")
	assert.NotContains(t, logText, "Sample.Film")

	// 2. --all prints every surviving row.
	outAll, err := executeCommand("--config", cfgPath, "--verbose=false", "search", "--raw=false", "--json=false", "--sort", "rank", "--cached-only=false", "--all", "tt0000001")
	require.NoError(t, err)
	assert.Contains(t, outAll, "── 4K · 2 of 2 ──")
	assert.Contains(t, outAll, "2160p.Hindi")

	// 3. --cached-only prints only the cached row (1080p).
	outCached, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json=false", "--sort", "rank", "--cached-only", "--all=false", "tt0000001")
	require.NoError(t, err)
	assert.NotContains(t, outCached, "── 4K")
	assert.Contains(t, outCached, "── 1080p · 1 of 1 ──")
	assert.Contains(t, outCached, "1080p.Tamil")

	// 4. --json rows carry group and language.
	outJSON, err := executeCommand("--config", cfgPath, "search", "--raw=false", "--json", "--sort", "cached", "--cached-only=false", "--all=false", "tt0000001")
	require.NoError(t, err)
	var jrows []struct {
		Group    string `json:"group"`
		Language string `json:"language"`
	}
	require.NoError(t, json.Unmarshal([]byte(outJSON), &jrows), outJSON)
	require.Len(t, jrows, 2)
	assert.Equal(t, "2160p", jrows[0].Group)
	assert.Equal(t, "tamil", jrows[0].Language)

	// 5. Invalid [search.resolutions] key errors before any request hits Prowlarr.
	badCfgPath := filepath.Join(t.TempDir(), "bad.toml")
	cfg.Search.Resolutions = map[string]int{"1440p": 5}
	require.NoError(t, cfg.SaveToFile(badCfgPath))
	before := prowlarrHits.Load()
	_, err = executeCommand("--config", badCfgPath, "search", "--raw=false", "--json=false", "--sort", "rank", "--cached-only=false", "--all=false", "tt0000001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1440p")
	assert.Equal(t, before, prowlarrHits.Load())
}
