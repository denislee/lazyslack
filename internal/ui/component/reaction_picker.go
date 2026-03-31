package component

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type ReactionPickedMsg struct {
	Emoji string
}

type ReactionPicker struct {
	input    textinput.Model
	emojis   []emojiEntry
	filtered []emojiEntry
	cursor   int
	width    int
	height   int
}

type emojiEntry struct {
	name    string
	unicode string
}

func NewReactionPicker(width, height int) ReactionPicker {
	ti := textinput.New()
	ti.Placeholder = "Search emoji..."
	ti.Focus()
	ti.SetWidth(30)

	emojis := defaultEmojis()
	return ReactionPicker{
		input:    ti,
		emojis:   emojis,
		filtered: emojis,
		width:    width,
		height:   height,
	}
}

func (r *ReactionPicker) Init() tea.Cmd {
	return textinput.Blink
}

func (r *ReactionPicker) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if len(r.filtered) > 0 && r.cursor < len(r.filtered) {
				return func() tea.Msg {
					return ReactionPickedMsg{Emoji: r.filtered[r.cursor].name}
				}
			}
			return nil
		case "down", "ctrl+n":
			if r.cursor < len(r.filtered)-1 {
				r.cursor++
			}
			return nil
		case "up", "ctrl+p":
			if r.cursor > 0 {
				r.cursor--
			}
			return nil
		}
	}

	var cmd tea.Cmd
	r.input, cmd = r.input.Update(msg)

	// Filter emojis
	query := strings.ToLower(strings.TrimSpace(r.input.Value()))
	if query == "" {
		r.filtered = r.emojis
	} else {
		r.filtered = r.filtered[:0]
		for _, e := range r.emojis {
			if strings.Contains(e.name, query) {
				r.filtered = append(r.filtered, e)
			}
		}
	}
	if r.cursor >= len(r.filtered) {
		r.cursor = max(0, len(r.filtered)-1)
	}

	return cmd
}

func (r ReactionPicker) View() string {
	var b strings.Builder
	b.WriteString("  " + r.input.View() + "\n\n")

	maxVisible := 10
	start := 0
	if r.cursor >= maxVisible {
		start = r.cursor - maxVisible + 1
	}

	for i := start; i < len(r.filtered) && i < start+maxVisible; i++ {
		e := r.filtered[i]
		line := fmt.Sprintf("  %s  :%s:", e.unicode, e.name)
		if i == r.cursor {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color("170")).
				Bold(true).
				Render("> " + e.unicode + "  :" + e.name + ":")
		}
		b.WriteString(line + "\n")
	}

	if len(r.filtered) == 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("  No matching emoji"))
	}

	style := lipgloss.NewStyle().
		Width(40).
		Padding(1, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("33")).
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("252"))

	return lipgloss.Place(
		r.width, r.height,
		lipgloss.Center, lipgloss.Center,
		style.Render(b.String()),
	)
}

func defaultEmojis() []emojiEntry {
	return []emojiEntry{
		{"+1", "\U0001F44D"},
		{"-1", "\U0001F44E"},
		{"heart", "\u2764\uFE0F"},
		{"fire", "\U0001F525"},
		{"eyes", "\U0001F440"},
		{"tada", "\U0001F389"},
		{"rocket", "\U0001F680"},
		{"100", "\U0001F4AF"},
		{"thumbsup", "\U0001F44D"},
		{"thumbsdown", "\U0001F44E"},
		{"joy", "\U0001F602"},
		{"smile", "\U0001F604"},
		{"thinking_face", "\U0001F914"},
		{"wave", "\U0001F44B"},
		{"pray", "\U0001F64F"},
		{"clap", "\U0001F44F"},
		{"raised_hands", "\U0001F64C"},
		{"white_check_mark", "\u2705"},
		{"x", "\u274C"},
		{"warning", "\u26A0\uFE0F"},
		{"bulb", "\U0001F4A1"},
		{"star", "\u2B50"},
		{"sparkles", "\u2728"},
		{"zap", "\u26A1"},
		{"muscle", "\U0001F4AA"},
		{"ok_hand", "\U0001F44C"},
		{"sunglasses", "\U0001F60E"},
		{"sweat_smile", "\U0001F605"},
		{"sob", "\U0001F62D"},
		{"skull", "\U0001F480"},
		{"ghost", "\U0001F47B"},
		{"coffee", "\u2615"},
		{"beer", "\U0001F37A"},
		{"pizza", "\U0001F355"},
		{"trophy", "\U0001F3C6"},
		{"gem", "\U0001F48E"},
		{"bell", "\U0001F514"},
		{"memo", "\U0001F4DD"},
		{"bug", "\U0001F41B"},
		{"wrench", "\U0001F527"},
	}
}
