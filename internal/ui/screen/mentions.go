package screen

import (
	"image/color"
	"log/slog"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
)

type MentionsScreen struct {
	results   []slack.SearchResult
	cursor    int
	client    *slack.Client
	formatter *slack.Formatter
	lastPoll  time.Time
	loading   bool
	err       string
	width     int
	height    int
}

func NewMentionsScreen(client *slack.Client, formatter *slack.Formatter) *MentionsScreen {
	return &MentionsScreen{
		client:    client,
		formatter: formatter,
		loading:   true,
	}
}

type mentionsDataMsg struct {
	results []slack.SearchResult
}

type mentionsErrorMsg struct {
	err error
}

func (s *MentionsScreen) Init() tea.Cmd {
	return func() tea.Msg {
		results, err := s.client.Search("to:me")
		if err != nil {
			return mentionsErrorMsg{err: err}
		}
		return mentionsDataMsg{results: results}
	}
}

func (s *MentionsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case mentionsDataMsg:
		slog.Info("mentions data received", "count", len(msg.results))
		s.results = deduplicateMentions(msg.results)
		s.loading = false
		if s.cursor >= len(s.results) {
			s.cursor = max(len(s.results)-1, 0)
		}
		return s, nil

	case mentionsErrorMsg:
		slog.Error("mentions load error", "error", msg.err)
		s.err = msg.err.Error()
		s.loading = false
		return s, nil

	case tea.KeyPressMsg:
		switch {
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

		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			s.cursor = 0
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			if len(s.results) > 0 {
				s.cursor = len(s.results) - 1
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f", "pgdown"))):
			page := max((s.height-2)/3, 1)
			s.cursor = min(s.cursor+page, len(s.results)-1)
			if s.cursor < 0 {
				s.cursor = 0
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b", "pgup"))):
			page := max((s.height-2)/3, 1)
			s.cursor = max(s.cursor-page, 0)
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", "l"))):
			if r := s.SelectedResult(); r != nil {
				return s, func() tea.Msg {
					return OpenChannelMsg{Channel: slack.Channel{
						ID:   r.ChannelID,
						Name: r.ChannelName,
					}}
				}
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			s.loading = true
			s.results = nil
			return s, s.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			return s, tea.Quit
		}
	}

	return s, nil
}

func (s *MentionsScreen) View() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")).Render("Mentions")
	b.WriteString(title + "\n")

	if s.loading {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(1, 1).
			Render("Loading..."))
		return b.String()
	}

	if s.err != "" {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Padding(0, 1).
			Render(s.err))
		return b.String()
	}

	if len(s.results) == 0 {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(1, 1).
			Render("No mentions"))
		return b.String()
	}

	itemHeight := 2
	availHeight := s.height - 1 // title
	visible := max(availHeight/itemHeight, 1)

	start := 0
	if s.cursor >= visible {
		start = s.cursor - visible + 1
	}
	end := min(start+visible, len(s.results))

	contentWidth := s.width - 4

	for i := start; i < end; i++ {
		r := s.results[i]
		isSelected := i == s.cursor

		age := s.formatter.FormatTimestampAge(r.Message.Timestamp)
		prefix := "#"
		if r.IsIM {
			prefix = "@"
		}
		channel := prefix + r.ChannelName

		// Channel + age on one line
		chanStyle := lipgloss.NewStyle().Foreground(mentionColor(r.Message.UserID))
		ageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		ageStr := ageStyle.Render(age)
		header := chanStyle.Render(truncate(channel, contentWidth-len(age)-1)) + " " + ageStr

		// Message preview on second line
		preview := s.formatter.Format(r.Message.Text)
		preview = strings.ReplaceAll(preview, "\n", " ")
		preview = truncate(preview, contentWidth)
		previewStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		content := header + "\n" + previewStyle.Render(preview)

		if isSelected {
			b.WriteString(lipgloss.NewStyle().
				BorderLeft(true).
				BorderStyle(lipgloss.ThickBorder()).
				BorderForeground(lipgloss.Color("33")).
				PaddingLeft(1).
				Render(content) + "\n")
		} else {
			b.WriteString(lipgloss.NewStyle().PaddingLeft(3).Render(content) + "\n")
		}
	}

	return b.String()
}

func (s *MentionsScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *MentionsScreen) SetMentions(results []slack.SearchResult) {
	s.results = deduplicateMentions(results)
	s.loading = false
	if s.cursor >= len(s.results) {
		s.cursor = max(len(s.results)-1, 0)
	}
}

func (s *MentionsScreen) SetLastPoll(t time.Time) {
	s.lastPoll = t
}

func (s *MentionsScreen) SetPollError(err error) {
	s.err = "poll: " + err.Error()
}

func (s *MentionsScreen) SelectedResult() *slack.SearchResult {
	if len(s.results) > 0 && s.cursor < len(s.results) {
		return &s.results[s.cursor]
	}
	return nil
}

func (s *MentionsScreen) SelectedChannelID() string {
	if r := s.SelectedResult(); r != nil {
		return r.ChannelID
	}
	return ""
}

func (s *MentionsScreen) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter", "l"), key.WithHelp("enter/l", "open")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}

func deduplicateMentions(results []slack.SearchResult) []slack.SearchResult {
	type mentionKey struct {
		channelID string
		userID    string
	}
	seen := make(map[mentionKey]bool, len(results))
	out := make([]slack.SearchResult, 0, len(results))
	for _, r := range results {
		k := mentionKey{channelID: r.ChannelID, userID: r.Message.UserID}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

func mentionColor(id string) color.Color {
	colors := []string{"33", "170", "42", "214", "196", "99", "220", "75"}
	hash := 0
	for _, c := range id {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return lipgloss.Color(colors[hash%len(colors)])
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}
