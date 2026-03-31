package component

import (
	"fmt"
	"io"
	"log/slog"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/user/lazyslack/internal/slack"
)

type ChannelItem struct {
	Channel slack.Channel
}

func (c ChannelItem) FilterValue() string { return c.Channel.Name }

type channelDelegate struct{}

func (d channelDelegate) Height() int                               { return 1 }
func (d channelDelegate) Spacing() int                              { return 0 }
func (d channelDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd   { return nil }
func (d channelDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(ChannelItem)
	if !ok {
		return
	}

	ch := item.Channel
	prefix := "#"
	prefixStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	if ch.IsIM {
		prefix = "@"
		prefixStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))
	} else if ch.IsMPIM {
		prefix = "@@"
		prefixStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))
	} else if ch.IsPrivate {
		prefix = "🔒"
	}

	name := ch.Name
	nameStyle := lipgloss.NewStyle()
	if ch.UnreadCount > 0 {
		nameStyle = nameStyle.Bold(true)
	}

	badge := ""
	if ch.UnreadCount > 0 {
		badge = fmt.Sprintf(" %d", ch.UnreadCount)
	}

	// Truncate name so the whole line fits in m.Width()
	// Layout: "> " or "  " (2) + prefix + name + badge
	prefixW := runewidth.StringWidth(prefix)
	maxName := m.Width() - 2 - prefixW - len(badge) - 2 // extra margin for list chrome
	if maxName < 1 {
		maxName = 1
	}
	name = runewidth.Truncate(name, maxName, "…")

	// Render badge with style after truncation
	if badge != "" {
		badge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true).
			Render(badge)
	}

	if index == m.Index() {
		nameStyle = nameStyle.Foreground(lipgloss.Color("170")).Bold(true)
		fmt.Fprintf(w, "> %s%s%s", prefixStyle.Render(prefix), nameStyle.Render(name), badge)
	} else {
		fmt.Fprintf(w, "  %s%s%s", prefixStyle.Render(prefix), nameStyle.Render(name), badge)
	}
}

type ChannelList struct {
	list        list.Model
	allChannels []slack.Channel
	unreadOnly  bool
}

func NewChannelList(width, height int, unreadOnly bool) ChannelList {
	l := list.New([]list.Item{}, channelDelegate{}, width, height)
	l.Title = "Channels"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))

	l.KeyMap.Filter = key.NewBinding(
		key.WithKeys("f", "/"),
		key.WithHelp("f", "filter"),
	)
	l.KeyMap.CursorUp = key.NewBinding(
		key.WithKeys("up", "k", "ctrl+p"),
		key.WithHelp("↑/k", "up"),
	)
	l.KeyMap.CursorDown = key.NewBinding(
		key.WithKeys("down", "j", "ctrl+n"),
		key.WithHelp("↓/j", "down"),
	)
	l.KeyMap.PrevPage = key.NewBinding(
		key.WithKeys("pgup", "ctrl+b", "ctrl+u"),
		key.WithHelp("ctrl+b/u", "page up"),
	)
	l.KeyMap.NextPage = key.NewBinding(
		key.WithKeys("pgdown", "ctrl+f", "ctrl+d"),
		key.WithHelp("ctrl+f/d", "page down"),
	)

	return ChannelList{list: l, unreadOnly: unreadOnly}
}

func (c *ChannelList) SetChannels(channels []slack.Channel) {
	c.allChannels = channels
	c.applyFilter()
}

func (c *ChannelList) ToggleUnreadOnly() {
	c.unreadOnly = !c.unreadOnly
	c.applyFilter()
}

func (c *ChannelList) IsUnreadOnly() bool {
	return c.unreadOnly
}

func (c *ChannelList) applyFilter() {
	channels := c.allChannels
	if c.unreadOnly {
		filtered := make([]slack.Channel, 0)
		for _, ch := range channels {
			if ch.UnreadCount > 0 {
				filtered = append(filtered, ch)
			}
		}
		channels = filtered
	}

	// Partition: unread channels first, then the rest
	var unread, read []slack.Channel
	for _, ch := range channels {
		if ch.UnreadCount > 0 {
			unread = append(unread, ch)
		} else {
			read = append(read, ch)
		}
	}

	ordered := make([]slack.Channel, 0, len(channels))
	ordered = append(ordered, unread...)
	ordered = append(ordered, read...)

	slog.Debug("channel list applyFilter",
		"total_input", len(c.allChannels),
		"unread_only", c.unreadOnly,
		"after_filter", len(channels),
		"unread", len(unread),
		"read", len(read),
	)

	items := make([]list.Item, len(ordered))
	for i, ch := range ordered {
		items[i] = ChannelItem{Channel: ch}
	}
	c.list.SetItems(items)
}

func (c *ChannelList) SetSize(w, h int) {
	c.list.SetSize(w, h)
}

func (c *ChannelList) SelectedChannel() *slack.Channel {
	item := c.list.SelectedItem()
	if item == nil {
		return nil
	}
	ci := item.(ChannelItem)
	return &ci.Channel
}

func (c *ChannelList) Update(msg tea.Msg) tea.Cmd {
	// If we are filtering, we might still want to allow up/down navigation
	if c.list.FilterState() == list.Filtering {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			switch {
			case key.Matches(keyMsg, key.NewBinding(key.WithKeys("down", "ctrl+n"))):
				c.list.CursorDown()
				return nil
			case key.Matches(keyMsg, key.NewBinding(key.WithKeys("up", "ctrl+p"))):
				c.list.CursorUp()
				return nil
			}
		}
	}

	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
	return cmd
}

func (c ChannelList) View() string {
	return c.list.View()
}

func (c *ChannelList) FilterState() list.FilterState {
	return c.list.FilterState()
}

func (c *ChannelList) ResetFilter() {
	c.list.ResetFilter()
}
