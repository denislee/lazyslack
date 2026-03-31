package ui

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/user/lazyslack/internal/config"
	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
	"github.com/user/lazyslack/internal/ui/screen"
)

// Screen and Overlay interfaces (re-exported from screen package for commands.go)
type Screen = screen.Screen

type Overlay interface {
	Update(msg tea.Msg) (Overlay, tea.Cmd)
	View() string
}

type App struct {
	stack     []Screen
	overlay   Overlay
	showHelp  bool
	client    *slack.Client
	formatter *slack.Formatter
	config    *config.Config
	width     int
	height    int

	// Polling
	pollTicker tea.Cmd
	lastActive time.Time
}

func NewApp(client *slack.Client, cfg *config.Config) *App {
	formatter := slack.NewFormatter(client.Cache(), cfg.Display.TimestampFormat)

	channelsScreen := screen.NewChannelsScreen(client, screen.ChannelsConfig{
		Types: cfg.Channels.Types,
	})

	app := &App{
		stack:      []Screen{channelsScreen},
		client:     client,
		formatter:  formatter,
		config:     cfg,
		lastActive: time.Now(),
	}
	return app
}

func (a *App) Init() tea.Cmd {
	var cmds []tea.Cmd
	if len(a.stack) > 0 {
		cmds = append(cmds, a.stack[0].Init())
	}
	cmds = append(cmds, a.startChannelListPolling())
	return tea.Batch(cmds...)
}

func (a *App) activeScreen() Screen {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

func (a *App) pushScreen(s Screen) tea.Cmd {
	s.SetSize(a.width, a.height)
	a.stack = append(a.stack, s)
	return s.Init()
}

func (a *App) popScreen() {
	if len(a.stack) > 1 {
		a.stack = a.stack[:len(a.stack)-1]
	}
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	a.lastActive = time.Now()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		for _, s := range a.stack {
			s.SetSize(msg.Width, msg.Height)
		}
		return a, nil

	case tea.KeyPressMsg:
		// Global keys
		if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
			return a, tea.Quit
		}

		// Toggle help
		if key.Matches(msg, key.NewBinding(key.WithKeys("?"))) {
			a.showHelp = !a.showHelp
			return a, nil
		}

		// Close help or overlay on escape
		if a.showHelp {
			if key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+["))) {
				a.showHelp = false
			}
			return a, nil
		}

		// Close overlay first
		if a.overlay != nil {
			if key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+["))) {
				a.overlay = nil
				return a, nil
			}
			var cmd tea.Cmd
			a.overlay, cmd = a.overlay.Update(msg)
			return a, cmd
		}

		// Ctrl+K: quick channel switcher — pop back to channel list
		if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+k"))) {
			// Pop all screens back to the channel list
			for len(a.stack) > 1 {
				a.stack = a.stack[:len(a.stack)-1]
			}
			// Let it fall through so the ctrl+k is passed to the channel list to trigger filter
		}

		// Global search
		if key.Matches(msg, key.NewBinding(key.WithKeys("/"))) {
			if _, isSearch := a.activeScreen().(*screen.SearchScreen); !isSearch {
				searchScreen := screen.NewSearchScreen(a.client, a.formatter)
				return a, a.pushScreen(searchScreen)
			}
		}

	// Screen navigation messages
	case screen.OpenChannelMsg:
		prefix := "#"
		if msg.Channel.IsIM {
			prefix = "@"
		}
		chatScreen := screen.NewChatScreen(a.client, a.formatter, msg.Channel.ID, prefix+msg.Channel.Name)
		cmd := a.pushScreen(chatScreen)
		pollCmd := a.startChannelPolling(msg.Channel.ID)
		return a, tea.Batch(cmd, pollCmd)

	case screen.OpenThreadMsg:
		threadScreen := screen.NewThreadScreen(a.client, a.formatter, msg.ChannelID, msg.ChannelName, msg.ThreadTS, msg.ParentMsg)
		return a, a.pushScreen(threadScreen)

	case screen.GoBackMsg:
		a.popScreen()
		return a, nil

	case screen.JumpToChannelMsg:
		// Pop back to channels, then push chat
		for len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
		}
		chatScreen := screen.NewChatScreen(a.client, a.formatter, msg.ChannelID, "#"+msg.ChannelName)
		cmd := a.pushScreen(chatScreen)
		pollCmd := a.startChannelPolling(msg.ChannelID)
		return a, tea.Batch(cmd, pollCmd)

	case pollTickMsg:
		return a.handlePollTick(msg)

	case pollResultMsg:
		if chatScreen, ok := a.activeScreen().(*screen.ChatScreen); ok {
			if chatScreen.ChannelID() == msg.channelID {
				chatScreen.SetMessages(msg.messages)
				if len(msg.messages) > 0 {
					latestTS := msg.messages[len(msg.messages)-1].Timestamp
					markCmd := func() tea.Msg {
						_ = a.client.MarkChannel(msg.channelID, latestTS)
						return nil
					}
					return a, markCmd
				}
			}
		}
		return a, nil

	case channelListPollTickMsg:
		fetchCmd := func() tea.Msg {
			channels, err := a.client.GetChannels(a.config.Channels.Types)
			if err != nil {
				return pollErrorMsg{err: err}
			}
			return channelListRefreshMsg{channels: channels}
		}
		nextPoll := a.startChannelListPolling()
		return a, tea.Batch(fetchCmd, nextPoll)

	case channelListRefreshMsg:
		// Update channel list screen (always the first screen in the stack)
		if channelsScreen, ok := a.stack[0].(*screen.ChannelsScreen); ok {
			channelsScreen.SetChannels(msg.channels)
		}
		// Update unread count on chat screen if active
		if chatScreen, ok := a.activeScreen().(*screen.ChatScreen); ok {
			total := 0
			for _, ch := range msg.channels {
				if ch.ID != chatScreen.ChannelID() && ch.UnreadCount > 0 {
					total += ch.UnreadCount
				}
			}
			chatScreen.SetUnreadCount(total)
		}
		return a, nil

	case pollErrorMsg:
		// Silently ignore poll errors, will retry on next tick
		return a, nil
	}

	// Delegate to active screen
	active := a.activeScreen()
	if active != nil {
		newScreen, cmd := active.Update(msg)
		a.stack[len(a.stack)-1] = newScreen
		return a, cmd
	}

	return a, nil
}

