package player

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

type Monitor struct {
	client       *IPCClient
	media        matcher.ParsedMedia
	scrobbler    ScrobbleHandler
	socketPath   string
	pollInterval time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup

	mu            sync.Mutex
	started       bool
	paused        bool
	lastProgress  float64
	lastTimePos   float64
	duration      float64
	onProgress    func(PlaybackProgress)
}

func NewMonitor(client *IPCClient, media matcher.ParsedMedia, scrobbler ScrobbleHandler, socketPath string) *Monitor {
	return &Monitor{
		client:       client,
		media:        media,
		scrobbler:    scrobbler,
		socketPath:   socketPath,
		pollInterval: 1 * time.Second,
		stopCh:       make(chan struct{}),
	}
}

func (m *Monitor) SetProgressCallback(cb func(PlaybackProgress)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onProgress = cb
}

func (m *Monitor) Start() {
	m.wg.Add(1)
	go m.run()
}

func (m *Monitor) run() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		select {
		case <-m.stopCh:
			m.handleStop(ctx)
			return
		case <-ticker.C:
			m.poll(ctx)
		}
	}
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

	dur, _ := m.client.GetFloatProperty(ctx, "duration")
	paused, _ := m.client.GetBoolProperty(ctx, "pause")

	m.mu.Lock()
	m.lastTimePos = timePos
	m.lastProgress = percentPos
	if dur > 0 {
		m.duration = dur
	}
	wasPaused := m.paused
	m.paused = paused
	progressCb := m.onProgress

	if !m.started && timePos > 0 {
		m.started = true
		if m.scrobbler != nil {
			_ = m.scrobbler.Start(ctx, m.media, percentPos)
		}
	} else if m.started {
		if paused && !wasPaused {
			if m.scrobbler != nil {
				_ = m.scrobbler.Pause(ctx, m.media, percentPos)
			}
		} else if !paused && wasPaused {
			if m.scrobbler != nil {
				_ = m.scrobbler.Start(ctx, m.media, percentPos)
			}
		}
	}
	m.mu.Unlock()

	if progressCb != nil {
		progressCb(PlaybackProgress{
			TimePos:    timePos,
			PercentPos: percentPos,
			Duration:   dur,
			Paused:     paused,
		})
	}
}

func (m *Monitor) handleStop(ctx context.Context) {
	m.mu.Lock()
	started := m.started
	finalProg := m.lastProgress
	m.mu.Unlock()

	if started && m.scrobbler != nil {
		_, _ = m.scrobbler.Stop(ctx, m.media, finalProg)
	}

	_ = m.client.Close()
	if m.socketPath != "" {
		_ = os.Remove(m.socketPath)
	}
}

func (m *Monitor) Stop() {
	select {
	case <-m.stopCh:
		return
	default:
		close(m.stopCh)
		m.wg.Wait()
	}
}

func (m *Monitor) GetLastProgress() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastProgress
}
