package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// pendingResume holds a stream waiting on the user's resume decision.
type pendingResume struct {
	title    string
	percent  float64
	pausedAt time.Time
	play     func(resumePercent float64) tea.Cmd
}

func renderResumeModal(theme Theme, p *pendingResume, width int) string {
	if p == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(theme.ModalHeader.Render("Resume Playback"))
	sb.WriteString("\n\n")

	body := lipgloss.NewStyle().Foreground(ColorText)
	sb.WriteString(body.Render("  ") +
		lipgloss.NewStyle().Bold(true).Foreground(ColorPeach).Render(p.title))
	sb.WriteString("\n")

	stopped := fmt.Sprintf("  You stopped %.0f%% in", p.percent)
	if when := humanizeSince(p.pausedAt, time.Now()); when != "" {
		stopped += ", " + when
	}
	sb.WriteString(body.Render(stopped + "."))
	sb.WriteString("\n\n")

	// the button styles already carry padding and a right margin
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Left,
		theme.ModalFocus.Render(fmt.Sprintf("[r] Resume %.0f%%", p.percent)),
		theme.ModalButton.Render("[s] Start over"),
		theme.ModalButton.Render("[esc] Cancel"),
	))

	return theme.ModalBox.Width(min(66, width-2)).Render(sb.String())
}

// humanizeSince renders how long ago a playback position was recorded.
func humanizeSince(when, now time.Time) string {
	if when.IsZero() {
		return ""
	}

	d := now.Sub(when)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	case d < 30*24*time.Hour:
		return plural(int(d.Hours()/24), "day")
	default:
		return "on " + when.Format("2006-01-02")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s ago", unit)
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}
