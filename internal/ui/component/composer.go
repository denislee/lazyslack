package component

import (
	"encoding/json"
	"os"
	"path/filepath"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const maxHistory = 500

type Composer struct {
	textarea     textarea.Model
	focused      bool
	width        int
	history      []string
	historyIndex int // -1 means "not navigating" (showing draft)
	draft        string // saves current input when navigating history
}

func NewComposer(width int) Composer {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.SetWidth(width - 2)
	ta.ShowLineNumbers = false
	ta.CharLimit = 4000
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 12

	c := Composer{
		textarea:     ta,
		width:        width,
		historyIndex: -1,
	}
	c.loadHistory()
	return c
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
	c.historyIndex = -1
	c.draft = ""
}

func (c *Composer) SetValue(s string) {
	c.textarea.Reset()
	c.textarea.InsertString(s)
}

func (c *Composer) SetWidth(w int) {
	c.width = w
	c.textarea.SetWidth(w - 2)
}

// Height returns the total rendered height of the composer (content + border).
func (c *Composer) Height() int {
	return c.textarea.Height() + 2
}

func (c *Composer) SaveToHistory(text string) {
	if text == "" {
		return
	}
	// Avoid consecutive duplicates
	if len(c.history) > 0 && c.history[len(c.history)-1] == text {
		return
	}
	c.history = append(c.history, text)
	if len(c.history) > maxHistory {
		c.history = c.history[len(c.history)-maxHistory:]
	}
	c.persistHistory()
}

func (c *Composer) Update(msg tea.Msg) tea.Cmd {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(kmsg, key.NewBinding(key.WithKeys("up", "ctrl+p"))) {
			c.historyPrev()
			return nil
		}
		if key.Matches(kmsg, key.NewBinding(key.WithKeys("down", "ctrl+n"))) {
			c.historyNext()
			return nil
		}
	}

	var cmd tea.Cmd
	c.textarea, cmd = c.textarea.Update(msg)
	return cmd
}

func (c *Composer) historyPrev() {
	if len(c.history) == 0 {
		return
	}
	if c.historyIndex == -1 {
		// Save current draft before navigating
		c.draft = c.textarea.Value()
		c.historyIndex = len(c.history) - 1
	} else if c.historyIndex > 0 {
		c.historyIndex--
	}
	c.setTextareaValue(c.history[c.historyIndex])
}

func (c *Composer) historyNext() {
	if c.historyIndex == -1 {
		return
	}
	if c.historyIndex < len(c.history)-1 {
		c.historyIndex++
		c.setTextareaValue(c.history[c.historyIndex])
	} else {
		// Back to draft
		c.historyIndex = -1
		c.setTextareaValue(c.draft)
	}
}

func (c *Composer) setTextareaValue(s string) {
	c.textarea.Reset()
	c.textarea.InsertString(s)
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

// Disk persistence

func (c *Composer) historyPath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "lazyslack", "history.json")
}

func (c *Composer) loadHistory() {
	data, err := os.ReadFile(c.historyPath())
	if err != nil {
		return
	}
	var history []string
	if err := json.Unmarshal(data, &history); err != nil {
		return
	}
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
	c.history = history
}

func (c *Composer) persistHistory() {
	p := c.historyPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(c.history)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0o644)
}
