package ui

import (
	"log/slog"
	"time"

	"charm.land/bubbles/v2/key"
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
	statusBar      component.StatusBar
	quickSwitcher  *component.QuickSwitcher
	showHelp       bool
	sidebarVisible bool
	sidebarFocus   paneState
	client         *slack.Client
	formatter      *slack.Formatter
	config         *config.Config
	pinnedChannels       []string
	readTimestamps       map[string]string // channelID -> lastReadTS
	threadReadTimestamps map[string]string // "channelID:threadTS" -> lastReadTS (in-memory only)
	width                int
	height               int

	// Polling
	pollTicker tea.Cmd
	lastActive time.Time
}

func NewApp(client *slack.Client, cfg *config.Config) *App {
	formatter := slack.NewFormatter(client.Cache(), cfg.Display.TimestampFormat)
	uiState := config.LoadUIState()

	mentionsScreen := screen.NewMentionsScreen(client, formatter)

	app := &App{
		stack:                []Screen{mentionsScreen},
		statusBar:            component.NewStatusBar(),
		sidebarVisible:       uiState.SidebarVisible,
		client:               client,
		formatter:            formatter,
		config:               cfg,
		pinnedChannels:       uiState.PinnedChannels,
		readTimestamps:       uiState.ReadTimestamps,
		threadReadTimestamps: make(map[string]string),
		lastActive:           time.Now(),
	}
	return app
}

func (a *App) saveUIState() {
	config.SaveUIState(config.UIState{
		SidebarVisible: a.sidebarVisible,
		PinnedChannels: a.pinnedChannels,
		ReadTimestamps: a.readTimestamps,
		UnreadOnly:     true, // Default back to true if no ChannelsScreen
	})
}

