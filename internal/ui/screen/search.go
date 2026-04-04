package screen

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
)

type SearchScreen struct {
	input     textinput.Model
	results   []slack.SearchResult
	cursor    int
	client    *slack.Client
	formatter *slack.Formatter
	query     string
	loading   bool
	err       string
	width     int
	height    int
	lastQuery string
	debounce  time.Time
}

type JumpToChannelMsg struct {
	ChannelID   string
	ChannelName string
	MessageTS   string
}

func NewSearchScreen(client *slack.Client, formatter *slack.Formatter) *SearchScreen {
	ti := textinput.New()
	ti.Placeholder = "Search messages..."
	ti.Focus()
	ti.SetWidth(60)

	return &SearchScreen{
		input:     ti,
		client:    client,
		formatter: formatter,
	}
}

func (s *SearchScreen) InInsertMode() bool     { return true }
func (s *SearchScreen) StatusBarView() string  { return "" }
func (s *SearchScreen) SetStatusBarWidth(int) {}

func (s *SearchScreen) Init() tea.Cmd {
	return textinput.Blink
}

type searchResultsMsg struct {
	query   string
	results []slack.SearchResult
}

type searchErrorMsg struct {
	err error
}

func (s *SearchScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case searchResultsMsg:
		if msg.query == strings.TrimSpace(s.input.Value()) {
			s.results = msg.results
			s.loading = false
			s.cursor = 0
		}
		return s, nil

	case searchErrorMsg:
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
			// Trigger search on enter if no results yet
			query := strings.TrimSpace(s.input.Value())
			if len(query) >= 2 {
				s.lastQuery = query
				s.loading = true
				return s, func() tea.Msg {
					results, err := s.client.Search(query)
					if err != nil {
						return searchErrorMsg{err: err}
					}
					return searchResultsMsg{query: query, results: results}
				}
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+n", "down"))):
			if s.cursor < len(s.results)-1 {
				s.cursor++
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+p", "up"))):
			if s.cursor > 0 {
				s.cursor--
			}
			return s, nil
		}
	}

	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)

	// Debounced search
	query := strings.TrimSpace(s.input.Value())
	if query != s.lastQuery && len(query) >= 2 {
		s.lastQuery = query
		s.loading = true
		searchFn := func() tea.Msg {
			results, err := s.client.Search(query)
			if err != nil {
				return searchErrorMsg{err: err}
			}
			return searchResultsMsg{query: query, results: results}
		}
		return s, tea.Batch(cmd, searchFn)
	}

	return s, cmd
}

func (s *SearchScreen) View() string {
	var b strings.Builder

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("33")).
		Padding(0, 1).
		Render("Search")

	headerBar := lipgloss.NewStyle().
		Width(s.width).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Render(header)

	b.WriteString(headerBar + "\n")
	b.WriteString("  " + s.input.View() + "\n\n")

	if s.loading {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 2).
			Render("Searching..."))
	} else if s.err != "" {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Padding(0, 2).
			Render("Error: " + s.err))
	} else if len(s.results) == 0 && s.lastQuery != "" {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 2).
			Render("No results"))
	} else {
		itemHeight := 4
		availHeight := s.height - 5 // header + border + input + blank
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

		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 2).
			Render(fmt.Sprintf("%d/%d", s.cursor+1, len(s.results))))
	}

	return b.String()
}

func (s *SearchScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.input.SetWidth(w - 6)
}

func (s *SearchScreen) SelectedResult() *slack.SearchResult {
	if len(s.results) > 0 && s.cursor < len(s.results) {
		return &s.results[s.cursor]
	}
	return nil
}

func (s *SearchScreen) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter", "l"), key.WithHelp("enter/l", "go to")),
		key.NewBinding(key.WithKeys("ctrl+n/p"), key.WithHelp("ctrl+n/p", "navigate")),
		key.NewBinding(key.WithKeys("escape", "ctrl+["), key.WithHelp("esc", "back")),
	}
}
