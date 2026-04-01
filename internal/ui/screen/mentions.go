package screen

import (
	"fmt"
	"image/color"
	"log/slog"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
)

type MentionsScreen struct {
	results        []slack.SearchResult
	pinnedChannels []slack.Channel
	readTimestamps map[string]string
	profilePanel   component.UserProfilePanel
	cursor         int
	client         *slack.Client
	formatter      *slack.Formatter
	lastPoll       time.Time
	loading        bool
	err            string
	width          int
	height         int
}

func NewMentionsScreen(client *slack.Client, formatter *slack.Formatter) *MentionsScreen {
	return &MentionsScreen{
		client:       client,
		formatter:    formatter,
		loading:      true,
		profilePanel: component.NewUserProfilePanel(),
	}
}

type mentionsDataMsg struct {
	results []slack.SearchResult
}

type mentionsErrorMsg struct {
	err error
}

func (s *MentionsScreen) ChannelID() string { return "" }
func (s *MentionsScreen) InInsertMode() bool {
	return false
}

func (s *MentionsScreen) Init() tea.Cmd {
	var cmds []tea.Cmd

	// Fetch mentions
	cmds = append(cmds, func() tea.Msg {
		results, err := s.client.Search("to:me")
		if err != nil {
			return mentionsErrorMsg{err: err}
		}
		return mentionsDataMsg{results: results}
	})

	return tea.Batch(cmds...)
}

