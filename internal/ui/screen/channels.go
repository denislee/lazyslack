package screen

import (
	"fmt"
	"log/slog"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
)

type ChannelsScreen struct {
	channelList    component.ChannelList
	statusBar      component.StatusBar
	client         *slack.Client
	config         ChannelsConfig
	readTimestamps map[string]string
	lastPoll       time.Time
	lastUnreadCount int
	width          int
	height         int
}

type ChannelsConfig struct {
	Types      []string
	PinnedIDs  []string
	UnreadOnly bool
}

func NewChannelsScreen(client *slack.Client, cfg ChannelsConfig) *ChannelsScreen {
	return &ChannelsScreen{
		channelList: component.NewChannelList(80, 20, cfg.UnreadOnly),
		statusBar:   component.NewStatusBar(),
		client:      client,
		config:      cfg,
	}
}

func (s *ChannelsScreen) Init() tea.Cmd {
	var cmds []tea.Cmd

	// Load from cache first if available
	if cachedChannels, err := s.client.Cache().LoadChannelsFromDisk(); err == nil && len(cachedChannels) > 0 {
		cmds = append(cmds, func() tea.Msg {
			return channelsDataMsg{channels: cachedChannels, isCached: true}
		})
	}

	// Fetch fresh channels in background
	cmds = append(cmds, func() tea.Msg {
		channels, err := s.client.GetChannels(s.config.Types, s.config.PinnedIDs)
		if err != nil {
			return channelsErrorMsg{err: err}
		}
		return channelsDataMsg{channels: channels, isCached: false}
	})

	return tea.Batch(cmds...)
}

type channelsDataMsg struct {
	channels []slack.Channel
	isCached bool
}

type channelsErrorMsg struct {
	err error
}

type OpenChannelMsg struct {
	Channel slack.Channel
}

type UnreadToggleMsg struct {
	UnreadOnly bool
}

func (s *ChannelsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case channelsDataMsg:
		unread := 0
		for _, ch := range msg.channels {
			if ch.UnreadCount > 0 {
				unread++
			}
		}
		slog.Info("channels data received", "total", len(msg.channels), "with_unread", unread, "cached", msg.isCached)
		s.channelList.SetChannels(msg.channels, s.config.PinnedIDs, s.readTimestamps)
		if msg.isCached {
			s.statusBar.SetStatus("Loading fresh channels in background...")
		} else {
			s.statusBar.SetStatus("Channels loaded")
		}
		return s, nil

	case channelsErrorMsg:
		slog.Error("channels load error", "error", msg.err)
		s.statusBar.SetError(msg.err.Error())
		return s, nil

	case tea.KeyPressMsg:
		// Don't intercept most keys when filtering, but do allow enter to quickly open the channel
		if s.channelList.FilterState() == list.Filtering {
			if key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+["))) {
				s.channelList.ResetFilter()
				return s, nil
			}
			if key.Matches(msg, key.NewBinding(key.WithKeys("enter"))) {
				if ch := s.channelList.SelectedChannel(); ch != nil {
					return s, func() tea.Msg {
						return OpenChannelMsg{Channel: *ch}
					}
				}
			}

			cmd := s.channelList.Update(msg)
			return s, cmd
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", "l"))):
			if ch := s.channelList.SelectedChannel(); ch != nil {
				return s, func() tea.Msg {
					return OpenChannelMsg{Channel: *ch}
				}
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("u"))):
			s.channelList.ToggleUnreadOnly()
			unread := s.channelList.IsUnreadOnly()
			if unread {
				s.statusBar.SetStatus("Showing unread only")
			} else {
				s.statusBar.SetStatus("")
			}
			return s, func() tea.Msg {
				return UnreadToggleMsg{UnreadOnly: unread}
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			return s, tea.Quit
		}
	}

	cmd := s.channelList.Update(msg)
	return s, cmd
}

func (s *ChannelsScreen) View() string {
	if !s.lastPoll.IsZero() {
		status := fmt.Sprintf("polled %s | %d unread", s.lastPoll.Format("15:04:05"), s.lastUnreadCount)
		s.statusBar.SetStatus(status)
	}
	return s.channelList.View() + "\n" + s.statusBar.View()
}

func (s *ChannelsScreen) SetLastPoll(t time.Time) {
	s.lastPoll = t
}

func (s *ChannelsScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.channelList.SetSize(w, h-1)
	s.statusBar.SetWidth(w)
}

func (s *ChannelsScreen) SetChannels(channels []slack.Channel, pinnedIDs []string, readTimestamps map[string]string) {
	unread := 0
	for _, ch := range channels {
		if ch.UnreadCount > 0 {
			unread++
		}
	}
	s.lastUnreadCount = unread
	s.config.PinnedIDs = pinnedIDs
	s.readTimestamps = readTimestamps
	s.channelList.SetChannels(channels, pinnedIDs, readTimestamps)
}

func (s *ChannelsScreen) SetReadTimestamps(readTimestamps map[string]string) {
	s.readTimestamps = readTimestamps
	s.channelList.SetChannels(s.channelList.AllChannels(), s.config.PinnedIDs, readTimestamps)
}

func (s *ChannelsScreen) SetPollError(err error) {
	s.statusBar.SetError("poll: " + err.Error())
}

func (s *ChannelsScreen) ResetFilter() {
	s.channelList.ResetFilter()
}

func (s *ChannelsScreen) IsUnreadOnly() bool {
	return s.channelList.IsUnreadOnly()
}

func (s *ChannelsScreen) FilterState() list.FilterState {
	return s.channelList.FilterState()
}

func (s *ChannelsScreen) SelectedChannel() *slack.Channel {
	return s.channelList.SelectedChannel()
}

func (s *ChannelsScreen) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter", "l"), key.WithHelp("enter/l", "open")),
		key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter")),
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	}
}
