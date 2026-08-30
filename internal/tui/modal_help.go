package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func renderHelpModal(theme Theme, width int) string {
	keys := []struct {
		key  string
		desc string
	}{
		{"Enter / Space", "Stream media with MPV"},
		{"Tab / 1,2,3", "Switch tabs (Torrents / Usenet / Web-DL)"},
		{"f / o", "Open file tree / folder browser"},
		{"/ ", "Search / filter library list"},
		{"Esc", "Clear search / close modal / back"},
		{"a", "Add new torrent (Magnet / URL)"},
		{"d / x", "Delete selected item"},
		{"r", "Refresh library and Trakt history"},
		{"p", "Pause / resume active torrent"},
		{"A", "Authenticate Trakt (Device Flow)"},
		{"?", "Toggle this help overlay"},
		{"q / Ctrl+C", "Quit application"},
	}

	var sb strings.Builder
	sb.WriteString(theme.ModalHeader.Render("Keyboard Shortcuts"))
	sb.WriteString("\n\n")

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Width(16)
	descStyle := lipgloss.NewStyle().Foreground(ColorText)

	for _, k := range keys {
		sb.WriteString(keyStyle.Render(k.key))
		sb.WriteString(descStyle.Render(k.desc))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(theme.ModalFocus.Render("Press Esc or ? to Close"))

	return theme.ModalBox.Width(min(56, width-4)).Render(sb.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
