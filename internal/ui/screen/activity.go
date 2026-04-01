package screen

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
)

type ActivityScreen struct {
	results   []slack.SearchResult
	cursor    int
	client    *slack.Client
	formatter *slack.Formatter
	loading   bool
	err       string
	width     int
	height    int
}

func NewActivityScreen(client *slack.Client, formatter *slack.Formatter) *ActivityScreen {
	return &ActivityScreen{
		client:    client,
		formatter: formatter,
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
		case key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+["))):
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
		itemHeight := 4 // each result takes ~4 lines
		availHeight := s.height - 3 // header + border
		visible := availHeight / itemHeight
		if visible < 1 {
			visible = 1
		}

		start := 0
		if s.cursor >= visible {
			start = s.cursor - visible + 1
		}
		end := start + visible
		if end > len(s.results) {
			end = len(s.results)
		}

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

			text := r.Message.Text
			if len(text) > s.width-10 {
				text = text[:s.width-10] + "..."
			}

			line := fmt.Sprintf("  %s  %s  %s\n  %s\n",
				channel, username, ts, s.formatter.Format(text))

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
		}

		// Position indicator
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 2).
			Render(fmt.Sprintf("%d/%d", s.cursor+1, len(s.results))))
	}

	return b.String()
}

func (s *ActivityScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
}

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
