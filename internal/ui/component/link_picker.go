package component

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
)

type LinkPickedMsg struct {
	URL string
}

type LinkPickerEntry struct {
	Label string
	URL   string
}

type LinkPicker struct {
	entries []LinkPickerEntry
	cursor  int
	width   int
}

func NewLinkPicker(msg *slack.Message, maxWidth int) LinkPicker {
	var entries []LinkPickerEntry

	// Add file URLs first (images are the primary use case)
	for _, f := range msg.Files {
		if f.URL == "" {
			continue
		}
		label := f.Name
		if strings.HasPrefix(f.Mimetype, "image/") {
			label = fmt.Sprintf("[image] %s", f.Name)
		} else {
			label = fmt.Sprintf("[file]  %s", f.Name)
		}
		entries = append(entries, LinkPickerEntry{Label: label, URL: f.URL})
	}

	// Add text URLs
	seen := make(map[string]bool)
	for _, e := range entries {
		seen[e.URL] = true
	}
	for _, u := range slack.ExtractURLs(msg.Text) {
		if seen[u] {
			continue
		}
		label := u
		if len(label) > maxWidth-8 {
			label = label[:maxWidth-11] + "..."
		}
		entries = append(entries, LinkPickerEntry{Label: label, URL: u})
	}

	return LinkPicker{entries: entries, width: maxWidth}
}

func (p *LinkPicker) Entries() []LinkPickerEntry {
	return p.entries
}

func (p *LinkPicker) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter", "l":
			if p.cursor >= 0 && p.cursor < len(p.entries) {
				url := p.entries[p.cursor].URL
				return func() tea.Msg {
					return LinkPickedMsg{URL: url}
				}
			}
		case "j", "down", "ctrl+n":
			if p.cursor < len(p.entries)-1 {
				p.cursor++
			}
		case "k", "up", "ctrl+p":
			if p.cursor > 0 {
				p.cursor--
			}
		}
	}
	return nil
}

func (p LinkPicker) View() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	b.WriteString(titleStyle.Render("  Open link") + "\n\n")

	for i, e := range p.entries {
		cursor := "  "
		itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Italic(true)
		if i == p.cursor {
			cursor = "> "
			itemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true).Italic(true)
		}
		b.WriteString(cursor + itemStyle.Render(e.Label) + "\n")
	}

	w := p.width
	if w < 40 {
		w = 40
	}
	if w > 80 {
		w = 80
	}

	style := lipgloss.NewStyle().
		Width(w).
		Padding(1, 0).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("33")).
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("252"))

	return style.Render(b.String())
}
