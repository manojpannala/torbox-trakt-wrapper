package search_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func hashes(n int, prefix string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%03d", prefix, i)
	}
	return out
}

func TestChecker_WaitsForMoreHashesBeforeSending(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue([]string{"h1", "h2"}, t0)

	batch, wait := c.Next(t0.Add(500 * time.Millisecond))
	assert.Nil(t, batch)
	assert.Equal(t, 1500*time.Millisecond, wait)

	c.Enqueue([]string{"h3"}, t0.Add(time.Second))
	batch, _ = c.Next(t0.Add(search.BatchWait))
	assert.Equal(t, []string{"h1", "h2", "h3"}, batch)
}

func TestChecker_AFullBatchGoesAtOnce(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue(hashes(search.BatchSize+5, "h"), t0)

	batch, wait := c.Next(t0)

	assert.Len(t, batch, search.BatchSize)
	assert.Zero(t, wait)
}

func TestChecker_OneRequestAtATimeAndSpaced(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue(hashes(search.BatchSize*2, "h"), t0)
	first, _ := c.Next(t0)
	require.Len(t, first, search.BatchSize)

	batch, wait := c.Next(t0)
	assert.Nil(t, batch)
	assert.Zero(t, wait, "a batch is out, so nothing is scheduled until it is done")

	c.Done(first, map[string]bool{}, t0.Add(300*time.Millisecond))
	batch, wait = c.Next(t0.Add(300 * time.Millisecond))
	assert.Nil(t, batch)
	assert.Equal(t, search.MinSpacing-300*time.Millisecond, wait)

	batch, _ = c.Next(t0.Add(search.MinSpacing))
	assert.Len(t, batch, search.BatchSize)
}

func TestChecker_NeverSendsMoreThanThirtyAMinute(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue(hashes(search.BatchSize*100, "h"), t0)
	sent := 0
	for now := t0; now.Before(t0.Add(time.Minute)); now = now.Add(100 * time.Millisecond) {
		if batch, _ := c.Next(now); batch != nil {
			sent++
			c.Done(batch, nil, now)
		}
	}
	assert.LessOrEqual(t, sent, 30)
}

func TestChecker_RemembersAnswersForTheCacheLifetime(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue([]string{"h1", "h2"}, t0)
	batch, _ := c.Next(t0.Add(search.BatchWait))
	c.Done(batch, map[string]bool{"h1": true}, t0.Add(search.BatchWait))

	cached, ok := c.Lookup("h1", t0.Add(time.Minute))
	assert.True(t, ok)
	assert.True(t, cached)
	cached, ok = c.Lookup("h2", t0.Add(time.Minute))
	assert.True(t, ok)
	assert.False(t, cached)

	c.Enqueue([]string{"h1"}, t0.Add(time.Minute))
	batch, wait := c.Next(t0.Add(time.Hour))
	assert.Nil(t, batch, "a fresh answer is not asked for again")
	assert.Zero(t, wait)

	_, ok = c.Lookup("h1", t0.Add(search.BatchWait+search.CachedTTL+time.Second))
	assert.False(t, ok, "a stale answer is forgotten")
}

func TestChecker_DoesNotQueueAHashTwice(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue([]string{"h1", "h1", ""}, t0)
	c.Enqueue([]string{"h1"}, t0)

	batch, _ := c.Next(t0.Add(search.BatchWait))

	assert.Equal(t, []string{"h1"}, batch)
}

func TestChecker_RateLimitBacksOffAndRetriesTheBatchFirst(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue([]string{"h1"}, t0)
	now := t0.Add(search.BatchWait)
	batch, _ := c.Next(now)
	c.Enqueue([]string{"h2"}, now)

	c.Limited(batch, 0, now)
	assert.Equal(t, search.BackoffFirst, c.RetryingIn(now))
	b, wait := c.Next(now.Add(time.Second))
	assert.Nil(t, b)
	assert.Equal(t, search.BackoffFirst-time.Second, wait)

	now = now.Add(search.BackoffFirst)
	batch, _ = c.Next(now)
	assert.Equal(t, []string{"h1", "h2"}, batch)

	c.Limited(batch, 0, now)
	assert.Equal(t, 2*search.BackoffFirst, c.RetryingIn(now), "a second 429 doubles the wait")

	now = now.Add(2 * search.BackoffFirst)
	batch, _ = c.Next(now)
	c.Done(batch, nil, now)
	c.Enqueue([]string{"h3"}, now)
	now = now.Add(search.BatchWait)
	batch, _ = c.Next(now)
	c.Limited(batch, 0, now)
	assert.Equal(t, search.BackoffFirst, c.RetryingIn(now), "a success resets the backoff")
}

func TestChecker_RateLimitHonoursALongerRetryAfter(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue([]string{"h1"}, t0)
	batch, _ := c.Next(t0.Add(search.BatchWait))

	c.Limited(batch, 90*time.Second, t0.Add(search.BatchWait))

	assert.Equal(t, 90*time.Second, c.RetryingIn(t0.Add(search.BatchWait)))
}

func TestChecker_BackoffStopsAtTheMaximum(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue([]string{"h1"}, t0)
	now := t0
	for range 10 {
		now = now.Add(search.BackoffMax + search.BatchWait)
		batch, _ := c.Next(now)
		require.NotNil(t, batch)
		c.Limited(batch, 0, now)
	}
	assert.Equal(t, search.BackoffMax, c.RetryingIn(now))
}

func TestChecker_AFailedBatchIsNotRetried(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue([]string{"h1"}, t0)
	batch, _ := c.Next(t0.Add(search.BatchWait))

	c.Failed()

	_, ok := c.Lookup(batch[0], t0.Add(search.BatchWait))
	assert.False(t, ok)
	b, wait := c.Next(t0.Add(time.Hour))
	assert.Nil(t, b)
	assert.Zero(t, wait)
}

func TestChecker_ClearDropsWhatIsNotSent(t *testing.T) {
	c := search.NewChecker()
	c.Enqueue([]string{"h1", "h2"}, t0)

	c.Clear()

	b, wait := c.Next(t0.Add(time.Hour))
	assert.Nil(t, b)
	assert.Zero(t, wait)
	c.Enqueue([]string{"h1"}, t0.Add(time.Hour))
	b, _ = c.Next(t0.Add(time.Hour + search.BatchWait))
	assert.Equal(t, []string{"h1"}, b, "a cleared hash can be queued again")
}
