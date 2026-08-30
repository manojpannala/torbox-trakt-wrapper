package tui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type AuthModal struct {
	DeviceCode *trakt.DeviceCodeResponse
	Spinner    spinner.Model
	Copied     bool
	StatusText string
}

func NewAuthModal() AuthModal {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ColorMauve)
	return AuthModal{
		Spinner:    s,
		StatusText: "Waiting for device authorization on trakt.tv...",
	}
}

func (m AuthModal) Render(theme Theme, width int) string {
	var sb strings.Builder
	sb.WriteString(theme.ModalHeader.Render("Trakt.tv Device Pairing"))
	sb.WriteString("\n\n")

	if m.DeviceCode == nil {
		sb.WriteString(m.Spinner.View())
		sb.WriteString(" Generating device code...\n")
		return theme.ModalBox.Width(min(60, width-4)).Render(sb.String())
	}

	sb.WriteString(lipgloss.NewStyle().Foreground(ColorText).Render(
		"1. Open verification URL in your browser:\n",
	))

	urlStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorSky).Underline(true)
	sb.WriteString("   " + urlStyle.Render(m.DeviceCode.VerificationURL) + "\n\n")

	sb.WriteString(lipgloss.NewStyle().Foreground(ColorText).Render(
		"2. Enter this one-time pairing code:\n\n",
	))

	codeBox := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorMauve).
		Background(ColorSurface0).
		Padding(0, 3).
		Render(m.DeviceCode.UserCode)

	sb.WriteString("   " + codeBox + "\n\n")

	if m.Copied {
		sb.WriteString(lipgloss.NewStyle().Foreground(ColorGreen).Render("   ✓ Copied code to clipboard!\n\n"))
	}

	sb.WriteString(m.Spinner.View())
	sb.WriteString(" " + lipgloss.NewStyle().Foreground(ColorSubtext0).Render(m.StatusText) + "\n\n")

	closeBtn := theme.ModalButton.Render("[Esc] Cancel")
	sb.WriteString(closeBtn)

	return theme.ModalBox.Width(min(60, width-4)).Render(sb.String())
}
