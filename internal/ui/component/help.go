package component

import (
	"charm.land/lipgloss/v2"
)

type HelpOverlay struct {
	width  int
	height int
}

func NewHelpOverlay(width, height int) HelpOverlay {
	return HelpOverlay{width: width, height: height}
}

func (h HelpOverlay) View() string {
	helpText := `
  lazyslack — Keyboard Reference

  Global
    ctrl+c     Quit
    ?          Toggle help
    /          Search
    ctrl+k     Channel switcher
    ctrl+l     Refresh
    esc        Back / close

  Channel List
    j/k ↑/↓    Navigate
    enter      Open channel
    f/tab      Filter channels
    u          Unread only
    q          Quit

  Chat (Normal Mode)
    j/k ↑/↓    Navigate messages
    g/G        Top / bottom
    ctrl+u/d   Page up / down
    i          Compose message
    enter      Open thread
    r          Reply in thread
    +          Add reaction
    esc        Back to channels

  Chat (Insert Mode)
    enter      Send message
    esc        Cancel / normal mode

  Thread
    Same as Chat, esc returns to chat

  Search
    (typing)   Search query
    ctrl+n/p   Navigate results
    enter      Jump to message
    esc        Close
`

	style := lipgloss.NewStyle().
		Width(50).
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("33")).
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("252"))

	return lipgloss.Place(
		h.width, h.height,
		lipgloss.Center, lipgloss.Center,
		style.Render(helpText),
	)
}
