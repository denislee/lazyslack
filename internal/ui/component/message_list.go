package component

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"

	"github.com/user/lazyslack/internal/slack"
)

type MessageList struct {
	viewport      viewport.Model
	messages      []slack.Message
	focusedIndex  int
	width         int
	height         int
	formatter     *slack.Formatter
	channelID     string
	readTimestamp string
}

func NewMessageList(formatter *slack.Formatter, width, height int) MessageList {
	vp := viewport.New(
		viewport.WithWidth(width),
		viewport.WithHeight(height),
	)
	vp.MouseWheelEnabled = true
	return MessageList{
		viewport:     vp,
		formatter:    formatter,
		width:        width,
		height:       height,
		focusedIndex: -1,
	}
}

func (m *MessageList) SetChannel(channelID string, readTimestamp string) {
	m.channelID = channelID
	m.readTimestamp = readTimestamp
	m.focusedIndex = -1
	m.render()
}

func (m *MessageList) SetReadTimestamp(ts string) {
	if ts > m.readTimestamp {
		m.readTimestamp = ts
		m.render()
	}
}

func (m *MessageList) SetMessages(msgs []slack.Message) {
	wasAtBottom := len(m.messages) == 0 || m.focusedIndex >= len(m.messages)-1
	m.messages = msgs
	
	// Keep focus within bounds if something was already focused
	if m.focusedIndex >= len(msgs) {
		m.focusedIndex = len(msgs) - 1
	}
	// If nothing was focused, keep it at -1 (user must explicitly select)

	m.render()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

func (m *MessageList) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.SetWidth(w)
	m.viewport.SetHeight(h)
	m.render()
}

func (m *MessageList) FocusedMessage() *slack.Message {
	if m.focusedIndex < 0 || m.focusedIndex >= len(m.messages) {
		return nil
	}
	return &m.messages[m.focusedIndex]
}

func (m *MessageList) FocusedIndex() int {
	return m.focusedIndex
}

func (m *MessageList) Messages() []slack.Message {
	return m.messages
}

// IsAtBottom reports whether the viewport's focus is at (or past) the last
// message, which is the passive-reading state where new incoming messages
// should count as already-read.
func (m *MessageList) IsAtBottom() bool {
	return len(m.messages) == 0 || m.focusedIndex >= len(m.messages)-1
}

func (m *MessageList) MoveUp() {
	if m.focusedIndex == -1 {
		if len(m.messages) > 0 {
			m.focusedIndex = len(m.messages) - 1
		} else {
			return
		}
	} else if m.focusedIndex > 0 {
		m.focusedIndex--
	}
	m.render()
	m.ensureVisible()
}

func (m *MessageList) MoveDown() {
	if m.focusedIndex == -1 {
		if len(m.messages) > 0 {
			m.focusedIndex = 0
		} else {
			return
		}
	} else if m.focusedIndex < len(m.messages)-1 {
		m.focusedIndex++
	}
	m.render()
	m.ensureVisible()
}

func (m *MessageList) GoToTop() {
	if len(m.messages) > 0 {
		m.focusedIndex = 0
		m.render()
		m.viewport.GotoTop()
	}
}

func (m *MessageList) GoToBottom() {
	if len(m.messages) > 0 {
		m.focusedIndex = len(m.messages) - 1
		m.render()
		m.viewport.GotoBottom()
	}
}

func (m *MessageList) PageUp() {
	if len(m.messages) == 0 {
		return
	}
	if m.focusedIndex == -1 {
		m.focusedIndex = len(m.messages) - 1
	}
	pageSize := m.height / 3
	if pageSize < 1 {
		pageSize = 1
	}
	m.focusedIndex -= pageSize
	if m.focusedIndex < 0 {
		m.focusedIndex = 0
	}
	m.render()
	m.ensureVisible()
}

func (m *MessageList) PageDown() {
	if len(m.messages) == 0 {
		return
	}
	if m.focusedIndex == -1 {
		m.focusedIndex = 0
	}
	pageSize := m.height / 3
	if pageSize < 1 {
		pageSize = 1
	}
	m.focusedIndex += pageSize
	if m.focusedIndex >= len(m.messages) {
		m.focusedIndex = len(m.messages) - 1
	}
	m.render()
	m.ensureVisible()
}

func (m *MessageList) ScrollToBottom() {
	if len(m.messages) > 0 {
		m.focusedIndex = len(m.messages) - 1
	}
	m.render()
	m.viewport.GotoBottom()
}

func (m *MessageList) FocusOnTimestamp(ts string) {
	for i, msg := range m.messages {
		if msg.Timestamp == ts {
			m.focusedIndex = i
			m.render()
			m.ensureVisible()
			return
		}
	}
	// Fallback: scroll to bottom if timestamp not found
	m.ScrollToBottom()
}

func (m *MessageList) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return cmd
}

func (m *MessageList) View() string {
	return m.viewport.View()
}

func (m *MessageList) render() {
	if len(m.messages) == 0 {
		m.viewport.SetContent(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("  No messages yet"))
		return
	}

	var b strings.Builder
	for i, msg := range m.messages {
		isSelected := m.focusedIndex != -1 && i == m.focusedIndex
		b.WriteString(m.formatMessage(&msg, isSelected))
		if i < len(m.messages)-1 {
			b.WriteString("\n\n")
		}
	}
	m.viewport.SetContent(b.String())
}

