package tui

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStderrTail_PicksTheInformativeMpvLine(t *testing.T) {
	w := &outputTail{}
	_, _ = w.Write([]byte("Error parsing option vfs-cache-max-size (option not found)\n" +
		"Setting commandline option --vfs-cache-max-size=5G failed.\n" +
		"Exiting... (Fatal error)\n"))

	assert.Equal(t, "Error parsing option vfs-cache-max-size (option not found)", w.errorLine())
}

func TestStderrTail_FallsBackWhenNoErrorKeyword(t *testing.T) {
	w := &outputTail{}
	_, _ = w.Write([]byte("Failed to recognize file format.\nExiting... (Errors when loading file)\n"))

	assert.Equal(t, "Failed to recognize file format.", w.errorLine())
}

func TestStderrTail_EmptyWhenNothingWritten(t *testing.T) {
	assert.Empty(t, (&outputTail{}).errorLine())
}

func TestStderrTail_KeepsOnlyTheTailAndStaysBounded(t *testing.T) {
	w := &outputTail{}
	for i := 0; i < 2000; i++ {
		_, _ = w.Write([]byte("noise line that is reasonably long to fill the buffer\n"))
	}
	_, _ = w.Write([]byte("Error: the real failure\n"))

	assert.LessOrEqual(t, len(w.buf), outputTailLimit)
	assert.Equal(t, "Error: the real failure", w.errorLine())
}

func TestStderrTail_TruncatesOverlongLines(t *testing.T) {
	w := &outputTail{}
	_, _ = w.Write([]byte("Error: " + strings.Repeat("x", 500) + "\n"))

	got := w.errorLine()
	assert.Equal(t, 100, len([]rune(got)))
	assert.True(t, strings.HasSuffix(got, "…"))
}

func TestOutputTail_HandlesCarriageReturnStatusNoise(t *testing.T) {
	w := &outputTail{}
	_, _ = w.Write([]byte("AV: 00:00:01 / 00:10:00\rAV: 00:00:02 / 00:10:00\r"))
	_, _ = w.Write([]byte("Error: stream lost\n"))

	assert.Equal(t, "Error: stream lost", w.errorLine())
}

func TestOutputTail_RealMpvFailureIsCaptured(t *testing.T) {
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("mpv not installed")
	}

	c := exec.Command("mpv", "--vfs-cache-max-size=5G", "/dev/null")
	tail := &outputTail{}
	c.Stdout = tail
	c.Stderr = tail

	require.Error(t, c.Run())
	assert.Contains(t, tail.errorLine(), "vfs-cache-max-size")
}
