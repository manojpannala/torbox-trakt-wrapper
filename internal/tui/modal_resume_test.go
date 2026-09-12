package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHumanizeSince(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name string
		when time.Time
		want string
	}{
		{"seconds", now.Add(-20 * time.Second), "just now"},
		{"one minute", now.Add(-1 * time.Minute), "1 minute ago"},
		{"minutes", now.Add(-25 * time.Minute), "25 minutes ago"},
		{"one hour", now.Add(-90 * time.Minute), "1 hour ago"},
		{"hours", now.Add(-5 * time.Hour), "5 hours ago"},
		{"one day", now.Add(-30 * time.Hour), "1 day ago"},
		{"days", now.Add(-72 * time.Hour), "3 days ago"},
		{"long ago falls back to a date", now.AddDate(0, -3, 0), "on 2026-06-12"},
		{"unknown", time.Time{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, humanizeSince(tc.when, now))
		})
	}
}

func TestRenderResumeModal_ShowsThePositionAndWhen(t *testing.T) {
	p := &pendingResume{
		title:    "Test Feature Alpha (2023)",
		percent:  41,
		pausedAt: time.Now().Add(-72 * time.Hour),
	}

	out := renderResumeModal(DefaultTheme(), p, 120)

	assert.Contains(t, out, "Test Feature Alpha (2023)")
	assert.Contains(t, out, "41%")
	assert.Contains(t, out, "3 days ago")
	assert.Contains(t, out, "[r]")
	assert.Contains(t, out, "[s]")
}

func TestRenderResumeModal_SurvivesANilPrompt(t *testing.T) {
	assert.NotPanics(t, func() { renderResumeModal(DefaultTheme(), nil, 120) })
}
