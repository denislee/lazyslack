package screen

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
)

type chatFocus int

const (
	focusMessages chatFocus = iota
	focusComposer
)

type UpdateReadTimestampMsg struct {
	ChannelID string
	Timestamp string
}

type ChatScreen struct {
	messageList    component.MessageList
	composer       component.Composer
	statusBar      component.StatusBar
	reactionPicker *component.ReactionPicker
	client         *slack.Client
	formatter      *slack.Formatter
	channelID      string
	channelName    string
	readTimestamp  string
	focus          chatFocus
	width          int
	height         int
}

func NewChatScreen(client *slack.Client, formatter *slack.Formatter, channelID, channelName string, readTimestamp string) *ChatScreen {
	s := &ChatScreen{
		messageList:   component.NewMessageList(formatter, 80, 15),
		composer:      component.NewComposer(80),
		statusBar:     component.NewStatusBar(),
		client:        client,
		formatter:     formatter,
		channelID:     channelID,
		channelName:   channelName,
		readTimestamp: readTimestamp,
		focus:         focusMessages,
	}
	s.messageList.SetChannel(channelID, readTimestamp)
	s.statusBar.SetChannel(channelName)
	return s
}

func (s *ChatScreen) ChannelID() string    { return s.channelID }
func (s *ChatScreen) InInsertMode() bool {
	return s.focus == focusComposer || s.reactionPicker != nil
}

func (s *ChatScreen) Init() tea.Cmd {
	var cmds []tea.Cmd

	// 1. Load from cache immediately
	if cachedMsgs, err := s.client.Cache().LoadMessagesFromDisk(s.channelID); err == nil && len(cachedMsgs) > 0 {
		cmds = append(cmds, func() tea.Msg {
			return chatMessagesMsg{channelID: s.channelID, messages: cachedMsgs, isCached: true}
		})
	}

	// 2. Fetch fresh messages in background
	cmds = append(cmds, func() tea.Msg {
		msgs, err := s.client.GetMessages(s.channelID, 50, "")
		if err != nil {
			return chatErrorMsg{err: err}
		}
		return chatMessagesMsg{channelID: s.channelID, messages: msgs, isCached: false}
	})

	return tea.Batch(cmds...)
}

type chatMessagesMsg struct {
	channelID string
	messages  []slack.Message
	isCached  bool
}

type chatErrorMsg struct {
	err error
}

type OpenThreadMsg struct {
	ChannelID   string
	ChannelName string
	ThreadTS    string
	ParentMsg   slack.Message
}

type GoBackMsg struct{}

func (s *ChatScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case chatMessagesMsg:
		if msg.channelID == s.channelID {
			if msg.isCached {
				s.messageList.SetMessages(msg.messages)
				s.messageList.ScrollToBottom()
			} else {
				// Merge fresh messages with current (possibly cached) messages
				current := s.messageList.Messages()
				merged := s.client.MergeMessages(current, msg.messages)
				s.messageList.SetMessages(merged)
				s.messageList.ScrollToBottom()
			}
		}
		return s, nil

	case chatErrorMsg:
		s.statusBar.SetError(msg.err.Error())
		return s, nil

	case component.ReactionPickedMsg:
		s.reactionPicker = nil
		focused := s.messageList.FocusedMessage()
		if focused != nil {
			return s, func() tea.Msg {
				err := s.client.AddReaction(s.channelID, focused.Timestamp, msg.Emoji)
				if err != nil {
					return chatErrorMsg{err: err}
				}
				// Refresh messages to show the reaction
				msgs, err := s.client.GetMessages(s.channelID, 50, "")
				if err != nil {
					return chatErrorMsg{err: err}
				}
				return chatMessagesMsg{channelID: s.channelID, messages: msgs}
			}
		}
		return s, nil

	case tea.KeyPressMsg:
		// Handle reaction picker if open
		if s.reactionPicker != nil {
			if key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+["))) {
				s.reactionPicker = nil
				return s, nil
			}
			cmd := s.reactionPicker.Update(msg)
			return s, cmd
		}

		if s.focus == focusComposer {
			return s.handleComposerKey(msg)
		}
		return s.handleNormalKey(msg)

	case tea.PasteMsg:
		if s.focus == focusComposer {
			cmd := s.composer.Update(msg)
			return s, cmd
		}
	}

	return s, nil
}

func (s *ChatScreen) checkReadStatus() tea.Cmd {
	focused := s.messageList.FocusedMessage()
	if focused != nil && (s.readTimestamp == "" || focused.Timestamp > s.readTimestamp) {
		s.readTimestamp = focused.Timestamp
		s.messageList.SetReadTimestamp(s.readTimestamp)
		return func() tea.Msg {
			return UpdateReadTimestampMsg{
				ChannelID: s.channelID,
				Timestamp: s.readTimestamp,
			}
		}
	}
	return nil
}

