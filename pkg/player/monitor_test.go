package player_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
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
	var currentPos = 10.0
	var percentPos = 5.0
	var isPaused = false

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

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelDial()

	client, err := player.DialIPC(dialCtx, sockPath, 2*time.Second)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	scrobbler := &mockScrobbler{}
	media := matcher.ParsedMedia{
		CleanTitle: "Test Movie Alpha",
		Year:       2023,
		Type:       matcher.MediaTypeMovie,
	}

	monitor := player.NewMonitor(client, media, scrobbler, sockPath, nil)

	var progressEvents []player.PlaybackProgress
	var mu sync.Mutex
	monitor.SetProgressCallback(func(p player.PlaybackProgress) {
		mu.Lock()
		progressEvents = append(progressEvents, p)
		mu.Unlock()
	})

	monitor.Start(ctx)

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

type scrobbleCall struct {
	kind     string
	progress float64
	at       time.Time
}

type recordingScrobbler struct {
	mu    sync.Mutex
	calls []scrobbleCall
}

func (r *recordingScrobbler) record(kind string, progress float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, scrobbleCall{kind, progress, time.Now()})
}

func (r *recordingScrobbler) Start(_ context.Context, _ matcher.ParsedMedia, progress float64) error {
	r.record("start", progress)
	return nil
}

func (r *recordingScrobbler) Pause(_ context.Context, _ matcher.ParsedMedia, progress float64) error {
	r.record("pause", progress)
	return nil
}

func (r *recordingScrobbler) Stop(_ context.Context, _ matcher.ParsedMedia, progress float64) (*trakt.ScrobbleResponse, error) {
	r.record("stop", progress)
	return &trakt.ScrobbleResponse{Action: "scrobble", Progress: progress}, nil
}

func (r *recordingScrobbler) snapshot() []scrobbleCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func (r *recordingScrobbler) kinds() []string {
	var out []string
	for _, c := range r.snapshot() {
		out = append(out, c.kind)
	}
	return out
}

func (r *recordingScrobbler) waitForCalls(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.snapshot()) >= n },
		5*time.Second, 10*time.Millisecond, "want %d scrobble calls", n)
}

func playingProps() map[string]any {
	return map[string]any{"time-pos": 10.0, "percent-pos": 5.0, "pause": false, "duration": 200.0}
}

func startEventMonitor(t *testing.T, fake *fakeMPV, rec *recordingScrobbler) *player.Monitor {
	t.Helper()
	media := matcher.ParsedMedia{CleanTitle: "Test Movie Alpha", Year: 2023, Type: matcher.MediaTypeMovie}
	mon := player.NewMonitor(dialFake(t, fake), media, rec, "", nil)
	mon.Start(context.Background())
	t.Cleanup(mon.Stop)
	return mon
}

func TestMonitor_PauseAndResumeInsideOneSecondBothReachTrakt(t *testing.T) {
	fake := startFakeMPV(t, playingProps())
	rec := &recordingScrobbler{}
	mon := startEventMonitor(t, fake, rec)

	rec.waitForCalls(t, 1)
	time.Sleep(1100 * time.Millisecond)
	fake.Set("pause", true)
	time.Sleep(300 * time.Millisecond)
	fake.Set("pause", false)
	rec.waitForCalls(t, 3)
	mon.Stop()

	calls := rec.snapshot()
	assert.Equal(t, []string{"start", "pause", "start", "stop"}, rec.kinds())
	assert.GreaterOrEqual(t, calls[2].at.Sub(calls[1].at), 950*time.Millisecond, "calls are spaced a second apart")
}

func TestMonitor_NoIPCRequestsWhilePaused(t *testing.T) {
	fake := startFakeMPV(t, playingProps())
	rec := &recordingScrobbler{}
	startEventMonitor(t, fake, rec)

	rec.waitForCalls(t, 1)
	fake.Set("pause", true)
	rec.waitForCalls(t, 2)
	time.Sleep(1500 * time.Millisecond)

	assert.Equal(t, 4, fake.Requests(), "only the four observe_property commands")
	assert.Zero(t, fake.Gets())
}

func TestMonitor_OpenedPausedSendsNothing(t *testing.T) {
	props := playingProps()
	props["pause"] = true
	fake := startFakeMPV(t, props)
	rec := &recordingScrobbler{}
	mon := startEventMonitor(t, fake, rec)

	// Longer than one poll interval, so a polling monitor would have started.
	time.Sleep(1500 * time.Millisecond)
	mon.Stop()

	assert.Empty(t, rec.snapshot(), "no start, and so no stop")
}

func TestMonitor_StartsWhenFirstUnpaused(t *testing.T) {
	props := playingProps()
	props["pause"] = true
	fake := startFakeMPV(t, props)
	rec := &recordingScrobbler{}
	startEventMonitor(t, fake, rec)

	time.Sleep(1500 * time.Millisecond)
	assert.Empty(t, rec.snapshot())

	fake.Set("pause", false)
	rec.waitForCalls(t, 1)
	calls := rec.snapshot()
	assert.Equal(t, "start", calls[0].kind)
	assert.InDelta(t, 5.0, calls[0].progress, 0.001)
}

func TestMonitor_SeekWhilePlayingResendsStart(t *testing.T) {
	fake := startFakeMPV(t, playingProps())
	rec := &recordingScrobbler{}
	startEventMonitor(t, fake, rec)

	rec.waitForCalls(t, 1)
	time.Sleep(1100 * time.Millisecond)
	fake.Set("percent-pos", 40.0)
	fake.Emit("playback-restart")
	rec.waitForCalls(t, 2)

	calls := rec.snapshot()
	assert.Equal(t, "start", calls[1].kind)
	assert.InDelta(t, 40.0, calls[1].progress, 0.001)
}

func TestMonitor_StopKeepsLastPositionWhenPropertiesGoUnavailable(t *testing.T) {
	fake := startFakeMPV(t, playingProps())
	rec := &recordingScrobbler{}
	mon := startEventMonitor(t, fake, rec)

	rec.waitForCalls(t, 1)
	fake.Set("percent-pos", 92.5)
	fake.Unset("percent-pos")
	fake.Unset("time-pos")
	time.Sleep(100 * time.Millisecond)
	mon.Stop()

	calls := rec.snapshot()
	last := calls[len(calls)-1]
	assert.Equal(t, "stop", last.kind)
	assert.InDelta(t, 92.5, last.progress, 0.001)
}

func TestMonitor_FallsBackToPollingWhenObserveIsRejected(t *testing.T) {
	fake := startFakeMPV(t, playingProps())
	fake.RejectObserve()
	rec := &recordingScrobbler{}
	mon := startEventMonitor(t, fake, rec)

	rec.waitForCalls(t, 1)
	mon.Stop()

	assert.Equal(t, []string{"start", "stop"}, rec.kinds())
	assert.Positive(t, fake.Gets(), "fallback reads properties")
}
