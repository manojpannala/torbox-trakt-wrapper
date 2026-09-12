package matcher

import "strings"

// SanitizeDisplay removes control characters from text that came from an API
// and is about to be written to a terminal. Torrent and file names are chosen
// by whoever created the torrent, and an escape sequence in one can clear the
// screen, redraw the UI to impersonate another item, or write to the clipboard
// via OSC 52.
func SanitizeDisplay(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		default:
			return r
		}
	}, s)
}
