package tui

import (
	"charm.land/lipgloss/v2"
)

var (
	ColorRosewater = lipgloss.Color("#f5e0dc")
	ColorFlamingo  = lipgloss.Color("#f2cdcd")
	ColorPink      = lipgloss.Color("#f5c2e7")
	ColorMauve     = lipgloss.Color("#cba6f7")
	ColorRed       = lipgloss.Color("#f38ba8")
	ColorMaroon    = lipgloss.Color("#eba0ac")
	ColorPeach     = lipgloss.Color("#fab387")
	ColorYellow    = lipgloss.Color("#f9e2af")
	ColorGreen     = lipgloss.Color("#a6e3a1")
	ColorTeal      = lipgloss.Color("#94e2d5")
	ColorSky       = lipgloss.Color("#89dceb")
	ColorSapphire  = lipgloss.Color("#74c7ec")
	ColorBlue      = lipgloss.Color("#89b4fa")
	ColorLavender  = lipgloss.Color("#b4befe")
	ColorText      = lipgloss.Color("#cdd6f4")
	ColorSubtext1  = lipgloss.Color("#bac2de")
	ColorSubtext0  = lipgloss.Color("#a6adc8")
	ColorOverlay2  = lipgloss.Color("#9399b2")
	ColorOverlay1  = lipgloss.Color("#7f849c")
	ColorOverlay0  = lipgloss.Color("#6c7086")
	ColorSurface2  = lipgloss.Color("#585b70")
	ColorSurface1  = lipgloss.Color("#45475a")
	ColorSurface0  = lipgloss.Color("#313244")
	ColorBase      = lipgloss.Color("#1e1e2e")
	ColorMantle    = lipgloss.Color("#181825")
	ColorCrust     = lipgloss.Color("#11111b")
)

type Theme struct {
	AppTitle lipgloss.Style
	Header   lipgloss.Style

	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	TabsBorder  lipgloss.Style

	ItemTitle       lipgloss.Style
	ItemSelected    lipgloss.Style
	ItemCursor      lipgloss.Style
	ItemSize        lipgloss.Style
	ItemStatusOk    lipgloss.Style
	ItemStatusWarn  lipgloss.Style
	ItemStatusError lipgloss.Style

	BadgeWatched    lipgloss.Style
	BadgeInProgress lipgloss.Style
	BadgeUnwatched  lipgloss.Style

	StatusBar   lipgloss.Style
	StatusKey   lipgloss.Style
	StatusDesc  lipgloss.Style
	StatusError lipgloss.Style
	StatusInfo  lipgloss.Style

	ModalBox     lipgloss.Style
	ModalHeader  lipgloss.Style
	ModalBody    lipgloss.Style
	ModalButton  lipgloss.Style
	ModalFocus   lipgloss.Style
	SearchPrompt lipgloss.Style
}

func DefaultTheme() Theme {
	return Theme{
		AppTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMauve).
			Background(ColorSurface0).
			Padding(0, 1),

		Header: lipgloss.NewStyle().
			Foreground(ColorText).
			Background(ColorMantle).
			Padding(0, 1),

		TabActive: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMantle).
			Background(ColorMauve).
			Padding(0, 2),

		TabInactive: lipgloss.NewStyle().
			Foreground(ColorSubtext0).
			Background(ColorSurface0).
			Padding(0, 2),

		TabsBorder: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(ColorSurface1),

		ItemTitle: lipgloss.NewStyle().
			Foreground(ColorText),

		ItemSelected: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMauve).
			Background(ColorSurface0),

		ItemCursor: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMauve),

		ItemSize: lipgloss.NewStyle().
			Foreground(ColorOverlay2),

		ItemStatusOk: lipgloss.NewStyle().
			Foreground(ColorGreen),

		ItemStatusWarn: lipgloss.NewStyle().
			Foreground(ColorPeach),

		ItemStatusError: lipgloss.NewStyle().
			Foreground(ColorRed),

		BadgeWatched: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorGreen),

		BadgeInProgress: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPeach),

		BadgeUnwatched: lipgloss.NewStyle().
			Foreground(ColorOverlay0),

		StatusBar: lipgloss.NewStyle().
			Foreground(ColorSubtext1).
			Background(ColorSurface0).
			Padding(0, 1),

		StatusKey: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMauve).
			Background(ColorSurface1).
			Padding(0, 1),

		StatusDesc: lipgloss.NewStyle().
			Foreground(ColorSubtext0).
			Padding(0, 1),

		StatusError: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorRed),

		StatusInfo: lipgloss.NewStyle().
			Foreground(ColorBlue),

		ModalBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMauve).
			Background(ColorBase).
			Padding(1, 2),

		ModalHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMauve).
			MarginBottom(1),

		ModalBody: lipgloss.NewStyle().
			Foreground(ColorText),

		ModalButton: lipgloss.NewStyle().
			Foreground(ColorSubtext1).
			Background(ColorSurface1).
			Padding(0, 2).
			Margin(1, 1, 0, 0),

		ModalFocus: lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMantle).
			Background(ColorMauve).
			Padding(0, 2).
			Margin(1, 1, 0, 0),

		SearchPrompt: lipgloss.NewStyle().
			Foreground(ColorMauve).
			Bold(true),
	}
}