func (s *ChatScreen) handleNormalKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	var cmds []tea.Cmd

	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"))):
		return s, func() tea.Msg { return GoBackMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down", "ctrl+n"))):
		s.messageList.MoveDown()
		cmds = append(cmds, s.checkReadStatus())

	case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up", "ctrl+p"))):
		s.messageList.MoveUp()
		cmds = append(cmds, s.checkReadStatus())

	case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
		s.messageList.GoToTop()
		cmds = append(cmds, s.checkReadStatus())

	case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
		s.messageList.GoToBottom()
		cmds = append(cmds, s.checkReadStatus())

	case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
		s.focus = focusComposer
		return s, s.composer.Focus()

	case key.Matches(msg, key.NewBinding(key.WithKeys("l"))):
		focused := s.messageList.FocusedMessage()
		if focused != nil {
			threadTS := focused.ThreadTS
			if threadTS == "" {
				threadTS = focused.Timestamp
			}
			return s, func() tea.Msg {
				return OpenThreadMsg{
					ChannelID:   s.channelID,
					ChannelName: s.channelName,
					ThreadTS:    threadTS,
					ParentMsg:   *focused,
				}
			}
		}
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		focused := s.messageList.FocusedMessage()
		if focused == nil {
			return s, nil
		}
		// If message has URLs, open the first one in the browser
		if urls := slack.ExtractURLs(focused.Text); len(urls) > 0 {
			_ = openBrowser(urls[0])
			return s, nil
		}
		// Otherwise open thread if it has replies
		if focused.ReplyCount > 0 {
			threadTS := focused.ThreadTS
			if threadTS == "" {
				threadTS = focused.Timestamp
			}
			return s, func() tea.Msg {
				return OpenThreadMsg{
					ChannelID:   s.channelID,
					ChannelName: s.channelName,
					ThreadTS:    threadTS,
					ParentMsg:   *focused,
				}
			}
		}
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("y"))):
		focused := s.messageList.FocusedMessage()
		if focused != nil {
			err := clipboard.WriteAll(focused.Text)
			if err != nil {
				s.statusBar.SetError("Failed to copy to clipboard")
			} else {
				s.statusBar.SetStatus("Copied to clipboard")
			}
		}
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("+", "r"))):
		if focused := s.messageList.FocusedMessage(); focused != nil {
			var existing []component.ExistingReaction
			for _, r := range focused.Reactions {
				existing = append(existing, component.ExistingReaction{Name: r.Name, HasMe: r.HasMe})
			}
			picker := component.NewReactionPicker(existing)
			s.reactionPicker = &picker
			return s, picker.Init()
		}
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("R"))):
		focused := s.messageList.FocusedMessage()
		if focused == nil || len(focused.Reactions) == 0 {
			return s, nil
		}
		allMine := true
		for _, r := range focused.Reactions {
			if !r.HasMe {
				allMine = false
				break
			}
		}
		reactions := focused.Reactions
		ts := focused.Timestamp
		chID := s.channelID
		return s, func() tea.Msg {
			for _, r := range reactions {
				if allMine {
					_ = s.client.RemoveReaction(chID, ts, r.Name)
				} else if !r.HasMe {
					_ = s.client.AddReaction(chID, ts, r.Name)
				}
			}
			msgs, err := s.client.GetMessages(chID, 50, "")
			if err != nil {
				return chatErrorMsg{err: err}
			}
			return chatMessagesMsg{channelID: chID, messages: msgs}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f", "pgdown"))):
		s.messageList.PageDown()
		cmds = append(cmds, s.checkReadStatus())

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b", "pgup"))):
		s.messageList.PageUp()
		cmds = append(cmds, s.checkReadStatus())
	}

	return s, tea.Batch(cmds...)
}

func (s *ChatScreen) handleComposerKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+["))):
		s.focus = focusMessages
		s.composer.Blur()
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		text := strings.TrimSpace(s.composer.Value())
		if text == "" {
			return s, nil
		}
		s.composer.Reset()
		s.focus = focusMessages
		s.composer.Blur()
		return s, func() tea.Msg {
			err := s.client.SendMessage(s.channelID, text)
			if err != nil {
				return chatErrorMsg{err: err}
			}
			// Refresh messages after sending
			msgs, err := s.client.GetMessages(s.channelID, 50, "")
			if err != nil {
				return chatErrorMsg{err: err}
			}
			return chatMessagesMsg{channelID: s.channelID, messages: msgs}
		}
	}

	cmd := s.composer.Update(msg)
	return s, cmd
}

func (s *ChatScreen) View() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("33")).
		Padding(0, 1).
		Render(s.channelName)

	headerBar := lipgloss.NewStyle().
		Width(s.width).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Render(header)

	modeIndicator := ""
	if s.focus == focusComposer {
		modeIndicator = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Render(" -- INSERT --")
	}

	messageView := s.messageList.View()
	if s.reactionPicker != nil {
		composerHeight := 3
		if s.focus == focusComposer {
			composerHeight = 5
		}
		msgHeight := s.height - 2 - composerHeight - 1 // header, composer, status
		if msgHeight < 3 {
			msgHeight = 3
		}
		messageView = lipgloss.Place(s.width, msgHeight, lipgloss.Center, lipgloss.Center, s.reactionPicker.View())
	}

	return headerBar + "\n" +
		messageView + "\n" +
		s.composer.View() + modeIndicator + "\n" +
		s.statusBar.View()
}

func (s *ChatScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
	composerHeight := 3
	if s.focus == focusComposer {
		composerHeight = 5
	}
	statusHeight := 1
	headerHeight := 2
	msgHeight := h - composerHeight - statusHeight - headerHeight
	if msgHeight < 3 {
		msgHeight = 3
	}
	s.messageList.SetSize(w, msgHeight)
	s.composer.SetWidth(w)
	s.statusBar.SetWidth(w)
}

func (s *ChatScreen) SetMessages(msgs []slack.Message) {
	s.messageList.SetMessages(msgs)
}

func (s *ChatScreen) SetUnreadCount(n int) {
	s.statusBar.SetUnreadCount(n)
}

func (s *ChatScreen) ShortHelp() []key.Binding {
	if s.focus == focusComposer {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
			key.NewBinding(key.WithKeys("escape", "ctrl+["), key.WithHelp("esc", "cancel")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "compose")),
		key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open link/thread")),
		key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "reply")),
		key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r/+", "react")),
		key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "react all")),
		key.NewBinding(key.WithKeys("escape", "ctrl+["), key.WithHelp("esc", "back")),
	}
}
