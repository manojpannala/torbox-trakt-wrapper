package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

type AddModal struct {
	Input textinput.Model
	// FromClipboard marks a value the user did not type, so the modal can say
	// so before Enter adds it.
	FromClipboard bool
}

func NewAddModal() AddModal {
	ti := textinput.New()
	ti.Placeholder = "magnet:?xt=urn:btih:... or https://..."
	ti.Focus()
	ti.CharLimit = 2048
	ti.SetWidth(50)
	return AddModal{Input: ti}
}

func (m AddModal) Render(theme Theme, width int) string {
	var sb strings.Builder
	sb.WriteString(theme.ModalHeader.Render("Add Download (Magnet / URL)"))
	sb.WriteString("\n\n")

	if m.FromClipboard {
		sb.WriteString(lipgloss.NewStyle().Foreground(ColorPeach).Render("Pasted from your clipboard \u2014 check it before adding:"))
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(ColorSubtext0).Render("Paste magnet link, NZB link, or direct media URL:"))
	}
	sb.WriteString("\n\n")

	sb.WriteString(m.Input.View())
	sb.WriteString("\n\n")

	confirmBtn := theme.ModalFocus.Render("[Enter] Add")
	cancelBtn := theme.ModalButton.Render("[Esc] Cancel")

	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, confirmBtn, "  ", cancelBtn))

	return theme.ModalBox.Width(min(66, width-2)).Render(sb.String())
}

// prefilledAddModal seeds the modal from the clipboard with the cursor at the
// start, so the scheme stays visible for the user to check.
func prefilledAddModal(value string) AddModal {
	m := NewAddModal()
	m.Input.SetValue(value)
	m.Input.SetCursor(0)
	m.FromClipboard = true
	return m
}

// clipboardPrefill reports whether clipboard text looks like something this
// modal can add, so unrelated clipboard contents are never pulled in.
func clipboardPrefill(clip string) (string, bool) {
	clip = strings.TrimSpace(clip)
	for _, prefix := range []string{"magnet:?", "http://", "https://"} {
		if strings.HasPrefix(clip, prefix) {
			return clip, true
		}
	}
	return "", false
}
