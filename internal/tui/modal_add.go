package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

type AddModal struct {
	Input textinput.Model
}

func NewAddModal() AddModal {
	ti := textinput.New()
	ti.Placeholder = "magnet:?xt=urn:btih:... or https://..."
	ti.Focus()
	ti.CharLimit = 2048
	ti.Width = 50
	return AddModal{Input: ti}
}

func (m AddModal) Render(theme Theme, width int) string {
	var sb strings.Builder
	sb.WriteString(theme.ModalHeader.Render("Add Download (Magnet / URL)"))
	sb.WriteString("\n\n")

	sb.WriteString(lipgloss.NewStyle().Foreground(ColorSubtext0).Render("Paste magnet link, NZB link, or direct media URL:"))
	sb.WriteString("\n\n")

	sb.WriteString(m.Input.View())
	sb.WriteString("\n\n")

	confirmBtn := theme.ModalFocus.Render("[Enter] Add")
	cancelBtn := theme.ModalButton.Render("[Esc] Cancel")

	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, confirmBtn, "  ", cancelBtn))

	return theme.ModalBox.Width(min(64, width-4)).Render(sb.String())
}
