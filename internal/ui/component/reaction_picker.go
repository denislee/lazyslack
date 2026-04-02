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
	input       textinput.Model
	emojis      []emojiEntry
	filtered    []emojiEntry
	myReactions map[string]bool
	cursor      int
}

// ExistingReaction describes a reaction already on the message.
type ExistingReaction struct {
	Name  string
	HasMe bool
}

type emojiEntry struct {
	name    string
	unicode string
}

func NewReactionPicker(existingReactions []ExistingReaction) ReactionPicker {
	ti := textinput.New()
	ti.Placeholder = "Search emoji..."
	ti.Prompt = " " // Empty prompt to avoid double cursor
	ti.Focus()
	ti.SetWidth(30)

	emojis := defaultEmojis()
	myReactions := make(map[string]bool)

	// Move existing reactions to the front of the list
	if len(existingReactions) > 0 {
		existing := make(map[string]bool, len(existingReactions))
		for _, r := range existingReactions {
			existing[r.Name] = true
			if r.HasMe {
				myReactions[r.Name] = true
			}
		}
		front := make([]emojiEntry, 0, len(existingReactions))
		rest := make([]emojiEntry, 0, len(emojis))
		for _, e := range emojis {
			if existing[e.name] {
				front = append(front, e)
			} else {
				rest = append(rest, e)
			}
		}
		// Add any existing reactions not in the default list
		seen := make(map[string]bool, len(front))
		for _, e := range front {
			seen[e.name] = true
		}
		for _, r := range existingReactions {
			if !seen[r.Name] {
				front = append(front, emojiEntry{name: r.Name})
			}
		}
		emojis = append(front, rest...)
	}

	return ReactionPicker{
		input:       ti,
		emojis:      emojis,
		filtered:    emojis,
		myReactions: myReactions,
	}
}

func (r *ReactionPicker) Init() tea.Cmd {
	return textinput.Blink
}

func (r *ReactionPicker) Update(msg tea.Msg) (tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if len(r.filtered) > 0 && r.cursor >= 0 && r.cursor < len(r.filtered) {
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
		case "esc", "ctrl+[":
			return nil
		}
	}

	var cmd tea.Cmd
	prevValue := r.input.Value()
	r.input, cmd = r.input.Update(msg)

	if r.input.Value() != prevValue {
		// Filter emojis from the full list
		query := strings.ToLower(strings.TrimSpace(r.input.Value()))
		if query == "" {
			r.filtered = r.emojis
		} else {
			filtered := make([]emojiEntry, 0)
			for _, e := range r.emojis {
				if strings.Contains(strings.ToLower(e.name), query) {
					filtered = append(filtered, e)
				}
			}
			r.filtered = filtered
		}
		r.cursor = 0 // Reset cursor on search
	}

	return cmd
}

func (r ReactionPicker) View() string {
	var b strings.Builder
	
	// Search bar
	b.WriteString("  🔍 " + r.input.View() + "\n\n")

	maxVisible := 12
	start := 0
	if r.cursor >= maxVisible {
		start = r.cursor - maxVisible + 1
	}

	myStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

	for i := start; i < len(r.filtered) && i < start+maxVisible; i++ {
		e := r.filtered[i]
		
		cursor := "  "
		itemStyle := lipgloss.NewStyle()
		if i == r.cursor {
			cursor = "> "
			itemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("170")).
				Bold(true)
		}
		
		emojiIcon := e.unicode
		if emojiIcon == "" {
			emojiIcon = "  " // Placeholder for missing icons
		}
		
		// Fixed alignment for emoji icon + name
		label := fmt.Sprintf("%s  :%s:", emojiIcon, e.name)
		if r.myReactions[e.name] {
			label += myStyle.Render(" ✓")
		}

		b.WriteString(cursor + itemStyle.Render(label) + "\n")
	}

	if len(r.filtered) == 0 {
		b.WriteString("\n  " + lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("No matching emoji"))
	}

	style := lipgloss.NewStyle().
		Width(44).
		Padding(1, 0).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("33")).
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("252"))

	return style.Render(b.String())
}

func defaultEmojis() []emojiEntry{
	return []emojiEntry{
		{"+1", "\U0001F44D"},
		{"-1", "\U0001F44E"},
		{"heart", "\u2764"},
		{"fire", "\U0001F525"},
		{"eyes", "\U0001F440"},
		{"tada", "\U0001F389"},
		{"rocket", "\U0001F680"},
		{"100", "\U0001F4AF"},
		{"check", "\u2705"},
		{"white_check_mark", "\u2705"},
		{"raised_hands", "\U0001F64C"},
		{"clap", "\U0001F44F"},
		{"pray", "\U0001F64F"},
		{"party_parrot", ""},
		{"thinking_face", "\U0001F914"},
		{"smile", "\U0001F604"},
		{"sweat_smile", "\U0001F605"},
		{"joy", "\U0001F602"},
		{"thumbsup", "\U0001F44D"},
		{"thumbsdown", "\U0001F44E"},
		{"wave", "\U0001F44B"},
		{"x", "\u274C"},
		{"warning", "\u26A0"},
		{"bulb", "\U0001F4A1"},
		{"star", "\u2B50"},
		{"sparkles", "\u2728"},
		{"zap", "\u26A1"},
		{"muscle", "\U0001F4AA"},
		{"ok_hand", "\U0001F44C"},
		{"sunglasses", "\U0001F60E"},
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
