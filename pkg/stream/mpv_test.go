package stream_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

// expiringVideo serves a real video whose first link works for exactly one
// request, like a signed link that expires while mpv is playing.
type expiringVideo struct {
	*httptest.Server
	data []byte

	mu            sync.Mutex
	firstUsed     bool
	furthestFresh int64 // furthest byte offset served on a renewed link
	freshRequests int
}

func newExpiringVideo(t *testing.T) *expiringVideo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "video.mkv")
	gen := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=60:size=320x240:rate=10",
		"-c:v", "mpeg4", "-b:v", "1M", path)
	out, err := gen.CombinedOutput()
	require.NoError(t, err, string(out))
	data, err := os.ReadFile(path) // #nosec G304
	require.NoError(t, err)

	v := &expiringVideo{data: data}
	v.Server = httptest.NewServer(http.HandlerFunc(v.serve))
	t.Cleanup(v.Close)
	return v
}

func (v *expiringVideo) serve(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	v.mu.Lock()
	allowed := false
	switch token {
	case "first":
		allowed = !v.firstUsed
		v.firstUsed = true
	case "renewed":
		allowed = true
		v.freshRequests++
		if start := rangeStart(r.Header.Get("Range")); start > v.furthestFresh {
			v.furthestFresh = start
		}
	}
	v.mu.Unlock()
	if !allowed {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	http.ServeContent(w, r, "video.mkv", time.Time{}, bytes.NewReader(v.data))
}

func rangeStart(header string) int64 {
	from, _, _ := strings.Cut(strings.TrimPrefix(header, "bytes="), "-")
	n, _ := strconv.ParseInt(from, 10, 64)
	return n
}

// playFromHalfway runs mpv with the wrapper's own player args, starting at
// 50% so it has to make a fresh range request after the first one.
func playFromHalfway(t *testing.T, target string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := append([]string{"--no-config", "--vo=null", "--ao=null", "--really-quiet",
		"--start=50%", "--length=2", "--demuxer-max-bytes=256KiB"},
		config.DefaultConfig().Player.Args...)
	home := t.TempDir()
	mpv := exec.CommandContext(ctx, "mpv", append(args, "--", target)...)
	mpv.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"XDG_STATE_HOME="+filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME="+filepath.Join(home, ".cache"))
	out, err := mpv.CombinedOutput()
	require.NoError(t, err, string(out))
}

func requireMpvAndFfmpeg(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("plays a real video")
	}
	for _, tool := range []string{"mpv", "ffmpeg"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not installed")
		}
	}
}

func TestRealMpv_KeepsPlayingWhenTheLinkExpires(t *testing.T) {
	t.Parallel()
	requireMpvAndFfmpeg(t)
	video := newExpiringVideo(t)
	var renewals atomic.Int32
	renew := func(context.Context) (string, error) {
		renewals.Add(1)
		return video.URL + "/dld/video.mkv?token=renewed", nil
	}
	p := startProxy(t, video.URL+"/dld/video.mkv?token=first", renew)

	playFromHalfway(t, p.URL())

	video.mu.Lock()
	defer video.mu.Unlock()
	assert.Equal(t, int32(1), renewals.Load())
	assert.False(t, p.RenewFailed())
	assert.GreaterOrEqual(t, video.freshRequests, 1)
	assert.Greater(t, video.furthestFresh, int64(len(video.data)/3), "mpv read the second half through the renewed link")
}

func TestRealMpv_ReportsALinkThatCouldNotBeRenewed(t *testing.T) {
	t.Parallel()
	requireMpvAndFfmpeg(t)
	video := newExpiringVideo(t)
	renew := func(context.Context) (string, error) { return "", errors.New("torbox is down") }
	p := startProxy(t, video.URL+"/dld/video.mkv?token=first", renew)

	playFromHalfway(t, p.URL())

	assert.True(t, p.RenewFailed())
}