func (s *MentionsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case mentionsDataMsg:
		slog.Info("mentions data received", "count", len(msg.results))
		s.SetMentions(msg.results)
		return s, nil

	case MentionsRefreshMsg: // From App poll
		s.SetMentions(msg.Results)
		return s, nil

	case mentionsErrorMsg:
		slog.Error("mentions load error", "error", msg.err)
		s.err = msg.err.Error()
		s.loading = false
		return s, nil

	case tea.KeyPressMsg:
		total := len(s.pinnedChannels) + len(s.results)
		if total == 0 {
			return s, nil
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down", "ctrl+n"))):
			if s.cursor < total-1 {
				s.cursor++
				s.updateProfilePanel()
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up", "ctrl+p"))):
			if s.cursor > 0 {
				s.cursor--
				s.updateProfilePanel()
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			s.cursor = 0
			s.updateProfilePanel()
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			s.cursor = total - 1
			s.updateProfilePanel()
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f", "pgdown"))):
			page := max((s.height-2)/3, 1)
			s.cursor = min(s.cursor+page, total-1)
			s.updateProfilePanel()
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b", "pgup"))):
			page := max((s.height-2)/3, 1)
			s.cursor = max(s.cursor-page, 0)
			s.updateProfilePanel()
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+f"))):
			id := s.SelectedChannelID()
			if id != "" {
				return s, func() tea.Msg {
					return component.ToggleFavoriteMsg{ChannelID: id}
				}
			}
			return s, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", "l"))):
			if s.cursor < len(s.pinnedChannels) {
				ch := s.pinnedChannels[s.cursor]
				return s, func() tea.Msg {
					return OpenChannelMsg{Channel: ch}
				}
			}

			idx := s.cursor - len(s.pinnedChannels)
			if idx < len(s.results) {
				r := s.results[idx]
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

func (s *MentionsScreen) updateProfilePanel() {
	var userID string
	if s.cursor < len(s.pinnedChannels) {
		ch := s.pinnedChannels[s.cursor]
		if ch.IsIM {
			// Extract user ID from IM channel ID? Slack DM channel IDs start with D.
			// We'd need to store the target UserID in the Channel struct.
		}
	} else {
		idx := s.cursor - len(s.pinnedChannels)
		if idx < len(s.results) {
			userID = s.results[idx].Message.UserID
		}
	}

	if userID != "" {
		user, err := s.client.ResolveUser(userID)
		if err == nil {
			s.profilePanel.SetUser(user)
		}
	} else {
		s.profilePanel.SetUser(nil)
	}
}

func (s *MentionsScreen) clampCursor() {
	total := len(s.pinnedChannels) + len(s.results)
	if s.cursor >= total {
		s.cursor = max(total-1, 0)
	}
}

func (s *MentionsScreen) View() string {
	content := s.renderMentions()

	if s.width > 40 {
		// Split view if wide enough
		profileW := 30
		mentionsW := s.width - profileW

		mentionsStyle := lipgloss.NewStyle().Width(mentionsW)
		s.profilePanel.SetSize(profileW, s.height)

		return lipgloss.JoinHorizontal(lipgloss.Top,
			mentionsStyle.Render(content),
			s.profilePanel.View(),
		)
	}

	return content
}

func (s *MentionsScreen) renderMentions() string {
	var b strings.Builder

	// 1. Pinned Channels
	if len(s.pinnedChannels) > 0 {
		title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")).Render("Favorites")
		b.WriteString(title + "\n")
		for i, ch := range s.pinnedChannels {
			isSelected := i == s.cursor
			prefix := "#"
			if ch.IsIM {
				prefix = "@"
			}

			badge := ""
			localTS := ""
			if s.readTimestamps != nil {
				localTS = s.readTimestamps[ch.ID]
			}
			isUnread := ch.LatestTS != "" && (localTS == "" || ch.LatestTS > localTS)

			if isUnread && ch.UnreadCount > 0 {
				badge = fmt.Sprintf(" %d", ch.UnreadCount)
			}

			// Truncate name to fit
			nameWidth := s.width - 4 - lipgloss.Width(badge)
			name := truncate(ch.Name, nameWidth)

			content := prefix + name + badge
			style := lipgloss.NewStyle()
			if isUnread {
				style = style.Foreground(lipgloss.Color("255")).Bold(true) // White for unread
			} else {
				style = style.Foreground(lipgloss.Color("244")) // Gray for read
			}

			if isSelected {
				b.WriteString(lipgloss.NewStyle().
					Foreground(lipgloss.Color("170")).
					Bold(true).
					Render("> "+content) + "\n")
			} else {
				b.WriteString("  " + style.Render(content) + "\n")
			}
		}
		b.WriteString("\n")
	}

	// 2. Mentions
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")).Render("Mentions")
	b.WriteString(title + "\n")

	if s.loading && len(s.results) == 0 {
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

	// Calculate how much space is left for mentions
	pinnedHeight := 0
	if len(s.pinnedChannels) > 0 {
		pinnedHeight = len(s.pinnedChannels) + 2 // title + blank line
	}
	availHeight := s.height - pinnedHeight - 1 // -1 for mentions title

	itemHeight := 2
	visible := max(availHeight/itemHeight, 1)

	mentionStartCursor := len(s.pinnedChannels)

	relativeMentionCursor := s.cursor - mentionStartCursor
	if relativeMentionCursor < 0 {
		relativeMentionCursor = 0
	}

	start := 0
	if relativeMentionCursor >= visible {
		start = relativeMentionCursor - visible + 1
	}
	end := min(start+visible, len(s.results))

	contentWidth := s.width - 4

	for i := start; i < end; i++ {
		r := s.results[i]
		isSelected := (i + mentionStartCursor) == s.cursor

		age := s.formatter.FormatTimestampAge(r.Message.Timestamp)
		prefix := "#"
		if r.IsIM {
			prefix = "@"
		}
		channel := prefix + r.ChannelName

		// Channel + age on one line
		chanStyle := lipgloss.NewStyle().Foreground(mentionColor(r.Message.UserID))
		isUnread := false
		if s.readTimestamps != nil {
			localTS := s.readTimestamps[r.ChannelID]
			if localTS == "" || r.Message.Timestamp > localTS {
				isUnread = true
			}
		}

		if isUnread {
			chanStyle = chanStyle.Bold(true)
		}

		ageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		ageStr := ageStyle.Render(age)
		header := chanStyle.Render(truncate(channel, contentWidth-len(age)-1)) + " " + ageStr

		// Message preview on second line
		preview := s.formatter.Format(r.Message.Text)
		preview = strings.ReplaceAll(preview, "\n", " ")
		preview = truncate(preview, contentWidth)
		previewStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		if isUnread {
			previewStyle = previewStyle.Bold(true).Foreground(lipgloss.Color("255"))
		}
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

func (s *MentionsScreen) Results() []slack.SearchResult {
	return s.results
}

func (s *MentionsScreen) SetReadTimestamps(readTimestamps map[string]string) {
	s.readTimestamps = readTimestamps
}

func (s *MentionsScreen) SetMentions(results []slack.SearchResult) {
	s.results = deduplicateMentions(results)
	// Sort mentions alphabetically by channel name
	sort.Slice(s.results, func(i, j int) bool {
		return strings.ToLower(s.results[i].ChannelName) < strings.ToLower(s.results[j].ChannelName)
	})
	s.loading = false
	s.clampCursor()
	s.updateProfilePanel()
}

func (s *MentionsScreen) SetPinnedChannels(channels []slack.Channel, pinnedIDs []string, readTimestamps map[string]string) {
	s.readTimestamps = readTimestamps
	pinnedMap := make(map[string]bool)
	for _, id := range pinnedIDs {
		pinnedMap[id] = true
	}

	filtered := make([]slack.Channel, 0)
	for _, ch := range channels {
		if pinnedMap[ch.ID] {
			filtered = append(filtered, ch)
		}
	}

	// Sort favorites alphabetically by name
	sort.Slice(filtered, func(i, j int) bool {
		return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
	})

	s.pinnedChannels = filtered
	s.clampCursor()
	s.updateProfilePanel()
}

func (s *MentionsScreen) SetLastPoll(t time.Time) {
	s.lastPoll = t
}

func (s *MentionsScreen) SetPollError(err error) {
	s.err = "poll: " + err.Error()
}

func (s *MentionsScreen) SelectedResult() *slack.SearchResult {
	idx := s.cursor - len(s.pinnedChannels)
	if idx >= 0 && idx < len(s.results) {
		return &s.results[idx]
	}
	return nil
}

func (s *MentionsScreen) SelectedChannel() *slack.Channel {
	if s.cursor >= 0 && s.cursor < len(s.pinnedChannels) {
		return &s.pinnedChannels[s.cursor]
	}
	return nil
}

func (s *MentionsScreen) SelectedChannelID() string {
	if ch := s.SelectedChannel(); ch != nil {
		return ch.ID
	}
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

	// Always sort results alphabetically
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].ChannelName) < strings.ToLower(out[j].ChannelName)
	})

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
