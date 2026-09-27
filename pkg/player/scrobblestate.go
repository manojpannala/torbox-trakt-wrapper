package player

import "time"

type scrobbleAction int

const (
	scrobbleNone scrobbleAction = iota
	scrobbleStart
	scrobblePause
)

// Trakt allows about one authenticated write a second and answers bursts with
// 429; the Stop that marks an item watched must never be the call that draws it.
const scrobbleSpacing = time.Second

type snapshot struct {
	timePos    float64
	percentPos float64
	duration   float64
	paused     bool
	restarts   uint64
}

type toldState int

const (
	toldNothing toldState = iota
	toldPlaying
	toldPaused
)

// scrobbleState compares the player's state with what Trakt was last told, so
// a burst of changes collapses to whatever holds when the next call may go.
type scrobbleState struct {
	told            toldState
	lastReturn      time.Time
	restartsAtStart uint64
}

func (s *scrobbleState) next(snap snapshot, now time.Time) (scrobbleAction, time.Duration) {
	action := s.due(snap)
	if action == scrobbleNone {
		return scrobbleNone, 0
	}
	return action, s.wait(now)
}

func (s *scrobbleState) due(snap snapshot) scrobbleAction {
	switch s.told {
	case toldNothing:
		if !snap.paused && snap.timePos > 0 {
			return scrobbleStart
		}
	case toldPlaying:
		if snap.paused {
			return scrobblePause
		}
		if snap.restarts > s.restartsAtStart {
			return scrobbleStart
		}
	case toldPaused:
		if !snap.paused {
			return scrobbleStart
		}
	}
	return scrobbleNone
}

func (s *scrobbleState) wait(now time.Time) time.Duration {
	if s.lastReturn.IsZero() {
		return 0
	}
	return max(0, s.lastReturn.Add(scrobbleSpacing).Sub(now))
}

func (s *scrobbleState) sent(action scrobbleAction, snap snapshot, returned time.Time) {
	s.lastReturn = returned
	switch action {
	case scrobbleStart:
		s.told = toldPlaying
		// Restarts up to now, including file load and the resume jump, are
		// what this Start already reports.
		s.restartsAtStart = snap.restarts
	case scrobblePause:
		s.told = toldPaused
	case scrobbleNone:
	}
}

func (s *scrobbleState) stopWait(now time.Time) (bool, time.Duration) {
	if s.told == toldNothing {
		return false, 0
	}
	return true, s.wait(now)
}
