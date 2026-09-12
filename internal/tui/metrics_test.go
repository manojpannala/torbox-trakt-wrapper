package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderMetrics_ShowsSpeedEtaAndSeedsWhileDownloading(t *testing.T) {
	got := renderMetrics(LibraryItem{
		Speed: 12 * 1024 * 1024,
		ETA:   300,
		Seeds: 42,
	})

	assert.Equal(t, "12.0 MiB/s  ETA 5m  S:42", got)
}

func TestRenderMetrics_IsEmptyForAFinishedItem(t *testing.T) {
	assert.Empty(t, renderMetrics(LibraryItem{DownloadState: "completed"}))
}

func TestRenderMetrics_OmitsWhateverIsUnknown(t *testing.T) {
	assert.Equal(t, "S:7", renderMetrics(LibraryItem{Seeds: 7}))
	assert.Equal(t, "1.0 MiB/s", renderMetrics(LibraryItem{Speed: 1024 * 1024}))
}

func TestFormatDuration_ScalesToTheMagnitude(t *testing.T) {
	assert.Equal(t, "45s", formatDuration(45))
	assert.Equal(t, "5m", formatDuration(300))
	assert.Equal(t, "2h05m", formatDuration(7500))
}
