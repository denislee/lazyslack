package screen

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
)

type ThreadScreen struct {
	messageList component.MessageList
	composer    component.Composer
	statusBar   component.StatusBar
	client      *slack.Client
	formatter   *slack.Formatter
	channelID   string
	channelName string
	threadTS    string
	parentMsg   slack.Message
	focus       chatFocus
	width       int
	height      int
}

func NewThreadScreen(client *slack.Client, formatter *slack.Formatter, channelID, channelName, threadTS string, parentMsg slack.Message) *ThreadScreen {
	s := &ThreadScreen{
		messageList: component.NewMessageList(formatter, 80, 15),
		composer:    component.NewComposer(80),
		statusBar:   component.NewStatusBar(),
		client:      client,
		formatter:   formatter,
		channelID:   channelID,
		channelName: channelName,
		threadTS:    threadTS,
		parentMsg:   parentMsg,
		focus:       focusMessages,
	}
	s.statusBar.SetChannel(fmt.Sprintf("Thread in %s", channelName))
	return s
}

func (s *ThreadScreen) ChannelID() string  { return s.channelID }
func (s *ThreadScreen) ThreadTS() string   { return s.threadTS }
func (s *ThreadScreen) InInsertMode() bool { return s.focus == focusComposer }

func (s *ThreadScreen) SetMessages(msgs []slack.Message) {
	s.messageList.SetMessages(msgs)
}

func (s *ThreadScreen) Init() tea.Cmd {
	return func() tea.Msg {
		msgs, err := s.client.GetThreadReplies(s.channelID, s.threadTS)
		if err != nil {
			return threadErrorMsg{err: err}
		}
		return threadMessagesMsg{messages: msgs}
	}
}

type threadMessagesMsg struct {
	messages []slack.Message
}

type threadErrorMsg struct {
	err error
}

func (s *ThreadScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.SetSize(msg.Width, msg.Height)
		return s, nil

	case threadMessagesMsg:
		s.messageList.SetMessages(msg.messages)
		s.messageList.ScrollToBottom()
		return s, nil

	case threadErrorMsg:
		s.statusBar.SetError(msg.err.Error())
		return s, nil

	case tea.KeyPressMsg:
		if s.focus == focusComposer {
			return s.handleComposerKey(msg)
		}
		return s.handleNormalKey(msg)
	}

	return s, nil
}

func (s *ThreadScreen) handleNormalKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"))):
		return s, func() tea.Msg { return GoBackMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down", "ctrl+n"))):
		s.messageList.MoveDown()
	case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up", "ctrl+p"))):
		s.messageList.MoveUp()
	case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
		s.messageList.GoToTop()
	case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
		s.messageList.GoToBottom()
	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f", "pgdown"))):
		s.messageList.PageDown()
	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b", "pgup"))):
		s.messageList.PageUp()
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		focused := s.messageList.FocusedMessage()
		if focused != nil {
			if urls := slack.ExtractURLs(focused.Text); len(urls) > 0 {
				_ = openBrowser(urls[0])
			}
		}
		return s, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
		s.focus = focusComposer
		return s, s.composer.Focus()
	}

	return s, nil
}

func (s *ThreadScreen) handleComposerKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
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
			err := s.client.SendThreadReply(s.channelID, s.threadTS, text)
			if err != nil {
				return threadErrorMsg{err: err}
			}
			msgs, err := s.client.GetThreadReplies(s.channelID, s.threadTS)
			if err != nil {
				return threadErrorMsg{err: err}
			}
			return threadMessagesMsg{messages: msgs}
		}
	}

	cmd := s.composer.Update(msg)
	return s, cmd
}

func (s *ThreadScreen) View() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("33")).
		Padding(0, 1).
		Render(fmt.Sprintf("Thread in %s", s.channelName))

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

	return headerBar + "\n" +
		s.messageList.View() + "\n" +
		s.composer.View() + modeIndicator + "\n" +
		s.statusBar.View()
}

func (s *ThreadScreen) SetSize(w, h int) {
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

func (s *ThreadScreen) ShortHelp() []key.Binding {
	if s.focus == focusComposer {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
			key.NewBinding(key.WithKeys("escape", "ctrl+["), key.WithHelp("esc", "cancel")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "reply")),
		key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open link")),
		key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"), key.WithHelp("esc/h", "back")),
	}
}
