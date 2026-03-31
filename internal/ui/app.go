package ui

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/config"
	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
	"github.com/user/lazyslack/internal/ui/screen"
)

// Screen re-exported from screen package
type Screen = screen.Screen

type paneState int

const (
	focusMain paneState = iota
	focusSidebar
)

type App struct {
	stack          []Screen
	quickSwitcher  *component.QuickSwitcher
	showHelp       bool
	sidebarVisible bool
	sidebarFocus   paneState
	client         *slack.Client
	formatter      *slack.Formatter
	config         *config.Config
	width          int
	height         int

	// Polling
	pollTicker tea.Cmd
	lastActive time.Time
}

func NewApp(client *slack.Client, cfg *config.Config) *App {
	formatter := slack.NewFormatter(client.Cache(), cfg.Display.TimestampFormat)
	uiState := config.LoadUIState()

	channelsScreen := screen.NewChannelsScreen(client, screen.ChannelsConfig{
		Types:      cfg.Channels.Types,
		UnreadOnly: uiState.UnreadOnly,
	})

	app := &App{
		stack:          []Screen{channelsScreen},
		sidebarVisible: uiState.SidebarVisible,
		client:         client,
		formatter:      formatter,
		config:         cfg,
		lastActive:     time.Now(),
	}
	return app
}

func (a *App) saveUIState() {
	unreadOnly := false
	if cs, ok := a.stack[0].(*screen.ChannelsScreen); ok {
		unreadOnly = cs.IsUnreadOnly()
	}
	config.SaveUIState(config.UIState{
		SidebarVisible: a.sidebarVisible,
		UnreadOnly:     unreadOnly,
	})
}

func (a *App) Init() tea.Cmd {
	var cmds []tea.Cmd
	if len(a.stack) > 0 {
		cmds = append(cmds, a.stack[0].Init())
	}
	cmds = append(cmds, a.startChannelListPolling())
	// Fetch usergroups in background for resolving <!subteam^...> mentions
	cmds = append(cmds, func() tea.Msg {
		_, _ = a.client.GetUserGroups()
		return nil
	})
	return tea.Batch(cmds...)
}

