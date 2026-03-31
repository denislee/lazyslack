package screen

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
)

type chatFocus int

const (
	focusMessages chatFocus = iota
	focusComposer
)

type ChatScreen struct {
	messageList    component.MessageList
	composer       component.Composer
	statusBar      component.StatusBar
	reactionPicker *component.ReactionPicker
	client         *slack.Client
	formatter      *slack.Formatter
	channelID      string
	channelName    string
	focus          chatFocus
	width          int
	height         int
}

func NewChatScreen(client *slack.Client, formatter *slack.Formatter, channelID, channelName string) *ChatScreen {
	s := &ChatScreen{
		messageList: component.NewMessageList(formatter, 80, 15),
		composer:    component.NewComposer(80),
		statusBar:   component.NewStatusBar(),
		client:      client,
		formatter:   formatter,
		channelID:   channelID,
		channelName: channelName,
		focus:       focusMessages,
	}
	s.statusBar.SetChannel(channelName)
	return s
}

func (s *ChatScreen) ChannelID() string    { return s.channelID }
func (s *ChatScreen) InInsertMode() bool   { return s.focus == focusComposer }

func (s *ChatScreen) Init() tea.Cmd {
	return func() tea.Msg {
		msgs, err := s.client.GetMessages(s.channelID, 50, "")
		if err != nil {
			return chatErrorMsg{err: err}
		}
		return chatMessagesMsg{channelID: s.channelID, messages: msgs}
	}
}

type chatMessagesMsg struct {
	channelID string
	messages  []slack.Message
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
			s.messageList.SetMessages(msg.messages)
			s.messageList.ScrollToBottom()
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
	}

	return s, nil
}

func (s *ChatScreen) handleNormalKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"))):
		return s, func() tea.Msg { return GoBackMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down", "ctrl+n"))):
		s.messageList.MoveDown()
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up", "ctrl+p"))):
		s.messageList.MoveUp()
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
		s.messageList.GoToTop()
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
		s.messageList.GoToBottom()
		return s, nil

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

	case key.Matches(msg, key.NewBinding(key.WithKeys("+", "r"))):
		if s.messageList.FocusedMessage() != nil {
			picker := component.NewReactionPicker(s.width, s.height)
			s.reactionPicker = &picker
			return s, picker.Init()
		}
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f", "pgdown"))):
		s.messageList.PageDown()
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b", "pgup"))):
		s.messageList.PageUp()
		return s, nil
	}

	return s, nil
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

	base := headerBar + "\n" +
		s.messageList.View() + "\n" +
		s.composer.View() + modeIndicator + "\n" +
		s.statusBar.View()

	if s.reactionPicker != nil {
		return s.reactionPicker.View()
	}

	return base
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
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r/+", "react")),
		key.NewBinding(key.WithKeys("escape", "ctrl+["), key.WithHelp("esc", "back")),
	}
}
