package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusStyle_MapsDownloadStateToASemanticRole(t *testing.T) {
	th := DefaultTheme()

	for _, tc := range []struct {
		state string
		want  string
	}{
		{"completed", "ok"},
		{"cached", "ok"},
		{"uploading", "ok"},
		{"downloading", "warn"},
		{"checking", "warn"},
		{"paused", "warn"},
		{"stalled", "warn"},
		{"failed", "error"},
		{"error", "error"},
		{"download failed", "error"},
	} {
		got := statusStyle(th, tc.state)
		switch tc.want {
		case "ok":
			assert.Equal(t, th.ItemStatusOk, got, "state %q", tc.state)
		case "warn":
			assert.Equal(t, th.ItemStatusWarn, got, "state %q", tc.state)
		case "error":
			assert.Equal(t, th.ItemStatusError, got, "state %q", tc.state)
		}
	}
}

func TestStatusStyle_DistinguishesFailureFromSuccess(t *testing.T) {
	th := DefaultTheme()

	assert.NotEqual(t, statusStyle(th, "completed"), statusStyle(th, "failed"),
		"a failed download rendered identically to a completed one")
}
