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

	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Name
	}
	p.fields = append(p.fields, profileField{label: "Name", value: displayName})
	p.fields = append(p.fields, profileField{label: "Handle", value: "@" + user.Name})
	
	emailVal := user.Email
	if emailVal == "" {
		emailVal = "MISSING_FROM_API"
	}
	p.fields = append(p.fields, profileField{label: "EMAIL_DEBUG", value: emailVal})

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

	contentWidth := w - 4 // padding + border

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

	valueStyle := lipgloss.NewStyle().
		Width(contentWidth)

	selectedValueStyle := valueStyle.
		Bold(true).
		Foreground(lipgloss.Color("255"))

	for i, f := range p.fields {
		rows = append(rows, labelStyle.Render(f.label))
		if i == p.focusedIndex {
			indicator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("33")).
				Render("▸ ")
			rows = append(rows, indicator+selectedValueStyle.Render(truncate(f.value, contentWidth-2)))
		} else {
			rows = append(rows, "  "+valueStyle.Render(truncate(f.value, contentWidth-2)))
		}
	}

	content := strings.Join(rows, "\n")

	panel := lipgloss.NewStyle().
		Width(w).
		Height(p.height).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("33")).
		PaddingLeft(1).
		PaddingRight(1).
		PaddingTop(1).
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