func (a *App) activeScreen() Screen {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

func (a *App) pushScreen(s Screen) tea.Cmd {
	a.stack = append(a.stack, s)
	a.resizeScreens()
	return s.Init()
}

func (a *App) popScreen() {
	if len(a.stack) > 1 {
		a.stack = a.stack[:len(a.stack)-1]
		a.resizeScreens()
		// Reset filter on the channel list when returning to it
		if channelsScreen, ok := a.activeScreen().(*screen.ChannelsScreen); ok {
			channelsScreen.ResetFilter()
		}
	}
}

func (a *App) sidebarWidth() int {
	if a.sidebarFocus != focusSidebar {
		// Collapsed width when not focused
		w := a.width / 8
		if w < 10 {
			w = 10
		}
		if w > 18 {
			w = 18
		}
		return w
	}
	w := a.width / 5
	if w < 15 {
		w = 15
	}
	if w > 30 {
		w = 30
	}
	return w
}

func (a *App) resizeScreens() {
	if a.sidebarVisible && len(a.stack) > 1 {
		sw := a.sidebarWidth()
		mainW := a.width - sw - 1 // 1 for border
		if mainW < 10 {
			mainW = 10
		}
		a.stack[0].SetSize(sw, a.height)
		for i := 1; i < len(a.stack); i++ {
			a.stack[i].SetSize(mainW, a.height)
		}
	} else {
		for _, s := range a.stack {
			s.SetSize(a.width, a.height)
		}
	}
}

func (a *App) isScreenInInsertMode() bool {
	target := a.activeScreen()
	if a.sidebarVisible && a.sidebarFocus == focusSidebar && len(a.stack) > 1 {
		target = a.stack[0]
	}
	switch s := target.(type) {
	case *screen.ChatScreen:
		return s.InInsertMode()
	case *screen.ThreadScreen:
		return s.InInsertMode()
	case *screen.SearchScreen:
		return true // search always has text input active
	case *screen.ChannelsScreen:
		return s.FilterState() == list.Filtering
	}
	return false
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	a.lastActive = time.Now()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resizeScreens()
		return a, nil

	case tea.KeyPressMsg:
		// Global keys
		if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))) {
			return a, tea.Quit
		}

		// Toggle help (not when typing in a text input)
		if key.Matches(msg, key.NewBinding(key.WithKeys("?"))) && !a.isScreenInInsertMode() {
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

		// Quick switcher overlay
		if a.quickSwitcher != nil {
			if key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+[", "ctrl+k"))) {
				a.quickSwitcher = nil
				return a, nil
			}
			var cmd tea.Cmd
			a.quickSwitcher, cmd = a.quickSwitcher.Update(msg)
			return a, cmd
		}

		// Ctrl+K: open quick switcher overlay (always available)
		if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+k"))) {
			qs := component.NewQuickSwitcher(a.client, a.width, a.height)
			a.quickSwitcher = qs
			return a, qs.Init()
		}

		// Tab to toggle focus between sidebar and main pane
		if a.sidebarVisible && len(a.stack) > 1 &&
			key.Matches(msg, key.NewBinding(key.WithKeys("tab"))) &&
			!a.isScreenInInsertMode() {
			if a.sidebarFocus == focusMain {
				a.sidebarFocus = focusSidebar
			} else {
				a.sidebarFocus = focusMain
			}
			a.resizeScreens()
			return a, nil
		}

		// Global search (skip when sidebar is focused so / can filter the channel list)
		if a.sidebarFocus != focusSidebar {
			if key.Matches(msg, key.NewBinding(key.WithKeys("/"))) {
				if _, isSearch := a.activeScreen().(*screen.SearchScreen); !isSearch {
					searchScreen := screen.NewSearchScreen(a.client, a.formatter)
					return a, a.pushScreen(searchScreen)
				}
			}
		}

		// Toggle sidebar layout
		if key.Matches(msg, Keys.ToggleLayout) && !a.isScreenInInsertMode() {
			a.sidebarVisible = !a.sidebarVisible
			if !a.sidebarVisible {
				a.sidebarFocus = focusMain
			}
			a.resizeScreens()
			a.saveUIState()
			return a, nil
		}

		// Search screen: enter opens selected result
		if searchScreen, ok := a.activeScreen().(*screen.SearchScreen); ok {
			if key.Matches(msg, key.NewBinding(key.WithKeys("enter"))) {
				if r := searchScreen.SelectedResult(); r != nil {
					for len(a.stack) > 1 {
						a.stack = a.stack[:len(a.stack)-1]
					}
					chatScreen := screen.NewChatScreen(a.client, a.formatter, r.ChannelID, "#"+r.ChannelName)
					cmd := a.pushScreen(chatScreen)
					pollCmd := a.startChannelPolling(r.ChannelID)
					return a, tea.Batch(cmd, pollCmd)
				}
			}
		}

	// Screen navigation messages
	case screen.OpenChannelMsg:
		// If this channel is already open, just focus the main pane without reloading
		if existing := a.findChatScreen(msg.Channel.ID); existing != nil {
			a.sidebarFocus = focusMain
			a.resizeScreens()
			return a, nil
		}
		prefix := "#"
		if msg.Channel.IsIM {
			prefix = "@"
		}
		if a.sidebarVisible && len(a.stack) > 1 {
			// In sidebar mode: replace the current chat (and any thread) with new channel
			a.stack = a.stack[:1]
		}
		chatScreen := screen.NewChatScreen(a.client, a.formatter, msg.Channel.ID, prefix+msg.Channel.Name)
		a.sidebarFocus = focusMain
		cmd := a.pushScreen(chatScreen)
		pollCmd := a.startChannelPolling(msg.Channel.ID)
		return a, tea.Batch(cmd, pollCmd)

	case component.QuickSwitchMsg:
		a.quickSwitcher = nil
		r := msg.Result
		if r.Channel != nil {
			// Open channel/person
			prefix := "#"
			if r.Channel.IsIM {
				prefix = "@"
			}
			if a.sidebarVisible && len(a.stack) > 1 {
				a.stack = a.stack[:1]
			}
			chatScreen := screen.NewChatScreen(a.client, a.formatter, r.Channel.ID, prefix+r.Channel.Name)
			a.sidebarFocus = focusMain
			cmd := a.pushScreen(chatScreen)
			pollCmd := a.startChannelPolling(r.Channel.ID)
			return a, tea.Batch(cmd, pollCmd)
		}
		if r.ChannelID != "" {
			// Jump to channel from message search
			for len(a.stack) > 1 {
				a.stack = a.stack[:len(a.stack)-1]
			}
			chatScreen := screen.NewChatScreen(a.client, a.formatter, r.ChannelID, "#"+r.ChannelName)
			cmd := a.pushScreen(chatScreen)
			pollCmd := a.startChannelPolling(r.ChannelID)
			return a, tea.Batch(cmd, pollCmd)
		}
		return a, nil

	case screen.UnreadToggleMsg:
		a.saveUIState()
		return a, nil

	case screen.OpenThreadMsg:
		threadScreen := screen.NewThreadScreen(a.client, a.formatter, msg.ChannelID, msg.ChannelName, msg.ThreadTS, msg.ParentMsg)
		cmd := a.pushScreen(threadScreen)
		pollCmd := a.startThreadPolling(msg.ChannelID, msg.ThreadTS)
		return a, tea.Batch(cmd, pollCmd)

	case screen.GoBackMsg:
		if a.sidebarVisible && len(a.stack) == 2 {
			// At chat level in sidebar mode: move focus to sidebar instead of closing chat
			a.sidebarFocus = focusSidebar
			a.resizeScreens()
			return a, nil
		}
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
		chatScreen := a.findChatScreen(msg.channelID)
		if chatScreen != nil {
			chatScreen.SetMessages(msg.messages)
		}
		// Only mark as read if the chat screen is the active (top) screen
		if chatScreen != nil && a.activeScreen() == chatScreen && len(msg.messages) > 0 {
			latestTS := msg.messages[len(msg.messages)-1].Timestamp
			markCmd := func() tea.Msg {
				_ = a.client.MarkChannel(msg.channelID, latestTS)
				return nil
			}
			return a, markCmd
		}
		return a, nil

	case threadPollTickMsg:
		threadScreen := a.findThreadScreen(msg.channelID, msg.threadTS)
		if threadScreen == nil {
			return a, nil
		}
		fetchCmd := func() tea.Msg {
			msgs, err := a.client.GetThreadReplies(msg.channelID, msg.threadTS)
			if err != nil {
				return pollErrorMsg{err: err}
			}
			return threadPollResultMsg{channelID: msg.channelID, threadTS: msg.threadTS, messages: msgs}
		}
		nextPoll := a.startThreadPolling(msg.channelID, msg.threadTS)
		return a, tea.Batch(fetchCmd, nextPoll)

	case threadPollResultMsg:
		if threadScreen := a.findThreadScreen(msg.channelID, msg.threadTS); threadScreen != nil {
			threadScreen.SetMessages(msg.messages)
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
			channelsScreen.SetLastPoll(time.Now())
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

	// Delegate key presses to sidebar when focused
	if a.sidebarVisible && a.sidebarFocus == focusSidebar && len(a.stack) > 1 {
		if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
			// "l" moves focus to main pane
			if key.Matches(keyMsg, key.NewBinding(key.WithKeys("l"))) {
				a.sidebarFocus = focusMain
				a.resizeScreens()
				return a, nil
			}
			// Remember selection before update
			var prevChannelID string
			if cs, ok := a.stack[0].(*screen.ChannelsScreen); ok {
				if ch := cs.SelectedChannel(); ch != nil {
					prevChannelID = ch.ID
				}
			}

			newScreen, cmd := a.stack[0].Update(msg)
			a.stack[0] = newScreen

			// Auto-load channel when cursor moves to a different one (skip during filtering)
			if cs, ok := a.stack[0].(*screen.ChannelsScreen); ok {
				if cs.FilterState() != list.Filtering {
					if ch := cs.SelectedChannel(); ch != nil && ch.ID != prevChannelID {
						a.stack = a.stack[:1]
						prefix := "#"
						if ch.IsIM {
							prefix = "@"
						}
						chatScreen := screen.NewChatScreen(a.client, a.formatter, ch.ID, prefix+ch.Name)
						initCmd := a.pushScreen(chatScreen)
						pollCmd := a.startChannelPolling(ch.ID)
						return a, tea.Batch(cmd, initCmd, pollCmd)
					}
				}
			}

			return a, cmd
		}
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

	var content string
	if a.sidebarVisible && len(a.stack) > 1 {
		sw := a.sidebarWidth()
		sidebarContent := a.stack[0].View()
		borderColor := lipgloss.Color("240") // gray when main is focused
		if a.sidebarFocus == focusSidebar {
			borderColor = lipgloss.Color("33") // blue when sidebar is focused
		}
		sidebarStyle := lipgloss.NewStyle().
			Width(sw).
			MaxWidth(sw).
			Height(a.height).
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(borderColor)
		content = lipgloss.JoinHorizontal(lipgloss.Top,
			sidebarStyle.Render(sidebarContent),
			active.View(),
		)
	} else {
		content = active.View()
	}

	if a.showHelp {
		content = component.NewHelpOverlay(a.width, a.height).View()
	} else if a.quickSwitcher != nil {
		content = a.quickSwitcher.View()
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

type threadPollTickMsg struct {
	channelID string
	threadTS  string
}

type threadPollResultMsg struct {
	channelID string
	threadTS  string
	messages  []slack.Message
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

func (a *App) startThreadPolling(channelID, threadTS string) tea.Cmd {
	return tea.Tick(a.config.Polling.Thread, func(t time.Time) tea.Msg {
		return threadPollTickMsg{channelID: channelID, threadTS: threadTS}
	})
}

// findChatScreen finds a ChatScreen for the given channel anywhere in the stack.
func (a *App) findChatScreen(channelID string) *screen.ChatScreen {
	for _, s := range a.stack {
		if cs, ok := s.(*screen.ChatScreen); ok && cs.ChannelID() == channelID {
			return cs
		}
	}
	return nil
}

// findThreadScreen finds a ThreadScreen for the given channel/thread anywhere in the stack.
func (a *App) findThreadScreen(channelID, threadTS string) *screen.ThreadScreen {
	for _, s := range a.stack {
		if ts, ok := s.(*screen.ThreadScreen); ok && ts.ChannelID() == channelID && ts.ThreadTS() == threadTS {
			return ts
		}
	}
	return nil
}

func (a *App) handlePollTick(msg pollTickMsg) (tea.Model, tea.Cmd) {
	// Keep polling as long as the channel's ChatScreen is in the stack
	// (it may be beneath a ThreadScreen)
	chatScreen := a.findChatScreen(msg.channelID)
	if chatScreen == nil {
		return a, nil
	}

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
