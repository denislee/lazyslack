package component

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/user/lazyslack/internal/slack"
)

type QuickSwitchResult struct {
	// Channel-based result
	Channel *slack.Channel
	// Message-based result (jump to channel)
	ChannelID   string
	ChannelName string
	MessageTS   string
}

type QuickSwitchMsg struct {
	Result QuickSwitchResult
}

type ToggleFavoriteMsg struct {
	ChannelID string
}

type quickSwitchSearchResultsMsg struct {
	query   string
	results []slack.SearchResult
}

type resultKind int

const (
	resultChannel resultKind = iota
	resultPerson
	resultMessage
)

type resultEntry struct {
	kind    resultKind
	channel *slack.Channel   // for channel/person results
	search  *slack.SearchResult // for message results
}

type quickSwitchTab int

const (
	tabChannels quickSwitchTab = iota
	tabMessages
)

type QuickSwitcher struct {
	input    textinput.Model
	client   *slack.Client
	channels []slack.Channel
	results  []resultEntry
	tab      quickSwitchTab
	cursor   int
	width    int
	height   int

	lastQuery     string
	searchResults []slack.SearchResult
	searchLoading bool
}

func NewQuickSwitcher(client *slack.Client, width, height int) *QuickSwitcher {
	ti := textinput.New()
	ti.Placeholder = "Jump to channel, person, or search..."
	ti.Focus()

	channels := client.Cache().GetAllChannels()

	qs := &QuickSwitcher{
		input:    ti,
		client:   client,
		channels: channels,
		width:    width,
		height:   height,
	}
	qs.input.SetWidth(qs.boxWidth() - 6) // box - border(2) - padding(2) - indent(2)
	qs.filterResults()
	return qs
}

func (qs *QuickSwitcher) Init() tea.Cmd {
	return textinput.Blink
}

func (qs *QuickSwitcher) Update(msg tea.Msg) (*QuickSwitcher, tea.Cmd) {
	switch msg := msg.(type) {
	case quickSwitchSearchResultsMsg:
		if msg.query == strings.TrimSpace(qs.input.Value()) {
			qs.searchResults = msg.results
			qs.searchLoading = false
			qs.filterResults()
		}
		return qs, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(qs.results) > 0 && qs.cursor < len(qs.results) {
				entry := qs.results[qs.cursor]
				var result QuickSwitchResult
				switch entry.kind {
				case resultChannel, resultPerson:
					result.Channel = entry.channel
				case resultMessage:
					result.ChannelID = entry.search.ChannelID
					result.ChannelName = entry.search.ChannelName
					result.MessageTS = entry.search.Message.Timestamp
				}
				return qs, func() tea.Msg {
					return QuickSwitchMsg{Result: result}
				}
			}
			return qs, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+f"))):
			if len(qs.results) > 0 && qs.cursor < len(qs.results) {
				entry := qs.results[qs.cursor]
				if entry.channel != nil {
					id := entry.channel.ID
					return qs, func() tea.Msg {
						return ToggleFavoriteMsg{ChannelID: id}
					}
				}
			}
			return qs, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			if qs.tab == tabChannels {
				qs.tab = tabMessages
			} else {
				qs.tab = tabChannels
			}
			qs.cursor = 0
			qs.filterResults()
			
			// Trigger search if switching to messages and query is long enough
			query := strings.TrimSpace(qs.input.Value())
			if qs.tab == tabMessages && len(query) >= 2 {
				qs.searchLoading = true
				searchFn := func() tea.Msg {
					results, err := qs.client.Search(query)
					if err != nil {
						return quickSwitchSearchResultsMsg{query: query, results: nil}
					}
					return quickSwitchSearchResultsMsg{query: query, results: results}
				}
				return qs, searchFn
			}
			return qs, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "ctrl+n"))):
			qs.moveDown()
			return qs, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "ctrl+p"))):
			qs.moveUp()
			return qs, nil
		}
	}

	var cmd tea.Cmd
	qs.input, cmd = qs.input.Update(msg)

	query := strings.TrimSpace(qs.input.Value())
	qs.filterResults()

	// Async message search (only if on Messages tab)
	if qs.tab == tabMessages && query != qs.lastQuery && len(query) >= 2 {
		qs.lastQuery = query
		qs.searchLoading = true
		searchFn := func() tea.Msg {
			results, err := qs.client.Search(query)
			if err != nil {
				return quickSwitchSearchResultsMsg{query: query, results: nil}
			}
			return quickSwitchSearchResultsMsg{query: query, results: results}
		}
		return qs, tea.Batch(cmd, searchFn)
	} else if len(query) < 2 || qs.tab == tabChannels {
		qs.lastQuery = ""
		if qs.tab == tabChannels {
			qs.searchResults = nil
		}
		qs.searchLoading = false
		qs.filterResults()
	}

	return qs, cmd
}

