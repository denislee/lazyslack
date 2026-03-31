package screen

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
)

type ChannelsScreen struct {
	channelList component.ChannelList
	statusBar   component.StatusBar
	client      *slack.Client
	config      ChannelsConfig
	width       int
	height      int
}

type ChannelsConfig struct {
	Types []string
}

func NewChannelsScreen(client *slack.Client, cfg ChannelsConfig) *ChannelsScreen {
	return &ChannelsScreen{
		channelList: component.NewChannelList(80, 20),
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
		channels, err := s.client.GetChannels(s.config.Types)
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

func (s *ChannelsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case channelsDataMsg:
		s.channelList.SetChannels(msg.channels)
		if msg.isCached {
			s.statusBar.SetStatus("Loading fresh channels in background...")
		} else {
			s.statusBar.SetStatus("Channels loaded")
		}
		return s, nil

	case channelsErrorMsg:
		s.statusBar.SetError(msg.err.Error())
		return s, nil

	case tea.KeyPressMsg:
		// Don't intercept most keys when filtering, but do allow enter to quickly open the channel
		if s.channelList.FilterState() == list.Filtering {
			if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+k"))) {
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
			if s.channelList.IsUnreadOnly() {
				s.statusBar.SetStatus("Showing unread only")
			} else {
				s.statusBar.SetStatus("")
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			return s, tea.Quit
		}
	}

	cmd := s.channelList.Update(msg)
	return s, cmd
}

func (s *ChannelsScreen) View() string {
	return s.channelList.View() + "\n" + s.statusBar.View()
}

func (s *ChannelsScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.channelList.SetSize(w, h-1)
	s.statusBar.SetWidth(w)
}

func (s *ChannelsScreen) SetChannels(channels []slack.Channel) {
	s.channelList.SetChannels(channels)
}

func (s *ChannelsScreen) ResetFilter() {
	s.channelList.ResetFilter()
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
