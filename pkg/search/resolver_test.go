package search_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/search"
)

func pending(n, indexerID int) []search.Result {
	resolve := func(context.Context) (string, error) { return "", nil }
	rows := make([]search.Result, n)
	for i := range rows {
		rows[i] = search.Result{GUID: fmt.Sprintf("g%02d", i), IndexerID: indexerID, Seeders: i, Resolve: resolve}
	}
	return rows
}

func keys(rows []search.Result) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.GUID
	}
	return out
}

func finish(r *search.Resolver, rows []search.Result) {
	for _, row := range rows {
		r.Done(row, nil)
	}
}

func TestResolver_TwoAtATimeBestSeededFirst(t *testing.T) {
	r := search.NewResolver()
	rows := pending(5, 1)

	first := r.Next(rows, "", false)

	assert.Equal(t, []string{"g04", "g03"}, keys(first))
	assert.Empty(t, r.Next(rows, "", false), "two are already out")
	assert.True(t, r.Busy())

	r.Done(first[0], nil)
	assert.Equal(t, []string{"g02"}, keys(r.Next(rows, "", false)))
}

func TestResolver_OnlyTheFirstTenGoWithoutTheCursor(t *testing.T) {
	r := search.NewResolver()
	rows := pending(30, 1)
	total := 0
	for {
		batch := r.Next(rows, "", false)
		if len(batch) == 0 {
			break
		}
		total += len(batch)
		finish(r, batch)
	}
	assert.Equal(t, search.ResolveUpFront, total)
	assert.False(t, r.Busy())
}

func TestResolver_TheCursorTakesTheBudgetToTwenty(t *testing.T) {
	r := search.NewResolver()
	rows := pending(30, 1)
	for {
		batch := r.Next(rows, "", false)
		if len(batch) == 0 {
			break
		}
		finish(r, batch)
	}

	started := 0
	for i := range 15 {
		batch := r.Next(rows, rows[i].Key(), false)
		started += len(batch)
		finish(r, batch)
	}
	assert.Equal(t, search.ResolveBudget-search.ResolveUpFront, started)
}

func TestResolver_EnterResolvesPastTheBudget(t *testing.T) {
	r := search.NewResolver()
	rows := pending(30, 1)
	for i := range 15 {
		finish(r, r.Next(rows, rows[i].Key(), false))
	}
	assert.Empty(t, r.Next(rows, rows[15].Key(), false), "the budget is spent")

	got := r.Next(rows, rows[15].Key(), true)

	assert.Equal(t, []string{"g15"}, keys(got))
}

func TestResolver_NeverResolvesARowTwice(t *testing.T) {
	r := search.NewResolver()
	rows := pending(1, 1)
	finish(r, r.Next(rows, "", false))

	assert.Empty(t, r.Next(rows, rows[0].Key(), true))
}

func TestResolver_ARateLimitStopsThatIndexerOnly(t *testing.T) {
	r := search.NewResolver()
	rows := append(pending(3, 1), search.Result{GUID: "other", IndexerID: 2, Seeders: -1, Resolve: pending(1, 2)[0].Resolve})
	first := r.Next(rows, "", false)

	r.Done(first[0], fmt.Errorf("download: %w", search.ErrRateLimited))
	r.Done(first[1], errors.New("boom"))

	assert.True(t, r.Limited(1))
	assert.False(t, r.Limited(2))
	assert.Equal(t, []string{"other"}, keys(r.Next(rows, "", false)))
}

func TestResolver_SkipsRowsThatAreNotPending(t *testing.T) {
	r := search.NewResolver()
	rows := []search.Result{{GUID: "hashed", Hash: sampleHex, Resolve: pending(1, 1)[0].Resolve}, {GUID: "no-link"}}

	assert.Empty(t, r.Next(rows, "", false))
}
