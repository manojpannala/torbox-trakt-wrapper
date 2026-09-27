package player

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

// seeking comes first: mpv sends one batch of changes in observation order, so
// the monitor sees a seek begin before the positions it reports mid-seek.
var observedProperties = []string{"seeking", "time-pos", "percent-pos", "pause", "duration"}

// scrobbleCallTimeout bounds every Trakt call the monitor makes, including the
// final Stop, so none of them can hold up shutdown indefinitely.
const scrobbleCallTimeout = 10 * time.Second

type Monitor struct {
	client       *IPCClient
	media        matcher.ParsedMedia
	scrobbler    ScrobbleHandler
	socketPath   string
	pollInterval time.Duration
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup
	wake         chan struct{}

	mu         sync.Mutex
	snap       snapshot
	onProgress func(PlaybackProgress)
	logger     *slog.Logger
	cancel     context.CancelFunc
}

func NewMonitor(client *IPCClient, media matcher.ParsedMedia, scrobbler ScrobbleHandler, socketPath string, logger *slog.Logger) *Monitor {
	return &Monitor{
		client:       client,
		media:        media,
		scrobbler:    scrobbler,
		socketPath:   socketPath,
		pollInterval: 1 * time.Second,
		stopCh:       make(chan struct{}),
		wake:         make(chan struct{}, 1),
		logger:       cmp.Or(logger, slog.New(slog.DiscardHandler)),
	}
}

// SetProgressCallback's callback runs on every state change, around 50 times a
// second during playback, so it must be cheap.
func (m *Monitor) SetProgressCallback(cb func(PlaybackProgress)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onProgress = cb
}

func (m *Monitor) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel = cancel
	m.mu.Unlock()

	m.wg.Add(1)
	go m.run(ctx)
}

func (m *Monitor) run(ctx context.Context) {
	defer m.wg.Done()

	var poll <-chan time.Time
	if !m.observe(ctx) {
		ticker := time.NewTicker(m.pollInterval)
		defer ticker.Stop()
		poll = ticker.C
	}

	var state scrobbleState
	var due <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			m.handleStop(ctx, &state)
			return
		case <-m.stopCh:
			m.handleStop(ctx, &state)
			return
		case <-poll:
			m.poll(ctx)
		case <-m.wake:
		case <-due:
			due = nil
		}

		snap, progressCb := m.current()
		if progressCb != nil {
			progressCb(PlaybackProgress{
				TimePos:    snap.timePos,
				PercentPos: snap.percentPos,
				Duration:   snap.duration,
				Paused:     snap.paused,
			})
		}

		action, wait := state.next(snap, time.Now())
		switch {
		case action == scrobbleNone:
		case wait > 0:
			if due == nil {
				due = time.After(wait)
			}
		case ctx.Err() != nil:
			// Stop is already signalled; let the top select take that path
			// instead of sending a call that would only need cancelling.
		default:
			m.send(ctx, action, snap.percentPos)
			state.sent(action, snap, time.Now())
		}
	}
}

// observe registers its hooks before asking mpv to observe anything, because
// mpv answers each observe_property with the current value straight away.
func (m *Monitor) observe(ctx context.Context) bool {
	m.client.OnPropertyChange(m.onPropertyChange)
	m.client.OnEvent(func(event string, _ json.RawMessage) {
		if event != "playback-restart" {
			return
		}
		m.mu.Lock()
		m.snap.restarts++
		m.mu.Unlock()
		m.poke()
	})

	observeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for _, name := range observedProperties {
		if err := m.client.ObserveProperty(observeCtx, name); err != nil {
			m.logger.Debug("mpv property observation unavailable, polling instead", "property", name, "err", err)
			return false
		}
	}
	return true
}

func (m *Monitor) onPropertyChange(name string, data json.RawMessage) {
	// Unavailable, as on unload: keep the last value so Stop reports it.
	if len(data) == 0 {
		return
	}
	m.mu.Lock()
	switch name {
	case "seeking":
		_ = json.Unmarshal(data, &m.snap.seeking)
	case "time-pos":
		_ = json.Unmarshal(data, &m.snap.timePos)
	case "percent-pos":
		_ = json.Unmarshal(data, &m.snap.percentPos)
	case "duration":
		_ = json.Unmarshal(data, &m.snap.duration)
	case "pause":
		_ = json.Unmarshal(data, &m.snap.paused)
	}
	m.mu.Unlock()
	m.poke()
}

func (m *Monitor) poke() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Monitor) current() (snapshot, func(PlaybackProgress)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snap, m.onProgress
}

func (m *Monitor) poll(ctx context.Context) {
	timePos, err := m.client.GetFloatProperty(ctx, "time-pos")
	if err != nil {
		return
	}

	percentPos, err := m.client.GetFloatProperty(ctx, "percent-pos")
	if err != nil {
		return
	}

	paused, _ := m.client.GetBoolProperty(ctx, "pause")

	m.mu.Lock()
	needDuration := m.snap.duration <= 0
	m.mu.Unlock()

	var dur float64
	if needDuration {
		dur, _ = m.client.GetFloatProperty(ctx, "duration")
	}

	m.mu.Lock()
	m.snap.timePos = timePos
	m.snap.percentPos = percentPos
	m.snap.paused = paused
	if dur > 0 {
		m.snap.duration = dur
	}
	m.mu.Unlock()
}

func (m *Monitor) send(ctx context.Context, action scrobbleAction, percent float64) {
	if m.scrobbler == nil {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, scrobbleCallTimeout)
	defer cancel()
	switch action {
	case scrobbleStart:
		err := m.scrobbler.Start(callCtx, m.media, percent)
		m.logger.Debug("scrobble start", "title", m.media.CleanTitle, "percent", percent, "err", err)
	case scrobblePause:
		err := m.scrobbler.Pause(callCtx, m.media, percent)
		m.logger.Debug("scrobble pause", "title", m.media.CleanTitle, "percent", percent, "err", err)
	case scrobbleNone:
	}
}

func (m *Monitor) handleStop(ctx context.Context, state *scrobbleState) {
	started, wait := state.stopWait(time.Now())
	if started && m.scrobbler != nil {
		time.Sleep(wait)
		snap, _ := m.current()
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), scrobbleCallTimeout)
		_, err := m.scrobbler.Stop(stopCtx, m.media, snap.percentPos)
		cancel()
		m.logger.Debug("scrobble stop", "title", m.media.CleanTitle, "percent", snap.percentPos, "err", err)
	} else {
		m.logger.Debug("playback ended without a scrobble", "title", m.media.CleanTitle, "started", started)
	}

	_ = m.client.Close()
	if m.socketPath != "" {
		_ = os.Remove(m.socketPath)
	}
}

func (m *Monitor) Stop() {
	m.stopOnce.Do(func() {
		// Cancel first so run never starts a call once Stop has begun.
		m.mu.Lock()
		cancel := m.cancel
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		close(m.stopCh)
	})
	m.wg.Wait()
}

func (m *Monitor) GetLastProgress() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snap.percentPos
}
