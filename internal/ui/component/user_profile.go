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
	focusedIndex int // 0 = avatar (when present), 1+ = fields
	scrollOffset int
	width        int
	height       int
	avatarStr    string
}

func NewUserProfilePanel() UserProfilePanel {
	return UserProfilePanel{
		focusedIndex: 0,
		fields:       []profileField{{label: "Email", value: "Loading..."}},
	}
}

func (p *UserProfilePanel) SetAvatar(s string) {
	p.avatarStr = s
}

func (p *UserProfilePanel) hasAvatar() bool {
	return p.avatarStr != ""
}

func (p *UserProfilePanel) totalItems() int {
	n := len(p.fields)
	if p.hasAvatar() {
		n++
	}
	return n
}

func (p *UserProfilePanel) IsAvatarFocused() bool {
	return p.hasAvatar() && p.focusedIndex == 0
}

// fieldIndex returns the index into p.fields for the current focusedIndex.
// Returns -1 if avatar is focused.
func (p *UserProfilePanel) fieldIndex() int {
	if p.hasAvatar() {
		return p.focusedIndex - 1
	}
	return p.focusedIndex
}

func (p *UserProfilePanel) AvatarURL() string {
	if p.user != nil {
		return p.user.ImageURL
	}
	return ""
}

func (p *UserProfilePanel) SetUser(user *slack.User) {
	p.user = user
	p.fields = p.fields[:0]
	p.avatarStr = ""
	p.focusedIndex = 0
	p.scrollOffset = 0
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
	total := p.totalItems()
	if total > 0 && p.focusedIndex < total-1 {
		p.focusedIndex++
	}
}

func (p *UserProfilePanel) UserID() string {
	if p.user == nil {
		return ""
	}
	return p.user.ID
}

func (p *UserProfilePanel) MoveUp() {
	if p.focusedIndex > 0 {
		p.focusedIndex--
	}
}

func (p *UserProfilePanel) FocusedValue() string {
	if p.IsAvatarFocused() {
		return p.AvatarURL()
	}
	fi := p.fieldIndex()
	if fi >= 0 && fi < len(p.fields) {
		return p.fields[fi].value
	}
	return ""
}

func (p *UserProfilePanel) View() string {
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
	lineCount := 0
	focusedStartLine := 0 // first line of the focused item
	focusedEndLine := 0   // last line of the focused item (exclusive)

	rows = append(rows, header, "")
	lineCount += 2

	if p.hasAvatar() {
		avatarLines := strings.Count(p.avatarStr, "\n") + 1
		if p.IsAvatarFocused() {
			focusedStartLine = lineCount
			// Render avatar with selection indicator
			indicator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("33")).
				Render("▸ Avatar")
			rows = append(rows, indicator)
			lineCount++
			rows = append(rows, p.avatarStr)
			lineCount += avatarLines
			focusedEndLine = lineCount
		} else {
			rows = append(rows, p.avatarStr)
			lineCount += avatarLines
		}
		rows = append(rows, "")
		lineCount++
	}

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(contentWidth)

	valueWidth := contentWidth - 2
	valueStyle := lipgloss.NewStyle().
		Width(valueWidth)

	selectedValueStyle := valueStyle.
		Bold(true).
		Foreground(lipgloss.Color("255"))

	fi := p.fieldIndex()
	for i, f := range p.fields {
		if i > 0 {
			rows = append(rows, "")
			lineCount++
		}
		if i == fi {
			focusedStartLine = lineCount
		}
		rows = append(rows, labelStyle.Render(f.label))
		lineCount++
		if i == fi {
			indicator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("33")).
				Render("▸ ")
			rows = append(rows, indicator+selectedValueStyle.Render(truncate(f.value, valueWidth)))
			focusedEndLine = lineCount + 1
		} else {
			rows = append(rows, "  "+valueStyle.Render(truncate(f.value, valueWidth)))
		}
		lineCount++
	}

	// Join all rows and split into individual lines for scrolling
	allContent := strings.Join(rows, "\n")
	lines := strings.Split(allContent, "\n")
	totalLines := len(lines)

	availHeight := p.height
	if availHeight <= 0 {
		availHeight = totalLines
	}

	if totalLines > availHeight {
		// Ensure focused item is fully visible
		if focusedStartLine < p.scrollOffset {
			p.scrollOffset = focusedStartLine
		}
		if focusedEndLine > p.scrollOffset+availHeight {
			p.scrollOffset = focusedEndLine - availHeight
		}

		maxOffset := totalLines - availHeight
		if p.scrollOffset > maxOffset {
			p.scrollOffset = maxOffset
		}
		if p.scrollOffset < 0 {
			p.scrollOffset = 0
		}

		end := p.scrollOffset + availHeight
		if end > totalLines {
			end = totalLines
		}
		lines = lines[p.scrollOffset:end]
	} else {
		p.scrollOffset = 0
	}

	content := strings.Join(lines, "\n")

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
