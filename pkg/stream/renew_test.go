package stream_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/stream"
)

// signedServer serves testFile only to the token in valid, and answers 403 to
// any other, the way an expired signed link is refused.
type signedServer struct {
	*httptest.Server
	valid    atomic.Value
	requests sync.Map // token -> *atomic.Int32
}

func newSignedServer(t *testing.T, valid string) *signedServer {
	t.Helper()
	s := &signedServer{}
	s.valid.Store(valid)
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		n, _ := s.requests.LoadOrStore(token, new(atomic.Int32))
		n.(*atomic.Int32).Add(1)
		if token != s.valid.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.ServeContent(w, r, "video.mkv", time.Time{}, bytes.NewReader(testFile))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *signedServer) link(token string) string {
	return s.URL + "/dld/video.mkv?token=" + token
}

func (s *signedServer) requestsFor(token string) int32 {
	n, ok := s.requests.Load(token)
	if !ok {
		return 0
	}
	return n.(*atomic.Int32).Load()
}

// countingRenewer hands out token-1, token-2, … and counts the calls.
type countingRenewer struct {
	server *signedServer
	calls  atomic.Int32
	delay  time.Duration
}

func (c *countingRenewer) renew(ctx context.Context) (string, error) {
	n := c.calls.Add(1)
	if _, ok := ctx.Deadline(); !ok {
		return "", errors.New("renewal has no deadline")
	}
	time.Sleep(c.delay)
	return c.server.link(fmt.Sprintf("token-%d", n)), nil
}

func TestRenew_AnExpiredLinkIsRenewedAndTheRequestRetried(t *testing.T) {
	t.Parallel()
	up := newSignedServer(t, "token-1")
	renewer := &countingRenewer{server: up}
	p := startProxy(t, up.link("expired"), renewer.renew)

	resp, body := get(t, http.MethodGet, p.URL(), http.Header{"Range": {"bytes=5000-5999"}})

	assert.Equal(t, http.StatusPartialContent, resp.StatusCode)
	assert.Equal(t, testFile[5000:6000], body)
	assert.Equal(t, int32(1), renewer.calls.Load())
	assert.False(t, p.RenewFailed())
}

func TestRenew_LaterRequestsGoStraightToTheRenewedLink(t *testing.T) {
	t.Parallel()
	up := newSignedServer(t, "token-1")
	renewer := &countingRenewer{server: up}
	p := startProxy(t, up.link("expired"), renewer.renew)

	first, _ := get(t, http.MethodGet, p.URL(), http.Header{"Range": {"bytes=0-99"}})
	second, _ := get(t, http.MethodGet, p.URL(), http.Header{"Range": {"bytes=100-199"}})

	assert.Equal(t, http.StatusPartialContent, first.StatusCode)
	assert.Equal(t, http.StatusPartialContent, second.StatusCode)
	assert.Equal(t, int32(1), up.requestsFor("expired"), "the expired link is tried once, then dropped")
	assert.Equal(t, int32(2), up.requestsFor("token-1"))
	assert.Equal(t, int32(1), renewer.calls.Load())
}

func TestRenew_RequestsThatExpireTogetherShareOneRenewal(t *testing.T) {
	t.Parallel()
	up := newSignedServer(t, "token-1")
	renewer := &countingRenewer{server: up, delay: 50 * time.Millisecond}
	p := startProxy(t, up.link("expired"), renewer.renew)

	var wg sync.WaitGroup
	statuses := make([]int, 8)
	for i := range statuses {
		wg.Go(func() {
			resp, _ := get(t, http.MethodGet, p.URL(), http.Header{"Range": {fmt.Sprintf("bytes=%d-%d", i*100, i*100+99)}})
			statuses[i] = resp.StatusCode
		})
	}
	wg.Wait()

	for i, status := range statuses {
		assert.Equal(t, http.StatusPartialContent, status, "request %d", i)
	}
	assert.Equal(t, int32(1), renewer.calls.Load())
}

func TestRenew_WhenTorBoxWontGiveANewLinkTheRejectionIsPassedOn(t *testing.T) {
	t.Parallel()
	up := newSignedServer(t, "never-issued")
	var logs bytes.Buffer
	renew := func(context.Context) (string, error) {
		return "", errors.New(`Get "https://api.torbox.app/v1/api/torrents/requestdl?token=tb-api-key": dial tcp: no route to host`)
	}
	p := startProxy(t, up.link("secret-signature"), renew, stream.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))))

	resp, _ := get(t, http.MethodGet, p.URL(), nil)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.True(t, p.RenewFailed())
	assert.Contains(t, logs.String(), "stream link renewal failed")
	assert.NotContains(t, logs.String(), "tb-api-key")
	assert.NotContains(t, logs.String(), "secret-signature")
}

func TestRenew_AFreshLinkThatIsAlsoRefusedGivesUp(t *testing.T) {
	t.Parallel()
	up := newSignedServer(t, "never-issued")
	renewer := &countingRenewer{server: up}
	p := startProxy(t, up.link("expired"), renewer.renew)

	resp, _ := get(t, http.MethodGet, p.URL(), nil)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, int32(1), renewer.calls.Load(), "one renewal per request, never a loop")
	assert.True(t, p.RenewFailed())
}

func TestRenew_NoMoreThanFiveRenewalsPerPlayback(t *testing.T) {
	t.Parallel()
	up := newSignedServer(t, "token-1")
	renewer := &countingRenewer{server: up}
	p := startProxy(t, up.link("expired"), renewer.renew, stream.WithMinRenewGap(0))

	for i := 1; i <= 5; i++ {
		up.valid.Store(fmt.Sprintf("token-%d", i))
		resp, _ := get(t, http.MethodGet, p.URL(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, "renewal %d", i)
	}
	up.valid.Store("token-6")
	resp, _ := get(t, http.MethodGet, p.URL(), nil)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, int32(5), renewer.calls.Load())
	assert.True(t, p.RenewFailed())
}

func TestRenew_NoSecondRenewalWithinTenSeconds(t *testing.T) {
	t.Parallel()
	up := newSignedServer(t, "token-1")
	renewer := &countingRenewer{server: up}
	p := startProxy(t, up.link("expired"), renewer.renew)

	first, _ := get(t, http.MethodGet, p.URL(), nil)
	up.valid.Store("token-2")
	second, _ := get(t, http.MethodGet, p.URL(), nil)

	assert.Equal(t, http.StatusOK, first.StatusCode)
	assert.Equal(t, http.StatusForbidden, second.StatusCode)
	assert.Equal(t, int32(1), renewer.calls.Load())
	assert.True(t, p.RenewFailed())
}

func TestRenew_OtherUpstreamErrorsAreNotTreatedAsExpiry(t *testing.T) {
	t.Parallel()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(up.Close)
	var calls atomic.Int32
	renew := func(context.Context) (string, error) { calls.Add(1); return up.URL, nil }
	p := startProxy(t, up.URL+"/dld/video.mkv", renew)

	resp, _ := get(t, http.MethodGet, p.URL(), nil)

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Zero(t, calls.Load())
	assert.False(t, p.RenewFailed())
}
