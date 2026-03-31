package screen

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Screen is the interface for all screens in the app.
// Unlike tea.Model, View() returns string so the root App can compose them.
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View() string
	SetSize(width, height int)
	ShortHelp() []key.Binding
}
