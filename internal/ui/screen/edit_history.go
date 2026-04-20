package screen

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
)

// renderMessageWithHistory returns the focused message formatted for the pager.
// When the message has no edit history, it just returns the formatted current text.
// Otherwise it renders each prior version above the current one, separated by rules.
func renderMessageWithHistory(formatter *slack.Formatter, msg *slack.Message, width int) string {
	current := formatter.Format(msg.Text)
	if len(msg.EditHistory) == 0 {
		return current
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Underline(true)
	tsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ruleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("237"))

	ruleWidth := width - 4
	if ruleWidth < 1 {
		ruleWidth = 1
	}
	rule := ruleStyle.Render(strings.Repeat("─", ruleWidth))

	var b strings.Builder
	b.WriteString(headerStyle.Render("Edit History:"))
	b.WriteString("\n\n")
	for i, edit := range msg.EditHistory {
		ts := formatter.FormatTimestamp(edit.Timestamp)
		b.WriteString(tsStyle.Render(fmt.Sprintf("[%s]", ts)))
		b.WriteString("\n")
		b.WriteString(formatter.Format(edit.Text))
		b.WriteString("\n\n")
		if i < len(msg.EditHistory)-1 {
			b.WriteString(rule)
			b.WriteString("\n\n")
		}
	}
	b.WriteString(headerStyle.Render("Current Version:"))
	b.WriteString("\n\n")
	b.WriteString(current)
	return b.String()
}
