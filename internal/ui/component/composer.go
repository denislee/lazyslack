package component

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type Composer struct {
	textarea textarea.Model
	focused  bool
	width    int
}

func NewComposer(width int) Composer {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.SetWidth(width - 2)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.CharLimit = 4000
	return Composer{
		textarea: ta,
		width:    width,
	}
}

func (c *Composer) Focus() tea.Cmd {
	c.focused = true
	return c.textarea.Focus()
}

func (c *Composer) Blur() {
	c.focused = false
	c.textarea.Blur()
}

func (c *Composer) IsFocused() bool {
	return c.focused
}

func (c *Composer) Value() string {
	return c.textarea.Value()
}

func (c *Composer) Reset() {
	c.textarea.Reset()
}

func (c *Composer) SetWidth(w int) {
	c.width = w
	c.textarea.SetWidth(w - 2)
}

func (c *Composer) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.textarea, cmd = c.textarea.Update(msg)
	return cmd
}

func (c Composer) View() string {
	style := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(c.width - 2)

	if c.focused {
		style = style.BorderForeground(lipgloss.Color("33"))
	}

	return style.Render(c.textarea.View())
}
