package search

import "time"

// The badge checker's pacing: TorBox sees at most one request every
// MinSpacing, each carrying up to BatchSize hashes.
const (
	BatchSize    = 100
	BatchWait    = 2 * time.Second
	MinSpacing   = 2 * time.Second
	CachedTTL    = 15 * time.Minute
	BackoffFirst = 30 * time.Second
	BackoffMax   = 5 * time.Minute
)

type checked struct {
	cached bool
	at     time.Time
}

// Checker schedules cached-status checks so that every search shares one
// budget. It does no I/O and is not safe for concurrent use: the caller
// sends each batch Next hands out and reports back with Done, Failed or
// Limited.
type Checker struct {
	known    map[string]checked
	queue    []string
	queued   map[string]bool
	oldest   time.Time
	lastSent time.Time
	busy     bool
	backoff  time.Duration
	paused   time.Time
}

func NewChecker() *Checker {
	return &Checker{known: map[string]checked{}, queued: map[string]bool{}}
}

// Lookup returns a hash's cached status if it was checked within CachedTTL.
func (c *Checker) Lookup(hash string, now time.Time) (cached, ok bool) {
	k, ok := c.known[hash]
	if !ok || now.Sub(k.at) > CachedTTL {
		return false, false
	}
	return k.cached, true
}

// Enqueue queues the hashes that have no fresh answer and aren't queued.
func (c *Checker) Enqueue(hashes []string, now time.Time) {
	for _, h := range hashes {
		if h == "" || c.queued[h] {
			continue
		}
		if _, ok := c.Lookup(h, now); ok {
			continue
		}
		if len(c.queue) == 0 {
			c.oldest = now
		}
		c.queue = append(c.queue, h)
		c.queued[h] = true
	}
}

// Next returns the batch to send now, or how long to wait before asking
// again. It returns neither while a batch is out or nothing is queued.
func (c *Checker) Next(now time.Time) ([]string, time.Duration) {
	if c.busy || len(c.queue) == 0 {
		return nil, 0
	}
	ready := c.lastSent.Add(MinSpacing)
	if len(c.queue) < BatchSize {
		ready = later(ready, c.oldest.Add(BatchWait))
	}
	ready = later(ready, c.paused)
	if now.Before(ready) {
		return nil, ready.Sub(now)
	}

	n := min(len(c.queue), BatchSize)
	batch := c.queue[:n:n]
	c.queue = c.queue[n:]
	for _, h := range batch {
		delete(c.queued, h)
	}
	c.busy = true
	c.lastSent = now
	return batch, 0
}

// Done records a batch's answer.
func (c *Checker) Done(batch []string, cached map[string]bool, now time.Time) {
	c.busy = false
	c.backoff = 0
	for _, h := range batch {
		c.known[h] = checked{cached: cached[h], at: now}
	}
}

// Failed ends a batch that got no answer. Its hashes stay unknown and are
// not retried until something queues them again.
func (c *Checker) Failed() {
	c.busy = false
}

// Limited puts a rate-limited batch back at the front of the queue and
// pauses for retryAfter, or for a backoff that doubles up to BackoffMax.
func (c *Checker) Limited(batch []string, retryAfter time.Duration, now time.Time) {
	c.busy = false
	if c.backoff == 0 {
		c.backoff = BackoffFirst
	} else {
		c.backoff = min(c.backoff*2, BackoffMax)
	}
	c.paused = now.Add(max(c.backoff, retryAfter))

	front := make([]string, 0, len(batch)+len(c.queue))
	for _, h := range batch {
		if !c.queued[h] {
			front = append(front, h)
			c.queued[h] = true
		}
	}
	c.queue = append(front, c.queue...)
	c.oldest = now
}

// RetryingIn is how long a rate-limit pause has left, or zero.
func (c *Checker) RetryingIn(now time.Time) time.Duration {
	if now.Before(c.paused) {
		return c.paused.Sub(now)
	}
	return 0
}

// Clear drops every hash not yet sent, for a search that was cancelled or
// replaced. Answers already recorded are kept.
func (c *Checker) Clear() {
	c.queue = nil
	c.queued = map[string]bool{}
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
