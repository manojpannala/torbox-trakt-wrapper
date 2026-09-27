package player

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var t0 = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

type ruleStep struct {
	at     time.Duration
	snap   snapshot
	action scrobbleAction
	wait   time.Duration
}

// runRules sends every action the moment it is allowed, and every call returns
// instantly, so a step's time is also its return time.
func runRules(t *testing.T, steps []ruleStep) *scrobbleState {
	t.Helper()
	var s scrobbleState
	for i, st := range steps {
		now := t0.Add(st.at)
		action, wait := s.next(st.snap, now)
		assert.Equal(t, st.action, action, "step %d action", i)
		assert.Equal(t, st.wait, wait, "step %d wait", i)
		if action != scrobbleNone && wait == 0 {
			s.sent(action, st.snap, now)
		}
	}
	return &s
}

var (
	playingAt = func(pct float64) snapshot { return snapshot{timePos: 10, percentPos: pct} }
	pausedAt  = func(pct float64) snapshot { return snapshot{timePos: 10, percentPos: pct, paused: true} }
)

func TestScrobbleState_Rules(t *testing.T) {
	ms := time.Millisecond
	tests := []struct {
		name  string
		steps []ruleStep
	}{
		{"opened paused never starts", []ruleStep{
			{0, pausedAt(5), scrobbleNone, 0},
			{3 * time.Second, pausedAt(5), scrobbleNone, 0},
		}},
		{"no start before time-pos moves", []ruleStep{
			{0, snapshot{}, scrobbleNone, 0},
			{time.Second, playingAt(5), scrobbleStart, 0},
		}},
		{"no start before percent-pos arrives", []ruleStep{
			{0, snapshot{timePos: 10}, scrobbleNone, 0},
			{time.Second, snapshot{timePos: 10, percentPos: 5}, scrobbleStart, 0},
		}},
		{"opened paused then unpaused starts once", []ruleStep{
			{0, pausedAt(5), scrobbleNone, 0},
			{2 * time.Second, playingAt(5), scrobbleStart, 0},
			{4 * time.Second, playingAt(6), scrobbleNone, 0},
		}},
		{"pause then resume inside a second sends both, spaced", []ruleStep{
			{0, playingAt(5), scrobbleStart, 0},
			{5 * time.Second, pausedAt(10), scrobblePause, 0},
			{5400 * ms, playingAt(10), scrobbleStart, 600 * ms},
			{6 * time.Second, playingAt(10), scrobbleStart, 0},
		}},
		{"a toggle burst coalesces to its final state", []ruleStep{
			{0, playingAt(5), scrobbleStart, 0},
			{200 * ms, pausedAt(5), scrobblePause, 800 * ms},
			{500 * ms, playingAt(5), scrobbleNone, 0},
			{800 * ms, pausedAt(5), scrobblePause, 200 * ms},
			{time.Second, pausedAt(5), scrobblePause, 0},
			{3 * time.Second, pausedAt(5), scrobbleNone, 0},
		}},
		{"seek while playing re-sends start", []ruleStep{
			{0, playingAt(5), scrobbleStart, 0},
			{5 * time.Second, snapshot{timePos: 80, percentPos: 40, restarts: 1}, scrobbleStart, 0},
			{8 * time.Second, snapshot{timePos: 83, percentPos: 41, restarts: 1}, scrobbleNone, 0},
		}},
		{"seek while paused sends nothing until resume", []ruleStep{
			{0, playingAt(5), scrobbleStart, 0},
			{2 * time.Second, pausedAt(5), scrobblePause, 0},
			{4 * time.Second, snapshot{timePos: 80, percentPos: 40, paused: true, restarts: 1}, scrobbleNone, 0},
			{6 * time.Second, snapshot{timePos: 80, percentPos: 40, restarts: 1}, scrobbleStart, 0},
			{9 * time.Second, snapshot{timePos: 83, percentPos: 41, restarts: 1}, scrobbleNone, 0},
		}},
		{"restarts before the first start are ignored", []ruleStep{
			{0, snapshot{restarts: 1}, scrobbleNone, 0},
			{time.Second, snapshot{timePos: 3, percentPos: 1, restarts: 1}, scrobbleStart, 0},
			{5 * time.Second, snapshot{timePos: 7, percentPos: 3, restarts: 1}, scrobbleNone, 0},
		}},
		{"no start while mpv seeks to the resume point", []ruleStep{
			{0, snapshot{timePos: 30, percentPos: 100, seeking: true}, scrobbleNone, 0},
			{100 * ms, snapshot{timePos: 30, percentPos: 3.3, seeking: true, restarts: 1}, scrobbleNone, 0},
			{200 * ms, snapshot{timePos: 30, percentPos: 3.3, restarts: 1}, scrobbleStart, 0},
			{3 * time.Second, snapshot{timePos: 33, percentPos: 3.4, restarts: 1}, scrobbleNone, 0},
		}},
		{"a seek re-start waits for the seek to finish", []ruleStep{
			{0, playingAt(5), scrobbleStart, 0},
			{5 * time.Second, snapshot{timePos: 80, percentPos: 40, seeking: true, restarts: 1}, scrobbleNone, 0},
			{5100 * ms, snapshot{timePos: 80, percentPos: 40, restarts: 1}, scrobbleStart, 0},
		}},
		{"resume during a seek waits for the seek to finish", []ruleStep{
			{0, playingAt(5), scrobbleStart, 0},
			{2 * time.Second, pausedAt(5), scrobblePause, 0},
			{4 * time.Second, snapshot{timePos: 80, percentPos: 40, seeking: true, restarts: 1}, scrobbleNone, 0},
			{4100 * ms, snapshot{timePos: 80, percentPos: 40, restarts: 1}, scrobbleStart, 0},
		}},
		{"pause is not held by a seek", []ruleStep{
			{0, playingAt(5), scrobbleStart, 0},
			{2 * time.Second, snapshot{timePos: 10, percentPos: 5, paused: true, seeking: true}, scrobblePause, 0},
		}},
		{"a pause and resume inside the spacing window collapse to nothing", []ruleStep{
			{0, playingAt(5), scrobbleStart, 0},
			{400 * ms, pausedAt(5), scrobblePause, 600 * ms},
			{time.Second, playingAt(5), scrobbleNone, 0},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runRules(t, tt.steps)
		})
	}
}

func TestScrobbleState_FailedCallIsNotRetried(t *testing.T) {
	var s scrobbleState
	action, _ := s.next(playingAt(5), t0)
	assert.Equal(t, scrobbleStart, action)
	s.sent(action, playingAt(5), t0)

	action, wait := s.next(playingAt(6), t0.Add(3*time.Second))
	assert.Equal(t, scrobbleNone, action)
	assert.Zero(t, wait)
}

func TestScrobbleState_StopWait(t *testing.T) {
	var never scrobbleState
	started, wait := never.stopWait(t0)
	assert.False(t, started, "never started means no stop")
	assert.Zero(t, wait)

	s := runRules(t, []ruleStep{{0, playingAt(5), scrobbleStart, 0}})
	started, wait = s.stopWait(t0.Add(400 * time.Millisecond))
	assert.True(t, started)
	assert.Equal(t, 600*time.Millisecond, wait, "stop honours the spacing")

	started, wait = s.stopWait(t0.Add(2 * time.Second))
	assert.True(t, started)
	assert.Zero(t, wait)
}