func (a *App) View() tea.View {
	active := a.activeScreen()
	if active == nil {
		return tea.NewView("lazyslack")
	}

	content := active.View()
	if a.showHelp {
		content = component.NewHelpOverlay(a.width, a.height).View()
	} else if a.overlay != nil {
		content += "\n" + a.overlay.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "lazyslack"
	return v
}

// Polling

type pollTickMsg struct {
	channelID string
}

type pollResultMsg struct {
	channelID string
	messages  []slack.Message
}

type pollErrorMsg struct {
	err error
}

type channelListPollTickMsg struct{}

type channelListRefreshMsg struct {
	channels []slack.Channel
}

func (a *App) startChannelListPolling() tea.Cmd {
	return tea.Tick(a.config.Polling.ChannelList, func(t time.Time) tea.Msg {
		return channelListPollTickMsg{}
	})
}

func (a *App) startChannelPolling(channelID string) tea.Cmd {
	return tea.Tick(a.config.Polling.ActiveChannel, func(t time.Time) tea.Msg {
		return pollTickMsg{channelID: channelID}
	})
}

func (a *App) handlePollTick(msg pollTickMsg) (tea.Model, tea.Cmd) {
	// Only poll if we're viewing this channel
	active := a.activeScreen()
	if active == nil {
		return a, nil
	}

	chatScreen, isChat := active.(*screen.ChatScreen)
	if !isChat || chatScreen.ChannelID() != msg.channelID {
		return a, nil
	}

	// Fetch and continue polling
	fetchCmd := func() tea.Msg {
		msgs, err := a.client.GetMessages(msg.channelID, 50, "")
		if err != nil {
			return pollErrorMsg{err: err}
		}
		return pollResultMsg{channelID: msg.channelID, messages: msgs}
	}
	nextPoll := a.startChannelPolling(msg.channelID)
	return a, tea.Batch(fetchCmd, nextPoll)
}