func (a *App) Init() tea.Cmd {
	slog.Info("app init", "poll_channel_list", a.config.Polling.ChannelList, "poll_active", a.config.Polling.ActiveChannel)
	var cmds []tea.Cmd
	if len(a.stack) > 0 {
		cmds = append(cmds, a.stack[0].Init())
	}

	// Load from cache immediately if available
	if cachedChannels, err := a.client.Cache().LoadChannelsFromDisk(); err == nil && len(cachedChannels) > 0 {
		slog.Info("loaded channels from cache", "count", len(cachedChannels))
		if ms, ok := a.stack[0].(*screen.MentionsScreen); ok {
			ms.SetPinnedChannels(cachedChannels, a.pinnedChannels, a.readTimestamps)
		}
		cmds = append(cmds, func() tea.Msg {
			return channelListRefreshMsg{channels: cachedChannels}
		})
	}

	// Trigger immediate fetch instead of waiting for first poll tick
	cmds = append(cmds, func() tea.Msg {
		channels, err := a.client.GetChannels(a.config.Channels.Types, a.pinnedChannels)
		if err != nil {
			return pollErrorMsg{err: err, source: "channel_list"}
		}
		return channelListRefreshMsg{channels: channels}
	})

	cmds = append(cmds, a.startChannelListPolling())
	cmds = append(cmds, a.startPriorityPolling())
	cmds = append(cmds, a.startMentionsPolling())
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

func (a *App) contentHeight() int {
	return max(a.height-1, 3) // reserve 1 line for status bar
}

func (a *App) resizeScreens() {
	ch := a.contentHeight()
	a.statusBar.SetWidth(a.width)
	if a.sidebarVisible && len(a.stack) > 1 {
		sw := a.sidebarWidth()
		mainW := a.width - sw - 1 // 1 for border
		if mainW < 10 {
			mainW = 10
		}
		a.stack[0].SetSize(sw, ch)
		for i := 1; i < len(a.stack); i++ {
			a.stack[i].SetSize(mainW, ch)
		}
	} else {
		for _, s := range a.stack {
			s.SetSize(a.width, ch)
		}
	}
	// Status bar always spans full terminal width
	for _, s := range a.stack {
		s.SetStatusBarWidth(a.width)
	}
}

func (a *App) isScreenInInsertMode() bool {
	target := a.activeScreen()
	if a.sidebarVisible && a.sidebarFocus == focusSidebar && len(a.stack) > 1 {
		target = a.stack[0]
	}
	if target == nil {
		return false
	}
	return target.InInsertMode()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	a.lastActive = time.Now()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resizeScreens()
		if a.quickSwitcher != nil {
			a.quickSwitcher.SetSize(msg.Width, msg.Height)
		}
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

		// Global search
		if key.Matches(msg, key.NewBinding(key.WithKeys("/"))) && !a.isScreenInInsertMode() {
			if _, isSearch := a.activeScreen().(*screen.SearchScreen); !isSearch {
				searchScreen := screen.NewSearchScreen(a.client, a.formatter)
				return a, a.pushScreen(searchScreen)
			}
		}

		// Activity screen (toggle)
		if key.Matches(msg, key.NewBinding(key.WithKeys("a"))) && !a.isScreenInInsertMode() {
			if _, isActivity := a.activeScreen().(*screen.ActivityScreen); isActivity {
				a.popScreen()
				return a, nil
			}
			activityScreen := screen.NewActivityScreen(a.client, a.formatter)
			return a, a.pushScreen(activityScreen)
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

		// Search/Activity screen: enter opens selected result
		if key.Matches(msg, key.NewBinding(key.WithKeys("enter"))) {
			var r *slack.SearchResult
			if s, ok := a.activeScreen().(*screen.SearchScreen); ok {
				r = s.SelectedResult()
			} else if s, ok := a.activeScreen().(*screen.ActivityScreen); ok {
				r = s.SelectedResult()
			}
			if r != nil {
				markCmds := a.collectMarkCmds(a.stack[1:])
				for len(a.stack) > 1 {
					a.stack = a.stack[:len(a.stack)-1]
				}
				chatScreen := screen.NewChatScreen(a.client, a.formatter, r.ChannelID, "#"+r.ChannelName, a.readTimestamps[r.ChannelID])
				if r.Message.Timestamp != "" {
					chatScreen.SetTargetMessage(r.Message.Timestamp)
				}
				cmd := a.pushScreen(chatScreen)
				pollCmd := a.startChannelPolling(r.ChannelID)
				return a, tea.Batch(append(markCmds, cmd, pollCmd)...)
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
		var markCmds []tea.Cmd
		if a.sidebarVisible && len(a.stack) > 1 {
			// In sidebar mode: replace the current chat (and any thread) with new channel
			markCmds = a.collectMarkCmds(a.stack[1:])
			a.stack = a.stack[:1]
		}
		chatScreen := screen.NewChatScreen(a.client, a.formatter, msg.Channel.ID, prefix+msg.Channel.Name, a.readTimestamps[msg.Channel.ID])
		a.sidebarFocus = focusMain
		cmd := a.pushScreen(chatScreen)
		pollCmd := a.startChannelPolling(msg.Channel.ID)

		return a, tea.Batch(append(markCmds, cmd, pollCmd)...)

	case component.QuickSwitchMsg:
		a.quickSwitcher = nil
		r := msg.Result
		if r.Channel != nil {
			// Open channel/person
			prefix := "#"
			if r.Channel.IsIM {
				prefix = "@"
			}
			var markCmds []tea.Cmd
			if a.sidebarVisible && len(a.stack) > 1 {
				markCmds = a.collectMarkCmds(a.stack[1:])
				a.stack = a.stack[:1]
			}
			chatScreen := screen.NewChatScreen(a.client, a.formatter, r.Channel.ID, prefix+r.Channel.Name, a.readTimestamps[r.Channel.ID])
			a.sidebarFocus = focusMain
			cmd := a.pushScreen(chatScreen)
			pollCmd := a.startChannelPolling(r.Channel.ID)

			return a, tea.Batch(append(markCmds, cmd, pollCmd)...)
		}
		if r.ChannelID != "" {
			// Jump to channel from message search, focusing on the specific message
			markCmds := a.collectMarkCmds(a.stack[1:])
			for len(a.stack) > 1 {
				a.stack = a.stack[:len(a.stack)-1]
			}
			chatScreen := screen.NewChatScreen(a.client, a.formatter, r.ChannelID, "#"+r.ChannelName, a.readTimestamps[r.ChannelID])
			if r.MessageTS != "" {
				chatScreen.SetTargetMessage(r.MessageTS)
			}
			cmd := a.pushScreen(chatScreen)
			pollCmd := a.startChannelPolling(r.ChannelID)
			return a, tea.Batch(append(markCmds, cmd, pollCmd)...)
		}
		return a, nil

	case component.ToggleFavoriteMsg:
		a.quickSwitcher = nil
		found := false
		for i, id := range a.pinnedChannels {
			if id == msg.ChannelID {
				a.pinnedChannels = append(a.pinnedChannels[:i], a.pinnedChannels[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			a.pinnedChannels = append(a.pinnedChannels, msg.ChannelID)
		}
		a.saveUIState()

		// Immediately tell the sidebar about the new pinned IDs
		if ms, ok := a.stack[0].(*screen.MentionsScreen); ok {
			// Using the currently cached channels in the client
			ms.SetPinnedChannels(a.client.Cache().GetAllChannels(), a.pinnedChannels, a.readTimestamps)
		}

		// Also trigger a fresh fetch to be sure
		fetchCmd := func() tea.Msg {
			channels, err := a.client.GetChannels(a.config.Channels.Types, a.pinnedChannels)
			if err != nil {
				return pollErrorMsg{err: err, source: "channel_list"}
			}
			return channelListRefreshMsg{channels: channels}
		}
		return a, fetchCmd

	case screen.OpenThreadMsg:
		threadKey := msg.ChannelID + ":" + msg.ThreadTS
		threadScreen := screen.NewThreadScreen(a.client, a.formatter, msg.ChannelID, msg.ChannelName, msg.ThreadTS, msg.ParentMsg, a.threadReadTimestamps[threadKey])
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
		var markCmd tea.Cmd
		if top := a.activeScreen(); top != nil {
			if cs, ok := top.(*screen.ChatScreen); ok {
				markCmd = a.markChatRead(cs.ChannelID())
			}
		}
		a.popScreen()
		return a, markCmd

	case screen.JumpToChannelMsg:
		// Pop back to channels, then push chat
		markCmds := a.collectMarkCmds(a.stack[1:])
		for len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
		}
		chatScreen := screen.NewChatScreen(a.client, a.formatter, msg.ChannelID, "#"+msg.ChannelName, a.readTimestamps[msg.ChannelID])
		if msg.MessageTS != "" {
			chatScreen.SetTargetMessage(msg.MessageTS)
		}
		cmd := a.pushScreen(chatScreen)
		pollCmd := a.startChannelPolling(msg.ChannelID)
		return a, tea.Batch(append(markCmds, cmd, pollCmd)...)

	case screen.UpdateReadTimestampMsg:
		// Thread updates stay in an in-memory map so scrolling a thread doesn't
		// bump the parent channel's read position (which would falsely mark
		// unread channel messages as read).
		if msg.ThreadTS != "" {
			key := msg.ChannelID + ":" + msg.ThreadTS
			if msg.Timestamp > a.threadReadTimestamps[key] {
				a.threadReadTimestamps[key] = msg.Timestamp
			}
			return a, nil
		}

		if a.readTimestamps == nil {
			a.readTimestamps = make(map[string]string)
		}
		if msg.Timestamp > a.readTimestamps[msg.ChannelID] {
			a.readTimestamps[msg.ChannelID] = msg.Timestamp
			a.saveUIState()

			// Propagate back to sidebar
			if ms, ok := a.stack[0].(*screen.MentionsScreen); ok {
				ms.SetReadTimestamps(a.readTimestamps)
			}
		}
		return a, nil

	case pollTickMsg:
		return a.handlePollTick(msg)

	case pollResultMsg:
		slog.Debug("poll result", "channel", msg.channelID, "messages", len(msg.messages))
		var advanceCmd tea.Cmd
		chatScreen := a.findChatScreen(msg.channelID)
		if chatScreen != nil {
			advanceCmd = chatScreen.SetMessages(msg.messages)
		}

		// Update LatestTS in cache and propagate to sidebar
		if len(msg.messages) > 0 {
			latestTS := msg.messages[len(msg.messages)-1].Timestamp
			if a.client.Cache().AdvanceChannelLatestTS(msg.channelID, latestTS) {
				// Refresh sidebar if it's showing this channel
				if ms, ok := a.stack[0].(*screen.MentionsScreen); ok {
					ms.SetPinnedChannels(a.client.Cache().GetAllChannels(), a.pinnedChannels, a.readTimestamps)
				}
			}
		}

		return a, advanceCmd

	case threadPollTickMsg:
		threadScreen := a.findThreadScreen(msg.channelID, msg.threadTS)
		if threadScreen == nil {
			return a, nil
		}
		slog.Debug("thread poll tick", "channel", msg.channelID, "thread", msg.threadTS)
		fetchCmd := func() tea.Msg {
			msgs, err := a.client.GetThreadReplies(msg.channelID, msg.threadTS)
			if err != nil {
				return pollErrorMsg{err: err, source: "thread"}
			}
			return threadPollResultMsg{channelID: msg.channelID, threadTS: msg.threadTS, messages: msgs}
		}
		nextPoll := a.startThreadPolling(msg.channelID, msg.threadTS)
		return a, tea.Batch(fetchCmd, nextPoll)

	case threadPollResultMsg:
		var advanceCmd tea.Cmd
		if threadScreen := a.findThreadScreen(msg.channelID, msg.threadTS); threadScreen != nil {
			advanceCmd = threadScreen.SetMessages(msg.messages)
		}
		return a, advanceCmd

	case priorityUnreadPollTickMsg:
		slog.Debug("priority unread poll tick fired")
		priorityIDs := a.getPriorityIDs()
		if len(priorityIDs) == 0 {
			return a, a.startPriorityPolling()
		}

		fetchCmd := func() tea.Msg {
			channels, err := a.client.GetUnreadCounts(priorityIDs)
			if err != nil {
				slog.Error("priority unread poll fetch failed", "error", err)
				return nil // Don't crash for background error
			}
			return channelListRefreshMsg{channels: channels}
		}
		return a, tea.Batch(fetchCmd, a.startPriorityPolling())

	case channelListPollTickMsg:
		slog.Debug("channel list poll tick fired")

		// Priority channels are kept fresh by startPriorityPolling on a short
		// ticker; skip them here to avoid hitting conversations.info twice for
		// the same channel every full-list refresh.
		fetchCmd := func() tea.Msg {
			channels, err := a.client.GetChannels(a.config.Channels.Types, nil)
			if err != nil {
				slog.Error("channel list poll fetch failed", "error", err)
				return pollErrorMsg{err: err, source: "channel_list"}
			}
			return channelListRefreshMsg{channels: channels}
		}
		nextPoll := a.startChannelListPolling()
		return a, tea.Batch(fetchCmd, nextPoll)

	case channelListRefreshMsg:
		unreadCount := 0
		for _, ch := range msg.channels {
			if ch.UnreadCount > 0 {
				unreadCount++
			}
		}
		slog.Info("channel list refreshed", "total", len(msg.channels), "with_unread", unreadCount)

		// Update sidebar with merged state from cache
		if ms, ok := a.stack[0].(*screen.MentionsScreen); ok {
			ms.SetPinnedChannels(a.client.Cache().GetAllChannels(), a.pinnedChannels, a.readTimestamps)
		}

		// Update channels screen if it's in the stack
		for _, s := range a.stack {
			if cs, ok := s.(*screen.ChannelsScreen); ok {
				cs.SetChannels(a.client.Cache().GetAllChannels(), a.pinnedChannels, a.readTimestamps)
			}
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

	case mentionsPollTickMsg:
		slog.Debug("mentions poll tick fired")
		fetchCmd := func() tea.Msg {
			results, err := a.client.Search("to:me")
			if err != nil {
				slog.Error("mentions poll fetch failed", "error", err)
				return pollErrorMsg{err: err, source: "mentions"}
			}
			return screen.MentionsRefreshMsg{Results: results}
		}
		nextPoll := a.startMentionsPolling()
		return a, tea.Batch(fetchCmd, nextPoll)

	case screen.MentionsRefreshMsg:
		slog.Info("mentions refreshed", "count", len(msg.Results))
		if ms, ok := a.stack[0].(*screen.MentionsScreen); ok {
			ms.SetReadTimestamps(a.readTimestamps)
			ms.SetMentions(msg.Results)
			ms.SetLastPoll(time.Now())
		}
		return a, nil

	case pollErrorMsg:
		slog.Error("poll error", "source", msg.source, "error", msg.err)
		if ms, ok := a.stack[0].(*screen.MentionsScreen); ok && msg.source == "mentions" {
			ms.SetPollError(msg.err)
		}
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
			var prevID string
			if ms, ok := a.stack[0].(*screen.MentionsScreen); ok {
				if ch := ms.SelectedChannel(); ch != nil {
					prevID = ch.ID
				} else if r := ms.SelectedResult(); r != nil {
					prevID = r.ChannelID + ":" + r.Message.Timestamp
				}
			}

			newScreen, cmd := a.stack[0].Update(msg)
			a.stack[0] = newScreen

			// Auto-load selection when cursor moves
			if ms, ok := a.stack[0].(*screen.MentionsScreen); ok {
				var currID string
				if ch := ms.SelectedChannel(); ch != nil {
					currID = ch.ID
				} else if r := ms.SelectedResult(); r != nil {
					currID = r.ChannelID + ":" + r.Message.Timestamp
				}

				if currID != "" && currID != prevID {
					a.stack = a.stack[:1]

					// Pinned channel or mention result (both open ChatScreen)
					var channelID, name string
					isIM := false
					if ch := ms.SelectedChannel(); ch != nil {
						channelID = ch.ID
						name = ch.Name
						isIM = ch.IsIM
					} else if r := ms.SelectedResult(); r != nil {
						channelID = r.ChannelID
						name = r.ChannelName
						isIM = r.IsIM
					}

					prefix := "#"
					if isIM {
						prefix = "@"
					}
					chatScreen := screen.NewChatScreen(a.client, a.formatter, channelID, prefix+name, a.readTimestamps[channelID])
					initCmd := a.pushScreen(chatScreen)
					pollCmd := a.startChannelPolling(channelID)
					return a, tea.Batch(cmd, initCmd, pollCmd)
				}
			}

			return a, cmd
		}
	}

	// Route non-key messages to quick switcher (e.g., async search results)
	if a.quickSwitcher != nil {
		var cmd tea.Cmd
		a.quickSwitcher, cmd = a.quickSwitcher.Update(msg)
		return a, cmd
	}

	// Delegate to active screen
	active := a.activeScreen()
	if active != nil {
		newScreen, cmd := active.Update(msg)
		a.stack[len(a.stack)-1] = newScreen

		// Also forward non-key messages to the sidebar screen (stack[0]) when it's
		// in the background, so its async commands (Init, refresh) still complete
		// even when another screen is on top.
		if len(a.stack) > 1 {
			if _, isKey := msg.(tea.KeyPressMsg); !isKey {
				newSidebar, sidebarCmd := a.stack[0].Update(msg)
				a.stack[0] = newSidebar
				return a, tea.Batch(cmd, sidebarCmd)
			}
		}

		return a, cmd
	}

	return a, nil
}

func (a *App) View() tea.View {
	active := a.activeScreen()
	if active == nil {
		return tea.NewView("lazyslack")
	}

	ch := a.contentHeight()

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
			Height(ch).
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
		// Full-screen overlay, no status bar
		v := tea.NewView(content)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		v.WindowTitle = "lazyslack"
		return v
	} else if a.quickSwitcher != nil {
		content = a.quickSwitcher.View()
		// Full-screen overlay, no status bar
		v := tea.NewView(content)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		v.WindowTitle = "lazyslack"
		return v
	}

	// Get status bar from focused screen, fall back to app-level bar
	var statusView string
	if a.sidebarVisible && a.sidebarFocus == focusSidebar && len(a.stack) > 1 {
		statusView = a.stack[0].StatusBarView()
	} else {
		statusView = active.StatusBarView()
	}
	if statusView == "" {
		statusView = a.statusBar.View()
	}

	// Ensure status bar always spans full terminal width
	statusView = lipgloss.NewStyle().
		Width(a.width).
		MaxWidth(a.width).
		Render(statusView)

	// Force content to fill exactly contentHeight so the status bar is always at the bottom
	content = lipgloss.NewStyle().Width(a.width).Height(ch).MaxHeight(ch).Render(content)

	full := lipgloss.JoinVertical(lipgloss.Left, content, statusView)

	v := tea.NewView(full)
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
	err    error
	source string // "channel_list", "channel", "thread"
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

type mentionsPollTickMsg struct{}

type priorityUnreadPollTickMsg struct{}

func (a *App) startChannelListPolling() tea.Cmd {
	return tea.Tick(a.config.Polling.ChannelList, func(t time.Time) tea.Msg {
		return channelListPollTickMsg{}
	})
}

func (a *App) startPriorityPolling() tea.Cmd {
	return tea.Tick(a.config.Polling.Priority, func(t time.Time) tea.Msg {
		return priorityUnreadPollTickMsg{}
	})
}

func (a *App) startMentionsPolling() tea.Cmd {
	return tea.Tick(a.config.Polling.ChannelList, func(t time.Time) tea.Msg {
		return mentionsPollTickMsg{}
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

// markChatRead returns a background command that calls conversations.mark for
// the given channel using the locally tracked read timestamp, falling back to
// the cached LatestTS. Returns nil if there is nothing meaningful to mark.
func (a *App) markChatRead(channelID string) tea.Cmd {
	ts := a.readTimestamps[channelID]
	if ts == "" {
		if ch := a.client.Cache().GetChannel(channelID); ch != nil {
			ts = ch.LatestTS
		}
	}
	if ts == "" {
		return nil
	}
	return func() tea.Msg {
		if err := a.client.MarkChannel(channelID, ts); err != nil {
			slog.Debug("mark channel failed", "channel", channelID, "error", err)
		}
		return nil
	}
}

// collectMarkCmds returns mark commands for every ChatScreen in the given
// slice. Use before popping or replacing screens so Slack's server-side read
// state stays in sync with what the user has viewed locally.
func (a *App) collectMarkCmds(screens []Screen) []tea.Cmd {
	var cmds []tea.Cmd
	for _, s := range screens {
		if cs, ok := s.(*screen.ChatScreen); ok {
			if cmd := a.markChatRead(cs.ChannelID()); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
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

func (a *App) getPriorityIDs() []string {
	ids := make([]string, 0, len(a.pinnedChannels))
	ids = append(ids, a.pinnedChannels...)
	if len(a.stack) > 0 {
		if ms, ok := a.stack[0].(*screen.MentionsScreen); ok {
			for _, r := range ms.Results() {
				ids = append(ids, r.ChannelID)
			}
		}
	}
	return ids
}

func (a *App) handlePollTick(msg pollTickMsg) (tea.Model, tea.Cmd) {
	// Keep polling as long as the channel's ChatScreen is in the stack
	// (it may be beneath a ThreadScreen)
	chatScreen := a.findChatScreen(msg.channelID)
	if chatScreen == nil {
		slog.Debug("poll tick ignored, no chat screen", "channel", msg.channelID)
		return a, nil
	}

	slog.Debug("channel poll tick", "channel", msg.channelID)
	fetchCmd := func() tea.Msg {
		msgs, err := a.client.GetMessages(msg.channelID, 50, "")
		if err != nil {
			return pollErrorMsg{err: err, source: "channel"}
		}
		return pollResultMsg{channelID: msg.channelID, messages: msgs}
	}
	nextPoll := a.startChannelPolling(msg.channelID)
	return a, tea.Batch(fetchCmd, nextPoll)
}
