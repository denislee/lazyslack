package ui

import "charm.land/lipgloss/v2"

var (
	// Colors
	ColorPrimary   = lipgloss.Color("33")  // blue
	ColorSecondary = lipgloss.Color("240") // gray
	ColorSuccess   = lipgloss.Color("42")  // green
	ColorError     = lipgloss.Color("196") // red
	ColorWarning   = lipgloss.Color("214") // orange
	ColorHighlight = lipgloss.Color("170") // purple
	ColorMuted     = lipgloss.Color("245") // light gray

	// Message styles
	StyleUsername = lipgloss.NewStyle().Bold(true)
	StyleTime    = lipgloss.NewStyle().Foreground(ColorSecondary)
	StyleMuted   = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleError   = lipgloss.NewStyle().Foreground(ColorError)
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSuccess)

	// Code styles
	StyleCode = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))
	StyleCodeBlock = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	// UI chrome
	StyleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)
	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Padding(0, 1)

	// Selection
	StyleSelected = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(ColorPrimary).
			PaddingLeft(1)
	StyleUnselected = lipgloss.NewStyle().
			PaddingLeft(3)

	// Reactions
	StyleReaction = lipgloss.NewStyle().
			Background(lipgloss.Color("237")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)
	StyleReactionOwn = lipgloss.NewStyle().
			Background(lipgloss.Color("24")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	// Thread indicator
	StyleThread = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Italic(true)

	// Channel list
	StyleChannelName    = lipgloss.NewStyle()
	StyleChannelUnread  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	StyleChannelPrefix  = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleDMPrefix       = lipgloss.NewStyle().Foreground(ColorHighlight)
)
