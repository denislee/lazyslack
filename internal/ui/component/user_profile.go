package component

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
)

type profileField struct {
	label string
	value string
}

type UserProfilePanel struct {
	user         *slack.User
	fields       []profileField
	focusedIndex int
	width        int
	height       int
}

func NewUserProfilePanel() UserProfilePanel {
	return UserProfilePanel{
		focusedIndex: 0,
		fields:       []profileField{{label: "Email", value: "Loading..."}},
	}
}

func (p *UserProfilePanel) SetUser(user *slack.User) {
	p.user = user
	p.fields = p.fields[:0]
	p.focusedIndex = 0
	if user == nil {
		return
	}

	name := user.RealName
	if name == "" {
		name = user.DisplayName
	}
	if name == "" {
		name = user.Name
	}
	p.fields = append(p.fields, profileField{label: "Name", value: name})
	p.fields = append(p.fields, profileField{label: "Handle", value: "@" + user.Name})

	if user.Title != "" {
		p.fields = append(p.fields, profileField{label: "Title", value: user.Title})
	}
	if user.StatusText != "" || user.StatusEmoji != "" {
		p.fields = append(p.fields, profileField{label: "Status", value: strings.TrimSpace(user.StatusEmoji + " " + user.StatusText)})
	}
	
	emailVal := user.Email
	if emailVal == "" {
		emailVal = "Not provided"
	}
	p.fields = append(p.fields, profileField{label: "Email", value: emailVal})

	if user.Phone != "" {
		p.fields = append(p.fields, profileField{label: "Phone", value: user.Phone})
	}
	if user.Timezone != "" {
		p.fields = append(p.fields, profileField{label: "Timezone", value: user.Timezone})
	}
	p.fields = append(p.fields, profileField{label: "Member ID", value: user.ID})
}

func (p *UserProfilePanel) SetSize(w, h int) { p.width = w; p.height = h }

func (p *UserProfilePanel) MoveDown() {
	if len(p.fields) > 0 && p.focusedIndex < len(p.fields)-1 {
		p.focusedIndex++
	}
}

func (p *UserProfilePanel) MoveUp() {
	if p.focusedIndex > 0 {
		p.focusedIndex--
	}
}

func (p *UserProfilePanel) FocusedValue() string {
	if len(p.fields) == 0 {
		return ""
	}
	return p.fields[p.focusedIndex].value
}

func (p UserProfilePanel) View() string {
	if p.user == nil || len(p.fields) == 0 {
		return ""
	}

	w := p.width
	if w < 4 {
		return ""
	}

	contentWidth := w - 3 // border-left + 1 padding left + 1 padding right

	// Presence indicator header
	presenceIcon := "○"
	presenceColor := lipgloss.Color("240")
	if p.user.Presence == "active" {
		presenceIcon = "●"
		presenceColor = lipgloss.Color("42")
	}
	presenceStr := lipgloss.NewStyle().Foreground(presenceColor).Render(presenceIcon)
	if p.user.IsBot {
		presenceStr += " bot"
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("33")).
		Width(contentWidth).
		Render("Profile " + presenceStr)

	var rows []string
	rows = append(rows, header, "")

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(contentWidth)

	valueWidth := contentWidth - 2
	valueStyle := lipgloss.NewStyle().
		Width(valueWidth)

	selectedValueStyle := valueStyle.
		Bold(true).
		Foreground(lipgloss.Color("255"))

	for i, f := range p.fields {
		if i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, labelStyle.Render(f.label))
		if i == p.focusedIndex {
			indicator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("33")).
				Render("▸ ")
			rows = append(rows, indicator+selectedValueStyle.Render(truncate(f.value, valueWidth)))
		} else {
			rows = append(rows, "  "+valueStyle.Render(truncate(f.value, valueWidth)))
		}
	}

	content := strings.Join(rows, "\n")

	panel := lipgloss.NewStyle().
		Width(w - 1).
		Height(p.height).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("33")).
		PaddingLeft(1).
		PaddingRight(1).
		Render(content)

	return panel
}

func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	for i := range s {
		if lipgloss.Width(s[:i]) >= maxWidth-1 {
			return fmt.Sprintf("%s…", s[:i])
		}
	}
	return s
}
