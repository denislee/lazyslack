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
	linkPicker     *component.LinkPicker
	pager          *component.Pager
	client         *slack.Client
	formatter      *slack.Formatter
	channelID      string
	channelName    string
	readTimestamp   string
	targetMessageTS string // when set, fetch messages around this TS and focus on it
	focus           chatFocus
	editingTS       string // non-empty when editing an existing message
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

func (s *ChatScreen) ChannelID() string          { return s.channelID }
func (s *ChatScreen) SetTargetMessage(ts string) { s.targetMessageTS = ts }
func (s *ChatScreen) InInsertMode() bool {
	return s.focus == focusComposer || s.reactionPicker != nil || s.linkPicker != nil || s.pager != nil
}

func (s *ChatScreen) Init() tea.Cmd {
	var cmds []tea.Cmd

	if s.targetMessageTS != "" {
		// Fetch messages around the target timestamp
		targetTS := s.targetMessageTS
		cmds = append(cmds, func() tea.Msg {
			msgs, err := s.client.GetMessagesAround(s.channelID, targetTS, 50)
			if err != nil {
				return chatErrorMsg{err: err}
			}
			return chatMessagesMsg{channelID: s.channelID, messages: msgs, targetTS: targetTS}
		})
	} else {
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
	}

	return tea.Batch(cmds...)
}

type chatMessagesMsg struct {
	channelID string
	messages  []slack.Message
	isCached  bool
	targetTS  string // when set, focus on this message instead of scrolling to bottom
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
				if msg.targetTS != "" {
					s.messageList.FocusOnTimestamp(msg.targetTS)
				} else {
					s.messageList.ScrollToBottom()
				}
			}
		}
		return s, nil

	case chatErrorMsg:
		s.statusBar.SetError(msg.err.Error())
		return s, nil

	case component.LinkPickedMsg:
		s.linkPicker = nil
		if msg.URL != "" {
			_ = openBrowser(msg.URL)
		}
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

	case component.PagerCloseMsg:
		s.pager = nil
		return s, nil

	case tea.KeyPressMsg:
		// Handle pager if open
		if s.pager != nil {
			p, cmd := s.pager.Update(msg)
			s.pager = &p
			return s, cmd
		}

		// Handle link picker if open
		if s.linkPicker != nil {
			if key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"))) {
				s.linkPicker = nil
				return s, nil
			}
			cmd := s.linkPicker.Update(msg)
			return s, cmd
		}

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
	s.statusBar.SetStatus("")
	s.statusBar.SetError("")
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
		s.editingTS = ""
		return s, s.composer.Focus()

	case key.Matches(msg, key.NewBinding(key.WithKeys("e"))):
		focused := s.messageList.FocusedMessage()
		if focused == nil {
			return s, nil
		}
		if focused.UserID == s.client.GetSelfID() {
			s.focus = focusComposer
			s.editingTS = focused.Timestamp
			s.composer.SetValue(focused.Text)
			return s, s.composer.Focus()
		}
		// View mode for other people's messages
		msgHeight := s.height - 2 // header only, no composer
		if msgHeight < 3 {
			msgHeight = 3
		}
		p := component.NewPager(focused.Text, s.width, msgHeight)
		s.pager = &p
		return s, nil

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
		urls := focused.AllURLs()
		if len(urls) == 1 {
			_ = openBrowser(urls[0])
			return s, nil
		}
		if len(urls) > 1 {
			picker := component.NewLinkPicker(focused, s.width-10)
			s.linkPicker = &picker
			return s, nil
		}
		// No URLs — open thread if it has replies
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
		s.editingTS = ""
		s.composer.Blur()
		s.composer.Reset()
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		text := strings.TrimSpace(s.composer.Value())
		if text == "" {
			return s, nil
		}
		s.composer.SaveToHistory(text)
		s.composer.Reset()
		s.focus = focusMessages
		s.composer.Blur()
		editingTS := s.editingTS
		s.editingTS = ""
		if editingTS != "" {
			return s, func() tea.Msg {
				err := s.client.UpdateMessage(s.channelID, editingTS, text)
				if err != nil {
					return chatErrorMsg{err: err}
				}
				msgs, err := s.client.GetMessages(s.channelID, 50, "")
				if err != nil {
					return chatErrorMsg{err: err}
				}
				return chatMessagesMsg{channelID: s.channelID, messages: msgs}
			}
		}
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
		if s.editingTS != "" {
			modeIndicator = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Render(" -- EDIT --")
		} else {
			modeIndicator = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Render(" -- INSERT --")
		}
	}

	messageView := s.messageList.View()
	if s.pager != nil {
		messageView = s.pager.View()
	} else if s.linkPicker != nil {
		msgHeight := s.height - 2
		if msgHeight < 3 {
			msgHeight = 3
		}
		messageView = lipgloss.Place(s.width, msgHeight, lipgloss.Center, lipgloss.Center, s.linkPicker.View())
	} else if s.reactionPicker != nil {
		msgHeight := s.height - 2
		if msgHeight < 3 {
			msgHeight = 3
		}
		messageView = lipgloss.Place(s.width, msgHeight, lipgloss.Center, lipgloss.Center, s.reactionPicker.View())
	}

	s.recalcMessageListSize()

	if s.pager != nil {
		return headerBar + "\n" + messageView
	}

	if s.focus == focusComposer {
		return headerBar + "\n" +
			messageView + "\n" +
			s.composer.View() + modeIndicator
	}

	return headerBar + "\n" + messageView
}

func (s *ChatScreen) StatusBarView() string {
	return s.statusBar.View()
}

func (s *ChatScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.composer.SetWidth(w)
	s.recalcMessageListSize()
}

func (s *ChatScreen) SetStatusBarWidth(w int) { s.statusBar.SetWidth(w) }

func (s *ChatScreen) recalcMessageListSize() {
	headerHeight := 2
	msgHeight := s.height - headerHeight
	if s.focus == focusComposer {
		msgHeight -= s.composer.Height()
	}
	if msgHeight < 3 {
		msgHeight = 3
	}
	s.messageList.SetSize(s.width, msgHeight)
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
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit/view")),
		key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open link/thread")),
		key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "reply")),
		key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r/+", "react")),
		key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "react all")),
		key.NewBinding(key.WithKeys("escape", "ctrl+["), key.WithHelp("esc", "back")),
	}
}
