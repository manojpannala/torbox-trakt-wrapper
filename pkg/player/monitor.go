package player

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

type scrobbleAction int

const (
	scrobbleNone scrobbleAction = iota
	scrobbleStart
	scrobblePause
)

type Monitor struct {
	client       *IPCClient
	media        matcher.ParsedMedia
	scrobbler    ScrobbleHandler
	socketPath   string
	pollInterval time.Duration
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup

	mu           sync.Mutex
	started      bool
	paused       bool
	lastProgress float64
	lastTimePos  float64
	duration     float64
	onProgress   func(PlaybackProgress)
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

func (m *Monitor) Start(ctx context.Context) {
	m.wg.Add(1)
	go m.run(ctx)
}

func (m *Monitor) run(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.handleStop(ctx)
			return
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

	paused, _ := m.client.GetBoolProperty(ctx, "pause")

	m.mu.Lock()
	dur := m.duration
	m.mu.Unlock()

	if dur <= 0 {
		if d, err := m.client.GetFloatProperty(ctx, "duration"); err == nil {
			dur = d
		}
	}

	m.mu.Lock()
	m.lastTimePos = timePos
	m.lastProgress = percentPos
	if dur > 0 {
		m.duration = dur
	}
	wasPaused := m.paused
	m.paused = paused
	progressCb := m.onProgress

	action := scrobbleNone
	if !m.started && timePos > 0 {
		m.started = true
		action = scrobbleStart
	} else if m.started {
		if paused && !wasPaused {
			action = scrobblePause
		} else if !paused && wasPaused {
			action = scrobbleStart
		}
	}
	m.mu.Unlock()

	if m.scrobbler != nil {
		switch action {
		case scrobbleStart:
			_ = m.scrobbler.Start(ctx, m.media, percentPos)
		case scrobblePause:
			_ = m.scrobbler.Pause(ctx, m.media, percentPos)
		case scrobbleNone:
		}
	}

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
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_, _ = m.scrobbler.Stop(stopCtx, m.media, finalProg)
		cancel()
	}

	_ = m.client.Close()
	if m.socketPath != "" {
		_ = os.Remove(m.socketPath)
	}
}

func (m *Monitor) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
	m.wg.Wait()
}

func (m *Monitor) GetLastProgress() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastProgress
}
