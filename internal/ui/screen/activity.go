package screen

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
)

type ActivityScreen struct {
	results      []slack.SearchResult
	cursor       int
	scrollOffset int
	statusBar    component.StatusBar
	client       *slack.Client
	formatter    *slack.Formatter
	loading      bool
	err          string
	width        int
	height       int
}

func NewActivityScreen(client *slack.Client, formatter *slack.Formatter) *ActivityScreen {
	return &ActivityScreen{
		client:    client,
		formatter: formatter,
		statusBar: component.NewStatusBar(),
		loading:   true,
	}
}

type activityResultsMsg struct {
	results []slack.SearchResult
}

type activityErrorMsg struct {
	err error
}

func (s *ActivityScreen) Init() tea.Cmd {
	return func() tea.Msg {
		results, err := s.client.Search("to:me")
		if err != nil {
			return activityErrorMsg{err: err}
		}
		return activityResultsMsg{results: results}
	}
}

func (s *ActivityScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case activityResultsMsg:
		s.results = msg.results
		s.loading = false
		s.cursor = 0
		return s, nil

	case activityErrorMsg:
		s.err = msg.err.Error()
		s.loading = false
		return s, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"))):
			return s, func() tea.Msg { return GoBackMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(s.results) > 0 && s.cursor < len(s.results) {
				r := s.results[s.cursor]
				return s, func() tea.Msg {
					return JumpToChannelMsg{
						ChannelID:   r.ChannelID,
						ChannelName: r.ChannelName,
						MessageTS:   r.Message.Timestamp,
					}
				}
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down", "ctrl+n"))):
			if s.cursor < len(s.results)-1 {
				s.cursor++
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up", "ctrl+p"))):
			if s.cursor > 0 {
				s.cursor--
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f", "pgdown"))):
			page := (s.height - 3) / 4
			if page < 1 {
				page = 1
			}
			s.cursor += page
			if s.cursor >= len(s.results) {
				s.cursor = len(s.results) - 1
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b", "pgup"))):
			page := (s.height - 3) / 4
			if page < 1 {
				page = 1
			}
			s.cursor -= page
			if s.cursor < 0 {
				s.cursor = 0
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			s.cursor = 0
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			if len(s.results) > 0 {
				s.cursor = len(s.results) - 1
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			s.loading = true
			s.results = nil
			return s, s.Init()
		}
	}

	return s, nil
}

func (s *ActivityScreen) View() string {
	var b strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("33")).
		Padding(0, 1).
		Render("Activity — Mentions")

	headerBar := lipgloss.NewStyle().
		Width(s.width).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Render(header)

	b.WriteString(headerBar + "\n")

	if s.loading {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(1, 2).
			Render("Loading activity..."))
	} else if s.err != "" {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Padding(1, 2).
			Render("Error: " + s.err))
	} else if len(s.results) == 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(1, 2).
			Render("No mentions found"))
	} else {
		itemHeight := 3 // header + text + separator
		availHeight := s.height - 2 // header bar (2 lines with border)
		visible := (availHeight + 1) / itemHeight // last item doesn't need separator
		if visible < 1 {
			visible = 1
		}

		// Adjust scroll offset to keep cursor visible
		if s.cursor < s.scrollOffset {
			s.scrollOffset = s.cursor
		}
		if s.cursor >= s.scrollOffset+visible {
			s.scrollOffset = s.cursor - visible + 1
		}
		if s.scrollOffset < 0 {
			s.scrollOffset = 0
		}

		start := s.scrollOffset
		end := start + visible
		if end > len(s.results) {
			end = len(s.results)
		}

		contentWidth := s.width - 6

		for i := start; i < end; i++ {
			r := s.results[i]
			isSelected := i == s.cursor

			channel := lipgloss.NewStyle().
				Foreground(lipgloss.Color("33")).
				Render("#" + r.ChannelName)

			username := r.Message.Username
			if username == "" {
				username = r.Message.UserID
			}

			ts := s.formatter.FormatTimestamp(r.Message.Timestamp)

			text := s.formatter.Format(r.Message.Text)
			text = slack.StripANSI(text)
			text = strings.ReplaceAll(text, "\n", " ")
			if len(text) > contentWidth {
				text = text[:contentWidth] + "…"
			}

			line := fmt.Sprintf("  %s  %s  %s\n  %s",
				channel, username, ts, text)

			if isSelected {
				line = lipgloss.NewStyle().
					BorderLeft(true).
					BorderStyle(lipgloss.ThickBorder()).
					BorderForeground(lipgloss.Color("33")).
					PaddingLeft(1).
					Render(line)
			} else {
				line = lipgloss.NewStyle().PaddingLeft(3).Render(line)
			}

			b.WriteString(line + "\n")
			if i < end-1 {
				b.WriteString("\n") // separator between items
			}
		}
	}

	return b.String()
}

func (s *ActivityScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *ActivityScreen) SetStatusBarWidth(w int) { s.statusBar.SetWidth(w) }

func (s *ActivityScreen) SelectedResult() *slack.SearchResult {
	if len(s.results) > 0 && s.cursor < len(s.results) {
		return &s.results[s.cursor]
	}
	return nil
}

func (s *ActivityScreen) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "navigate")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("escape"), key.WithHelp("esc", "back")),
	}
}

func (s *ActivityScreen) InInsertMode() bool { return false }

func (s *ActivityScreen) StatusBarView() string {
	s.statusBar.SetChannel("Activity")
	if len(s.results) > 0 {
		s.statusBar.SetStatus(fmt.Sprintf("%d/%d", s.cursor+1, len(s.results)))
	} else {
		s.statusBar.SetStatus("")
	}
	return s.statusBar.View()
}
