package player_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockScrobbler struct {
	mu           sync.Mutex
	startCalls   int
	pauseCalls   int
	stopCalls    int
	lastProgress float64
}

func (m *mockScrobbler) Start(ctx context.Context, media matcher.ParsedMedia, progress float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalls++
	m.lastProgress = progress
	return nil
}

func (m *mockScrobbler) Pause(ctx context.Context, media matcher.ParsedMedia, progress float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pauseCalls++
	m.lastProgress = progress
	return nil
}

func (m *mockScrobbler) Stop(ctx context.Context, media matcher.ParsedMedia, progress float64) (*trakt.ScrobbleResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls++
	m.lastProgress = progress
	return &trakt.ScrobbleResponse{Action: "scrobble", Progress: progress}, nil
}

func TestMonitor_ScrobbleLifecycle(t *testing.T) {
	var stateMu sync.RWMutex
	var currentPos float64 = 10.0
	var percentPos float64 = 5.0
	var isPaused bool = false

	sockPath, cleanup := startMockMPVSocket(t, func(cmd []interface{}) (interface{}, string) {
		if len(cmd) >= 2 && cmd[0] == "get_property" {
			stateMu.RLock()
			defer stateMu.RUnlock()
			switch cmd[1] {
			case "time-pos":
				return currentPos, "success"
			case "percent-pos":
				return percentPos, "success"
			case "duration":
				return 200.0, "success"
			case "pause":
				return isPaused, "success"
			}
		}
		return nil, "error"
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := player.DialIPC(ctx, sockPath, 2*time.Second)
	require.NoError(t, err)

	scrobbler := &mockScrobbler{}
	media := matcher.ParsedMedia{
		CleanTitle: "Test Movie Alpha",
		Year:       2023,
		Type:       matcher.MediaTypeMovie,
	}

	monitor := player.NewMonitor(client, media, scrobbler, sockPath)

	var progressEvents []player.PlaybackProgress
	var mu sync.Mutex
	monitor.SetProgressCallback(func(p player.PlaybackProgress) {
		mu.Lock()
		progressEvents = append(progressEvents, p)
		mu.Unlock()
	})

	monitor.Start()

	time.Sleep(1200 * time.Millisecond)

	scrobbler.mu.Lock()
	assert.GreaterOrEqual(t, scrobbler.startCalls, 1)
	scrobbler.mu.Unlock()

	stateMu.Lock()
	isPaused = true
	stateMu.Unlock()
	time.Sleep(1200 * time.Millisecond)

	scrobbler.mu.Lock()
	assert.GreaterOrEqual(t, scrobbler.pauseCalls, 1)
	scrobbler.mu.Unlock()

	stateMu.Lock()
	isPaused = false
	percentPos = 92.5
	stateMu.Unlock()
	time.Sleep(1200 * time.Millisecond)

	monitor.Stop()

	scrobbler.mu.Lock()
	assert.Equal(t, 1, scrobbler.stopCalls)
	assert.Equal(t, 92.5, scrobbler.lastProgress)
	scrobbler.mu.Unlock()
}