// fuzzyScore returns a match score for query against target.
// Returns -1 if no match. Lower score = better match.
// Exact substring matches always rank above fuzzy matches.
func fuzzyScore(query, target string) int {
	if query == "" {
		return 0
	}

	queryLower := strings.ToLower(query)
	targetLower := strings.ToLower(target)

	// Exact substring match gets best score (position of match)
	if idx := strings.Index(targetLower, queryLower); idx >= 0 {
		return idx
	}

	// Fuzzy: all query runes must appear in target in order
	queryRunes := []rune(queryLower)
	targetRunes := []rune(targetLower)

	qi := 0
	score := len(targetRunes) // base penalty so fuzzy always ranks below substring
	lastMatch := -1

	for ti := 0; ti < len(targetRunes) && qi < len(queryRunes); ti++ {
		if targetRunes[ti] == queryRunes[qi] {
			if lastMatch >= 0 {
				score += ti - lastMatch - 1 // gap penalty
			} else {
				score += ti // penalize late first match
			}
			lastMatch = ti
			qi++
		}
	}

	if qi < len(queryRunes) {
		return -1 // not all chars matched
	}

	return score
}

func (qs *QuickSwitcher) filterResults() {
	query := strings.TrimSpace(qs.input.Value())
	qs.results = qs.results[:0]

	if qs.tab == tabChannels {
		if query == "" {
			// No query: show all, grouped by kind
			for i := range qs.channels {
				ch := &qs.channels[i]
				if ch.IsIM || ch.IsMPIM {
					continue
				}
				qs.results = append(qs.results, resultEntry{kind: resultChannel, channel: ch})
			}
			for i := range qs.channels {
				ch := &qs.channels[i]
				if !ch.IsIM {
					continue
				}
				qs.results = append(qs.results, resultEntry{kind: resultPerson, channel: ch})
			}
		} else {
			// Fuzzy match and sort by score
			type scored struct {
				entry resultEntry
				score int
			}
			var matches []scored

			for i := range qs.channels {
				ch := &qs.channels[i]
				if ch.IsIM || ch.IsMPIM {
					continue
				}
				if s := fuzzyScore(query, ch.Name); s >= 0 {
					matches = append(matches, scored{resultEntry{kind: resultChannel, channel: ch}, s})
				}
			}
			for i := range qs.channels {
				ch := &qs.channels[i]
				if !ch.IsIM {
					continue
				}
				if s := fuzzyScore(query, ch.Name); s >= 0 {
					matches = append(matches, scored{resultEntry{kind: resultPerson, channel: ch}, s})
				}
			}

			sort.Slice(matches, func(i, j int) bool {
				return matches[i].score < matches[j].score
			})
			for _, m := range matches {
				qs.results = append(qs.results, m.entry)
			}
		}
	} else {
		// Messages from search API
		for i := range qs.searchResults {
			r := &qs.searchResults[i]
			qs.results = append(qs.results, resultEntry{kind: resultMessage, search: r})
		}
	}

	if qs.cursor >= len(qs.results) {
		qs.cursor = max(0, len(qs.results)-1)
	}
}

