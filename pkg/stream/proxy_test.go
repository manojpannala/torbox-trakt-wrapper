package stream_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/stream"
)

// testFile stands in for a video: 64 KiB of bytes that differ at every offset
// a test reads from.
var testFile = func() []byte {
	b := make([]byte, 64<<10)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}()

// fileServer serves testFile the way a CDN does, with Range, HEAD and ETag.
func fileServer(t *testing.T, check func(r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			check(r)
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Type", "video/x-matroska")
		http.ServeContent(w, r, "video.mkv", time.Unix(1700000000, 0), bytes.NewReader(testFile))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func startProxy(t *testing.T, upstream string, renew stream.Renewer, opts ...stream.Option) *stream.Proxy {
	t.Helper()
	p, err := stream.Start(upstream, renew, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func get(t *testing.T, method, target string, header http.Header) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, target, nil)
	require.NoError(t, err)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, body
}

func TestProxy_ForwardsARangeRequest(t *testing.T) {
	t.Parallel()
	up := fileServer(t, nil)
	p := startProxy(t, up.URL+"/dld/video.mkv?token=abc", nil)

	resp, body := get(t, http.MethodGet, p.URL(), http.Header{"Range": {"bytes=1000-1999"}})

	assert.Equal(t, http.StatusPartialContent, resp.StatusCode)
	assert.Equal(t, testFile[1000:2000], body)
	assert.Equal(t, "bytes 1000-1999/"+strconv.Itoa(len(testFile)), resp.Header.Get("Content-Range"))
	assert.Equal(t, "1000", resp.Header.Get("Content-Length"))
	assert.Equal(t, "bytes", resp.Header.Get("Accept-Ranges"))
	assert.Equal(t, "video/x-matroska", resp.Header.Get("Content-Type"))
	assert.Equal(t, `"v1"`, resp.Header.Get("ETag"))
	assert.NotEmpty(t, resp.Header.Get("Last-Modified"))
}

func TestProxy_ServesTheWholeFileWithoutARange(t *testing.T) {
	t.Parallel()
	up := fileServer(t, nil)
	p := startProxy(t, up.URL+"/dld/video.mkv", nil)

	resp, body := get(t, http.MethodGet, p.URL(), nil)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, testFile, body)
}

func TestProxy_HeadReportsTheSizeWithoutABody(t *testing.T) {
	t.Parallel()
	up := fileServer(t, nil)
	p := startProxy(t, up.URL+"/dld/video.mkv", nil)

	resp, body := get(t, http.MethodHead, p.URL(), nil)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, strconv.Itoa(len(testFile)), resp.Header.Get("Content-Length"))
	assert.Empty(t, body)
}

func TestProxy_NeverAsksUpstreamToCompress(t *testing.T) {
	t.Parallel()
	// Go's transport asks for gzip on a request without a Range, then strips
	// Content-Length from the answer, which leaves the player unable to seek.
	var acceptEncoding atomic.Value
	up := fileServer(t, func(r *http.Request) { acceptEncoding.Store(r.Header.Get("Accept-Encoding")) })
	p := startProxy(t, up.URL+"/dld/video.mkv", nil)

	resp, _ := get(t, http.MethodGet, p.URL(), nil)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "", acceptEncoding.Load())
}

func TestProxy_ListensOnLoopbackBehindAnUnguessablePath(t *testing.T) {
	t.Parallel()
	up := fileServer(t, nil)
	p := startProxy(t, up.URL+"/dld/video.mkv", nil)

	u, err := url.Parse(p.URL())
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", u.Hostname())
	assert.GreaterOrEqual(t, len(u.Path), 1+26, "the path must carry at least 128 bits of randomness")

	other := startProxy(t, up.URL+"/dld/video.mkv", nil)
	assert.NotEqual(t, u.Path, mustPath(t, other.URL()))
}

func mustPath(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u.Path
}

func TestProxy_RefusesOtherPathsAndMethods(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	up := fileServer(t, func(*http.Request) { hits.Add(1) })
	p := startProxy(t, up.URL+"/dld/video.mkv", nil)
	u, err := url.Parse(p.URL())
	require.NoError(t, err)

	wrongPath, _ := get(t, http.MethodGet, "http://"+u.Host+"/guess", nil)
	post, _ := get(t, http.MethodPost, p.URL(), nil)

	assert.Equal(t, http.StatusNotFound, wrongPath.StatusCode)
	assert.Equal(t, http.StatusMethodNotAllowed, post.StatusCode)
	assert.Zero(t, hits.Load(), "a refused request must not reach TorBox")
}

func TestProxy_UnreachableUpstreamIsABadGatewayAndTheLinkIsNotLogged(t *testing.T) {
	t.Parallel()
	gone := httptest.NewServer(http.NotFoundHandler())
	link := gone.URL + "/dld/video.mkv?token=secret-signature"
	gone.Close()
	var logs bytes.Buffer
	p := startProxy(t, link, nil, stream.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))

	resp, _ := get(t, http.MethodGet, p.URL(), nil)

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	assert.Contains(t, logs.String(), "stream upstream request failed")
	assert.NotContains(t, logs.String(), "secret-signature")
	assert.NotContains(t, logs.String(), "/dld/")
}

func TestProxy_CloseStopsServing(t *testing.T) {
	t.Parallel()
	up := fileServer(t, nil)
	p, err := stream.Start(up.URL+"/dld/video.mkv", nil)
	require.NoError(t, err)
	require.NoError(t, p.Close())

	_, err = http.Get(p.URL())
	assert.Error(t, err)
}
