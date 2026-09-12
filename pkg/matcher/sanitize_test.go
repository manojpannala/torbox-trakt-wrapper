package matcher_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

// A torrent name is chosen by whoever created the torrent, so it is hostile
// input rendered straight into a terminal.
func TestSanitizeDisplay_StripsTerminalControlSequences(t *testing.T) {
	c1CSI := string(rune(0x9b)) // the single-byte form of ESC [

	for _, tc := range []struct{ name, in, want string }{
		{"clear screen", "Movie\x1b[2JEVIL", "Movie[2JEVIL"},
		{"osc 52 clipboard write", "Movie\x1b]52;c;aGF4\x07", "Movie]52;c;aGF4"},
		{"carriage return overwrite", "Real Title\rFake Title", "Real TitleFake Title"},
		{"c1 csi introducer", "Movie" + c1CSI + "2JEVIL", "Movie2JEVIL"},
		{"del", "Movie\x7fX", "MovieX"},
		{"plain text untouched", "Test Feature Alpha (2023)", "Test Feature Alpha (2023)"},
		{"unicode untouched", "日本語のタイトル", "日本語のタイトル"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, matcher.SanitizeDisplay(tc.in))
		})
	}
}

func TestDisplayTitle_IsSafeToRender(t *testing.T) {
	parsed := matcher.ParseMedia("Movie\x1b[2J\x1b]52;c;aGF4\x07.2023.1080p.mkv")

	assert.NotContains(t, parsed.DisplayTitle(), "\x1b")
}