func (qs *QuickSwitcher) moveDown() {
	if qs.cursor < len(qs.results)-1 {
		qs.cursor++
	}
}

func (qs *QuickSwitcher) moveUp() {
	if qs.cursor > 0 {
		qs.cursor--
	}
}

func (qs *QuickSwitcher) View() string {
	var b strings.Builder

	// Render tabs
	activeTabStyle := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("33")).
		Bold(true)
	inactiveTabStyle := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(lipgloss.Color("250")).
		Background(lipgloss.Color("238"))

	tabs := []string{"Channels", "Messages"}
	var renderedTabs []string
	for i, t := range tabs {
		if quickSwitchTab(i) == qs.tab {
			renderedTabs = append(renderedTabs, activeTabStyle.Render(t))
		} else {
			renderedTabs = append(renderedTabs, inactiveTabStyle.Render(t))
		}
	}
	tabHint := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("  tab ⇥")
	b.WriteString("  " + strings.Join(renderedTabs, " ") + tabHint + "\n\n")

	b.WriteString("  " + qs.input.View() + "\n")

	if len(qs.results) == 0 && qs.lastQuery != "" && !qs.searchLoading {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("\n  No results"))
	}

	maxVisible := 15
	start := 0
	if qs.cursor >= maxVisible {
		start = qs.cursor - maxVisible + 1
	}

	boxW := qs.boxWidth() - 6 // border(2) + padding(2) + indent(2)

	// Track sections for headers
	query := strings.TrimSpace(qs.input.Value())
	var lastKind resultKind = -1
	visible := 0
	for i := start; i < len(qs.results) && visible < maxVisible; i++ {
		entry := qs.results[i]

		// Section header (only for channels tab with no query — fuzzy results are sorted by score)
		if qs.tab == tabChannels && query == "" && entry.kind != lastKind {
			var header string
			switch entry.kind {
			case resultChannel:
				header = "Channels"
			case resultPerson:
				header = "People"
			}
			headerStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Bold(true)
			b.WriteString("\n " + headerStyle.Render(header) + "\n")
			lastKind = entry.kind
		}

		line := qs.renderEntry(entry, boxW)
		if i == qs.cursor {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color("170")).
				Bold(true).
				Render("> " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
		visible++
	}

	if qs.searchLoading {
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("\n  Searching messages..."))
	}

	style := lipgloss.NewStyle().
		Width(qs.boxWidth()).
		Padding(1, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("33")).
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("252"))

	return lipgloss.Place(
		qs.width, qs.height,
		lipgloss.Center, lipgloss.Center,
		style.Render(b.String()),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(lipgloss.Color("233"))),
	)
}

func (qs *QuickSwitcher) boxWidth() int {
	w := qs.width * 2 / 3
	if w > 80 {
		w = 80
	}
	if w < 40 {
		w = 40
	}
	return w
}

func (qs *QuickSwitcher) renderEntry(entry resultEntry, maxW int) string {
	switch entry.kind {
	case resultChannel:
		prefix := "#"
		if entry.channel.IsPrivate {
			prefix = "🔒"
		}
		name := runewidth.Truncate(entry.channel.Name, maxW-2, "…")
		return prefix + name

	case resultPerson:
		name := runewidth.Truncate(entry.channel.Name, maxW-2, "…")
		return "@" + name

	case resultMessage:
		ch := "#" + entry.search.ChannelName
		user := entry.search.Message.Username
		if user == "" {
			user = entry.search.Message.UserID
		}
		meta := ch + " | " + user + ": "
		metaW := runewidth.StringWidth(meta)
		textW := maxW - metaW
		if textW < 10 {
			textW = 10
		}
		text := strings.ReplaceAll(entry.search.Message.Text, "\n", " ")
		text = runewidth.Truncate(text, textW, "…")

		metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		return metaStyle.Render(meta) + text
	}
	return ""
}

func (qs *QuickSwitcher) SetSize(w, h int) {
	qs.width = w
	qs.height = h
	qs.input.SetWidth(qs.boxWidth() - 6)
}
