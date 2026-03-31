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
	viewport     viewport.Model
	messages     []slack.Message
	focusedIndex int
	width        int
	height       int
	formatter    *slack.Formatter
}

func NewMessageList(formatter *slack.Formatter, width, height int) MessageList {
	vp := viewport.New(
		viewport.WithWidth(width),
		viewport.WithHeight(height),
	)
	vp.MouseWheelEnabled = true
	return MessageList{
		viewport:  vp,
		formatter: formatter,
		width:     width,
		height:    height,
	}
}

func (m *MessageList) SetMessages(msgs []slack.Message) {
	m.messages = msgs
	if m.focusedIndex >= len(msgs) {
		m.focusedIndex = len(msgs) - 1
	}
	if m.focusedIndex < 0 {
		m.focusedIndex = 0
	}
	m.render()
}

func (m *MessageList) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.SetWidth(w)
	m.viewport.SetHeight(h)
	m.render()
}

func (m *MessageList) FocusedMessage() *slack.Message {
	if len(m.messages) == 0 {
		return nil
	}
	return &m.messages[m.focusedIndex]
}

func (m *MessageList) FocusedIndex() int {
	return m.focusedIndex
}

func (m *MessageList) MoveUp() {
	if m.focusedIndex > 0 {
		m.focusedIndex--
		m.render()
		m.ensureVisible()
	}
}

func (m *MessageList) MoveDown() {
	if m.focusedIndex < len(m.messages)-1 {
		m.focusedIndex++
		m.render()
		m.ensureVisible()
	}
}

func (m *MessageList) GoToTop() {
	m.focusedIndex = 0
	m.render()
	m.viewport.GotoTop()
}

func (m *MessageList) GoToBottom() {
	if len(m.messages) > 0 {
		m.focusedIndex = len(m.messages) - 1
	}
	m.render()
	m.viewport.GotoBottom()
}

func (m *MessageList) ScrollToBottom() {
	if len(m.messages) > 0 {
		m.focusedIndex = len(m.messages) - 1
	}
	m.render()
	m.viewport.GotoBottom()
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
		isSelected := i == m.focusedIndex
		b.WriteString(m.formatMessage(&msg, isSelected))
		if i < len(m.messages)-1 {
			b.WriteString("\n")
		}
	}
	m.viewport.SetContent(b.String())
}

func (m *MessageList) formatMessage(msg *slack.Message, isSelected bool) string {
	contentWidth := m.width - 4

	// Header: username + timestamp
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(userColor(msg.UserID))
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	username := msg.Username
	if username == "" {
		username = msg.UserID
	}
	ts := m.formatter.FormatTimestamp(msg.Timestamp)

	header := nameStyle.Render(username)
	headerRight := timeStyle.Render(ts)
	gap := contentWidth - lipgloss.Width(header) - lipgloss.Width(headerRight)
	if gap < 1 {
		gap = 1
	}
	headerLine := header + strings.Repeat(" ", gap) + headerRight

	// Body
	body := m.formatter.Format(msg.Text)

	var lines []string
	lines = append(lines, headerLine)
	lines = append(lines, body)

	// Reactions
	if len(msg.Reactions) > 0 {
		var reactionParts []string
		for _, r := range msg.Reactions {
			emoji := m.formatter.FormatEmoji(r.Name)
			text := fmt.Sprintf("%s %d", emoji, r.Count)
			if r.HasMe {
				text = lipgloss.NewStyle().
					Background(lipgloss.Color("24")).
					Foreground(lipgloss.Color("252")).
					Padding(0, 1).
					Render(text)
			} else {
				text = lipgloss.NewStyle().
					Background(lipgloss.Color("237")).
					Foreground(lipgloss.Color("252")).
					Padding(0, 1).
					Render(text)
			}
			reactionParts = append(reactionParts, text)
		}
		lines = append(lines, strings.Join(reactionParts, " "))
	}

	// Thread indicator
	if msg.ReplyCount > 0 && msg.ThreadTS == "" {
		threadStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Italic(true)
		lines = append(lines, threadStyle.Render(fmt.Sprintf("  ↳ %d replies", msg.ReplyCount)))
	}

	content := strings.Join(lines, "\n")

	if isSelected {
		return lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("33")).
			PaddingLeft(1).
			Render(content)
	}
	return lipgloss.NewStyle().PaddingLeft(3).Render(content)
}

func (m *MessageList) ensureVisible() {
	if len(m.messages) == 0 {
		return
	}

	focusStartLine := 0
	for i := 0; i < m.focusedIndex; i++ {
		msgStr := m.formatMessage(&m.messages[i], false)
		focusStartLine += lipgloss.Height(msgStr) + 1 // +1 for the newline between messages
	}

	focusMsgStr := m.formatMessage(&m.messages[m.focusedIndex], true)
	focusHeight := lipgloss.Height(focusMsgStr)
	focusEndLine := focusStartLine + focusHeight - 1 // -1 because start line is inclusive

	yOffset := m.viewport.YOffset()

	if focusStartLine < yOffset {
		// Scroll up so the start of the message is at the top of the viewport
		m.viewport.SetYOffset(focusStartLine)
	} else if focusEndLine >= yOffset+m.viewport.Height() {
		// Scroll down so the end of the message is at the bottom of the viewport
		m.viewport.SetYOffset(focusEndLine - m.viewport.Height() + 1)
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
