package screen

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/user/lazyslack/internal/slack"
	"github.com/user/lazyslack/internal/ui/component"
)

type ThreadScreen struct {
	messageList    component.MessageList
	composer       component.Composer
	statusBar      component.StatusBar
	profilePanel   component.UserProfilePanel
	profileVisible bool
	client         *slack.Client
	formatter      *slack.Formatter
	channelID      string
	channelName    string
	threadTS       string
	readTimestamp  string
	parentMsg      slack.Message
	focus          chatFocus
	width          int
	height         int
}

func NewThreadScreen(client *slack.Client, formatter *slack.Formatter, channelID, channelName, threadTS string, parentMsg slack.Message, readTimestamp string) *ThreadScreen {
	s := &ThreadScreen{
		messageList:   component.NewMessageList(formatter, 80, 15),
		composer:      component.NewComposer(80),
		statusBar:     component.NewStatusBar(),
		client:        client,
		formatter:     formatter,
		channelID:     channelID,
		channelName:   channelName,
		threadTS:      threadTS,
		readTimestamp: readTimestamp,
		parentMsg:     parentMsg,
		focus:         focusMessages,
	}
	s.messageList.SetChannel(channelID, readTimestamp)
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
	var cmds []tea.Cmd

	// 1. Load from cache (using a special thread key)
	threadKey := s.channelID + ":" + s.threadTS
	if cachedMsgs, err := s.client.Cache().LoadMessagesFromDisk(threadKey); err == nil && len(cachedMsgs) > 0 {
		cmds = append(cmds, func() tea.Msg {
			return threadMessagesMsg{messages: cachedMsgs, isCached: true}
		})
	}

	// 2. Fetch fresh messages in background
	cmds = append(cmds, func() tea.Msg {
		msgs, err := s.client.GetThreadReplies(s.channelID, s.threadTS)
		if err != nil {
			return threadErrorMsg{err: err}
		}
		return threadMessagesMsg{messages: msgs, isCached: false}
	})

	return tea.Batch(cmds...)
}

type threadMessagesMsg struct {
	messages []slack.Message
	isCached bool
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
		if msg.isCached {
			s.messageList.SetMessages(msg.messages)
			s.messageList.ScrollToBottom()
		} else {
			current := s.messageList.Messages()
			merged := s.client.MergeMessages(current, msg.messages)
			s.messageList.SetMessages(merged)
			s.messageList.ScrollToBottom()
		}
		return s, nil

	case threadErrorMsg:
		s.statusBar.SetError(msg.err.Error())
		return s, nil

	case tea.KeyPressMsg:
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

func (s *ThreadScreen) checkReadStatus() tea.Cmd {
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

func (s *ThreadScreen) updateProfilePanel() {
	if !s.profileVisible {
		return
	}
	focused := s.messageList.FocusedMessage()
	if focused == nil {
		return
	}
	user, err := s.client.ResolveUser(focused.UserID)
	if err != nil {
		return
	}
	s.profilePanel.SetUser(user)
}

func (s *ThreadScreen) profilePanelWidth() int {
	pw := s.width / 3
	if pw < 28 {
		pw = 28
	}
	if pw > 40 {
		pw = 40
	}
	return pw
}

func (s *ThreadScreen) mainWidth() int {
	if s.profileVisible {
		return s.width - s.profilePanelWidth()
	}
	return s.width
}

func (s *ThreadScreen) handleProfileKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"))):
		s.profileVisible = false
		s.recalcSizes()
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
		s.profilePanel.MoveDown()
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
		s.profilePanel.MoveUp()
		return s, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("y"))):
		val := s.profilePanel.FocusedValue()
		if val != "" {
			err := clipboard.WriteAll(val)
			if err != nil {
				s.statusBar.SetError("Failed to copy to clipboard")
			} else {
				s.statusBar.SetStatus("Copied to clipboard")
			}
		}
		return s, nil
	}

	return s, nil
}

func (s *ThreadScreen) handleNormalKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	if s.profileVisible {
		return s.handleProfileKey(msg)
	}

	var cmds []tea.Cmd
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"))):
		return s, func() tea.Msg { return GoBackMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("l"))):
		focused := s.messageList.FocusedMessage()
		if focused == nil {
			return s, nil
		}
		s.profileVisible = true
		s.recalcSizes()
		s.updateProfilePanel()
		return s, nil

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
	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f", "pgdown"))):
		s.messageList.PageDown()
		cmds = append(cmds, s.checkReadStatus())
	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b", "pgup"))):
		s.messageList.PageUp()
		cmds = append(cmds, s.checkReadStatus())
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		focused := s.messageList.FocusedMessage()
		if focused != nil {
			if urls := slack.ExtractURLs(focused.Text); len(urls) > 0 {
				_ = openBrowser(urls[0])
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

	case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
		s.focus = focusComposer
		return s, s.composer.Focus()
	}

	return s, tea.Batch(cmds...)
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
	mw := s.mainWidth()

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("33")).
		Padding(0, 1).
		Render(fmt.Sprintf("Thread in %s", s.channelName))

	headerBar := lipgloss.NewStyle().
		Width(mw).
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

	main := headerBar + "\n" +
		s.messageList.View() + "\n" +
		s.composer.View() + modeIndicator + "\n" +
		s.statusBar.View()

	if !s.profileVisible {
		return main
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, main, s.profilePanel.View())
}

func (s *ThreadScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.recalcSizes()
}

func (s *ThreadScreen) recalcSizes() {
	mw := s.mainWidth()
	composerHeight := 3
	if s.focus == focusComposer {
		composerHeight = 5
	}
	statusHeight := 1
	headerHeight := 2
	msgHeight := s.height - composerHeight - statusHeight - headerHeight
	if msgHeight < 3 {
		msgHeight = 3
	}
	s.messageList.SetSize(mw, msgHeight)
	s.composer.SetWidth(mw)
	s.statusBar.SetWidth(mw)
	if s.profileVisible {
		s.profilePanel.SetSize(s.profilePanelWidth(), s.height)
	}
}

func (s *ThreadScreen) ShortHelp() []key.Binding {
	if s.focus == focusComposer {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
			key.NewBinding(key.WithKeys("escape", "ctrl+["), key.WithHelp("esc", "cancel")),
		}
	}
	if s.profileVisible {
		return []key.Binding{
			key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "reply")),
			key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "navigate")),
			key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "close profile")),
			key.NewBinding(key.WithKeys("escape", "ctrl+["), key.WithHelp("esc", "close profile")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "reply")),
		key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "profile")),
		key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank")),
		key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open link")),
		key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"), key.WithHelp("esc/h", "back")),
	}
}
