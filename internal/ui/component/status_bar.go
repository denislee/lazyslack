package component

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type StatusBar struct {
	width       int
	channel     string
	status      string
	err         string
	hints       []string
	unreadCount int
}

func NewStatusBar() StatusBar {
	return StatusBar{}
}

func (s *StatusBar) SetWidth(w int)           { s.width = w }
func (s *StatusBar) SetChannel(name string)   { s.channel = name }
func (s *StatusBar) SetStatus(text string)    { s.status = text }
func (s *StatusBar) SetError(text string)     { s.err = text }
func (s *StatusBar) SetHints(hints []string)  { s.hints = hints }
func (s *StatusBar) SetUnreadCount(n int)     { s.unreadCount = n }

func (s StatusBar) View() string {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252"))

	left := ""
	if s.channel != "" {
		left = lipgloss.NewStyle().Bold(true).Render(s.channel)
	}

	right := ""
	if s.err != "" {
		right = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(s.err)
	} else if s.status != "" {
		right = s.status
	} else if len(s.hints) > 0 {
		right = strings.Join(s.hints, " | ")
	}

	unread := ""
	if s.unreadCount > 0 {
		unread = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true).
			Render(fmt.Sprintf("%d unread", s.unreadCount))
	}

	if unread == "" {
		gap := max(s.width-lipgloss.Width(left)-lipgloss.Width(right)-2, 1)
		bar := fmt.Sprintf(" %s%s%s ", left, strings.Repeat(" ", gap), right)
		return style.Width(s.width).Render(bar)
	}

	totalContent := lipgloss.Width(left) + lipgloss.Width(unread) + lipgloss.Width(right) + 2
	gap := max(s.width-totalContent, 2)
	leftGap := gap / 2
	rightGap := gap - leftGap
	bar := fmt.Sprintf(" %s%s%s%s%s ", left, strings.Repeat(" ", leftGap), unread, strings.Repeat(" ", rightGap), right)
	return style.Width(s.width).Render(bar)
}
