package tui

import (
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestTruncateToWidth_LeavesShortTitlesAlone(t *testing.T) {
	assert.Equal(t, "Test.Feature.Alpha.mkv", truncateToWidth("Test.Feature.Alpha.mkv", 40))
}

func TestTruncateToWidth_CutsAsciiToTheColumn(t *testing.T) {
	got := truncateToWidth("Test.Feature.Alpha.2023.1080p.mkv", 20)

	assert.Equal(t, "Test.Feature.Alph...", got)
	assert.Equal(t, 20, lipgloss.Width(got))
}

func TestTruncateToWidth_CountsDoubleWidthCellsNotBytes(t *testing.T) {
	// 10 runes, 30 bytes, 20 display cells. The byte-based version cut this to
	// a third of the intended column.
	got := truncateToWidth("日本語のタイトルです", 12)

	assert.Equal(t, "日本語の...", got)
	assert.LessOrEqual(t, lipgloss.Width(got), 12)
}

func TestTruncateToWidth_NeverSplitsARune(t *testing.T) {
	got := truncateToWidth("日本語のタイトルですね", 11)

	assert.True(t, utf8.ValidString(got), "byte slicing emitted invalid UTF-8: %q", got)
	assert.LessOrEqual(t, lipgloss.Width(got), 11)
}

func TestPadToWidth_PadsByDisplayCellsNotRunes(t *testing.T) {
	// 8 runes, 16 cells. Go's %-20s pads to 20 *runes* — 28 cells — which
	// shoves every column to its right.
	assert.Equal(t, 20, lipgloss.Width(padToWidth("日本語のタイトル", 20)))
	assert.Equal(t, 20, lipgloss.Width(padToWidth("AbcdefghAbcdefgh", 20)))
}
