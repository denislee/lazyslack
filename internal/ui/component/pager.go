package component

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
)

type PagerCloseMsg struct{}

type Pager struct {
	lines      []string
	cursor     int // current line (0-indexed)
	scroll     int // first visible line
	selectFrom int // -1 if no visual selection
	width      int
	height     int
	statusMsg  string
}

func NewPager(content string, width, height int) Pager {
	p := Pager{selectFrom: -1}
	p.open(content, width, height)
	return p
}

// wrapLine splits a long line at word boundaries to fit within width.
func wrapLine(s string, width int) []string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return []string{s}
	}
	if strings.TrimSpace(s) == "" {
		return []string{s}
	}

	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{s}
	}

	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current+" "+word) > width {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	lines = append(lines, current)
	return lines
}

func (p *Pager) open(content string, w, h int) {
	p.width = w
	p.height = h

	raw := strings.Split(content, "\n")

	// Estimate gutter width from raw line count
	gutter := len(fmt.Sprintf("%d", len(raw))) + 2
	contentW := w - gutter - 1
	if contentW < 1 {
		contentW = 1
	}

	var lines []string
	for _, line := range raw {
		lines = append(lines, wrapLine(line, contentW)...)
	}

	p.lines = lines
	p.cursor = 0
	p.scroll = 0
	p.selectFrom = -1
}

func (p *Pager) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p Pager) Update(msg tea.Msg) (Pager, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "escape", "h", "ctrl+["))):
			return p, func() tea.Msg { return PagerCloseMsg{} }

		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
			if p.cursor < len(p.lines)-1 {
				p.cursor++
				p.ensureVisible()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
			if p.cursor > 0 {
				p.cursor--
				p.ensureVisible()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			p.cursor = 0
			p.scroll = 0

		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			p.cursor = len(p.lines) - 1
			p.ensureVisible()

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d"))):
			half := p.viewHeight() / 2
			p.cursor += half
			if p.cursor >= len(p.lines) {
				p.cursor = len(p.lines) - 1
			}
			p.ensureVisible()

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+f", "pgdown"))):
			full := p.viewHeight()
			p.cursor += full
			if p.cursor >= len(p.lines) {
				p.cursor = len(p.lines) - 1
			}
			p.ensureVisible()

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u"))):
			half := p.viewHeight() / 2
			p.cursor -= half
			if p.cursor < 0 {
				p.cursor = 0
			}
			p.ensureVisible()

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+b", "pgup"))):
			full := p.viewHeight()
			p.cursor -= full
			if p.cursor < 0 {
				p.cursor = 0
			}
			p.ensureVisible()

		case key.Matches(msg, key.NewBinding(key.WithKeys("v"))):
			if p.selectFrom == -1 {
				p.selectFrom = p.cursor
			} else {
				p.selectFrom = -1
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("y"))):
			text := p.selectedText()
			text = strings.ReplaceAll(text, "```", "")
			err := clipboard.WriteAll(text)
			if err != nil {
				p.statusMsg = "Copy failed"
			} else {
				p.statusMsg = "Copied!"
			}
		}
	}

	return p, nil
}

func (p *Pager) viewHeight() int {
	// Reserve 1 line for the status bar
	h := p.height - 1
	if h < 1 {
		h = 1
	}
	return h
}

func (p *Pager) ensureVisible() {
	vh := p.viewHeight()
	if p.cursor < p.scroll {
		p.scroll = p.cursor
	} else if p.cursor >= p.scroll+vh {
		p.scroll = p.cursor - vh + 1
	}
}

func (p *Pager) selectedText() string {
	if p.selectFrom == -1 {
		if p.cursor >= 0 && p.cursor < len(p.lines) {
			return p.lines[p.cursor]
		}
		return ""
	}
	lo, hi := p.selectFrom, p.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(p.lines) {
		hi = len(p.lines) - 1
	}
	return strings.Join(p.lines[lo:hi+1], "\n")
}

func (p Pager) View() string {
	if len(p.lines) == 0 {
		return ""
	}

	vh := p.viewHeight()
	gutterWidth := len(fmt.Sprintf("%d", len(p.lines))) + 1

	lineNumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	selectStyle := lipgloss.NewStyle().Background(lipgloss.Color("237"))
	cursorLineNumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	selectLineNumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	lo, hi := -1, -1
	if p.selectFrom != -1 {
		lo, hi = p.selectFrom, p.cursor
		if lo > hi {
			lo, hi = hi, lo
		}
	}

	var b strings.Builder
	end := p.scroll + vh
	if end > len(p.lines) {
		end = len(p.lines)
	}

	for i := p.scroll; i < end; i++ {
		line := p.lines[i]

		isCursor := i == p.cursor
		isSelected := lo != -1 && i >= lo && i <= hi

		num := fmt.Sprintf("%*d ", gutterWidth, i+1)

		if isCursor {
			b.WriteString(cursorLineNumStyle.Render(num))
			if isSelected {
				b.WriteString(selectStyle.Render(line))
			} else {
				b.WriteString(cursorStyle.Render(line))
			}
		} else if isSelected {
			b.WriteString(selectLineNumStyle.Render(num))
			b.WriteString(selectStyle.Render(line))
		} else {
			b.WriteString(lineNumStyle.Render(num))
			b.WriteString(line)
		}

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines
	rendered := strings.Count(b.String(), "\n") + 1
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	for rendered < vh {
		b.WriteString("\n")
		b.WriteString(lineNumStyle.Render(strings.Repeat(" ", gutterWidth+1)))
		b.WriteString(dimStyle.Render("~"))
		rendered++
	}

	// Status bar
	statusBg := lipgloss.Color("236")
	modeStyle := lipgloss.NewStyle().
		Background(statusBg).
		Foreground(lipgloss.Color("170")).
		Bold(true)

	var mode string
	if p.selectFrom != -1 {
		mode = " VISUAL "
	} else {
		mode = " VIEW "
	}

	pos := fmt.Sprintf(" %d/%d ", p.cursor+1, len(p.lines))
	help := " [j/k]move [v]isual [y]ank [h/q]uit "

	left := modeStyle.Render(mode)
	right := lipgloss.NewStyle().
		Background(statusBg).
		Foreground(lipgloss.Color("245")).
		Render(pos)

	midText := help
	if p.statusMsg != "" {
		midText = " " + p.statusMsg + " "
	}
	mid := lipgloss.NewStyle().
		Background(statusBg).
		Foreground(lipgloss.Color("240")).
		Render(midText)

	gap := p.width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	pad := lipgloss.NewStyle().Background(statusBg).Width(gap).Render("")
	bar := lipgloss.NewStyle().
		Background(statusBg).
		Foreground(lipgloss.Color("252")).
		Width(p.width).
		Render(left + mid + pad + right)

	b.WriteString("\n")
	b.WriteString(bar)

	return b.String()
}
