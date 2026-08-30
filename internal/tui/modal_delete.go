package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func renderDeleteModal(theme Theme, item *LibraryItem, width int) string {
	var sb strings.Builder
	sb.WriteString(theme.ModalHeader.Render("Confirm Deletion"))
	sb.WriteString("\n\n")

	itemName := ""
	if item != nil {
		itemName = item.CleanTitle
		if itemName == "" {
			itemName = item.RawName
		}
	}

	sb.WriteString(lipgloss.NewStyle().Foreground(ColorText).Render(
		fmt.Sprintf("Are you sure you want to remove:\n\n  %s\n\nThis will delete the item from your TorBox cloud library.",
			lipgloss.NewStyle().Bold(true).Foreground(ColorPeach).Render(itemName),
		),
	))
	sb.WriteString("\n\n")

	confirmBtn := theme.ModalFocus.Render("[y] Delete")
	cancelBtn := theme.ModalButton.Render("[n] Cancel")

	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, confirmBtn, "  ", cancelBtn))

	return theme.ModalBox.Width(min(62, width-2)).Render(sb.String())
}