func (m *MessageList) formatMessage(msg *slack.Message, isSelected bool) string {
	contentWidth := m.width - 4

	isUnread := m.readTimestamp == "" || msg.Timestamp > m.readTimestamp

	// Define colors based on read/unread status
	nameColor := userColor(msg.UserID)
	timeColor := lipgloss.Color("240")
	lineColor := lipgloss.Color("237")
	bodyColor := lipgloss.Color("252") // Default font color

	if isUnread {
		// Unread: White font, bold name
		bodyColor = lipgloss.Color("255")
	} else {
		// Read: Light gray body, but keep original name color
		bodyColor = lipgloss.Color("245")
		// nameColor stays as userColor(msg.UserID)
		timeColor = lipgloss.Color("238")
		lineColor = lipgloss.Color("236")
	}

	nameStyle := lipgloss.NewStyle().Bold(isUnread).Foreground(nameColor)
	timeStyle := lipgloss.NewStyle().Foreground(timeColor)

	username := msg.Username
	if username == "" {
		username = msg.UserID
	}

	presenceIcon := ""
	if user := m.formatter.GetUser(msg.UserID); user != nil {
		if user.Presence == "active" {
			presenceIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("● ") // Green for online
		} else {
			presenceIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("○ ") // Grey for offline
		}
	}

	ts := m.formatter.FormatTimestamp(msg.Timestamp)
	if msg.Edited {
		ts += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" (edited)")
	}

	header := presenceIcon + nameStyle.Render(username)
	if msg.ReplyCount > 0 && (msg.ThreadTS == "" || msg.ThreadTS == msg.Timestamp) {
		header += lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Render(" 💬")
	}

	headerRight := timeStyle.Render(ts)
	lineStyle := lipgloss.NewStyle().Foreground(lineColor)
	gap := contentWidth - lipgloss.Width(header) - lipgloss.Width(headerRight) - 2
	if gap < 1 {
		gap = 1
	}
	headerLine := header + " " + lineStyle.Render(strings.Repeat("─", gap)) + " " + headerRight

	// Body (collapse double newlines, word-wrapped to fit content width)
	bodyText := m.formatter.Format(msg.Text)
	for strings.Contains(bodyText, "\n\n") {
		bodyText = strings.ReplaceAll(bodyText, "\n\n", "\n")
	}
	body := lipgloss.NewStyle().Width(contentWidth).Foreground(bodyColor).Render(bodyText)

	var lines []string
	lines = append(lines, headerLine)
	lines = append(lines, body)

	// Files/attachments
	if len(msg.Files) > 0 {
		for _, f := range msg.Files {
			tag := "[file]"
			if strings.HasPrefix(f.Mimetype, "image/") {
				tag = "[image]"
			}
			tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Italic(true)
			lines = append(lines, fmt.Sprintf("  %s %s", tagStyle.Render(tag), nameStyle.Render(f.Name)))
		}
	}

	// Reactions
	if len(msg.Reactions) > 0 {
		var reactionParts []string
		for _, r := range msg.Reactions {
			emoji := m.formatter.FormatEmoji(r.Name)
			text := fmt.Sprintf("%s %d", emoji, r.Count)
			
			rStyle := lipgloss.NewStyle().
				Background(lipgloss.Color("237")).
				Foreground(bodyColor). // Follow body text color
				Padding(0, 1)
			
			if r.HasMe {
				rStyle = rStyle.Background(lipgloss.Color("24")).Foreground(lipgloss.Color("255"))
			}
			
			reactionParts = append(reactionParts, rStyle.Render(text))
		}
		lines = append(lines, strings.Join(reactionParts, " "))
	}

	// Thread indicator
	if msg.ReplyCount > 0 && (msg.ThreadTS == "" || msg.ThreadTS == msg.Timestamp) {
		threadStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Italic(true).
			Bold(isUnread)
		lines = append(lines, threadStyle.Render(fmt.Sprintf("  ↳ %d replies (Press Enter/L to view)", msg.ReplyCount)))
	}

	content := strings.Join(lines, "\n")

	// Apply styles to the entire message block
	containerStyle := lipgloss.NewStyle().PaddingLeft(3)

	if isSelected {
		borderColor := lipgloss.Color("33")
		if !isUnread {
			borderColor = lipgloss.Color("24")
		}
		return containerStyle.
			PaddingLeft(1).
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(borderColor).
			Render(content)
	}
	return containerStyle.Render(content)
}

func (m *MessageList) ensureVisible() {
	if m.focusedIndex < 0 || m.focusedIndex >= len(m.messages) {
		return
	}

	// Count lines from the rendered content to get exact positions
	focusStartLine := 0
	for i := 0; i < m.focusedIndex; i++ {
		isSelected := i == m.focusedIndex
		msgStr := m.formatMessage(&m.messages[i], isSelected)
		focusStartLine += strings.Count(msgStr, "\n") + 1 // lines in msg
		focusStartLine += 1                                // blank line separator (\n\n) between messages
	}

	focusMsgStr := m.formatMessage(&m.messages[m.focusedIndex], true)
	focusHeight := strings.Count(focusMsgStr, "\n") + 1
	focusEndLine := focusStartLine + focusHeight - 1

	yOffset := m.viewport.YOffset()
	viewHeight := m.viewport.Height()

	if focusStartLine < yOffset {
		m.viewport.SetYOffset(focusStartLine)
	} else if focusEndLine >= yOffset+viewHeight {
		m.viewport.SetYOffset(focusEndLine - viewHeight + 1)
	}
}

func userColor(userID string) color.Color {
	colors := []string{"33", "170", "42", "214", "196", "99", "220", "75"}
	hash := 0
	for _, c := range userID {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return lipgloss.Color(colors[hash%len(colors)])
}
