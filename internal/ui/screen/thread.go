package screen

import (
	"fmt"
	"log/slog"
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
	reactionPicker *component.ReactionPicker
	linkPicker     *component.LinkPicker
	pager          *component.Pager
	profileVisible bool
	client         *slack.Client
	formatter      *slack.Formatter
	channelID      string
	channelName    string
	threadTS       string
	readTimestamp  string
	parentMsg      slack.Message
	focus          chatFocus
	editingTS      string
	width          int
	height         int
}

func NewThreadScreen(client *slack.Client, formatter *slack.Formatter, channelID, channelName, threadTS string, parentMsg slack.Message, readTimestamp string) *ThreadScreen {
	s := &ThreadScreen{
		messageList:   component.NewMessageList(formatter, 80, 15),
		composer:      component.NewComposer(80),
		statusBar:     component.NewStatusBar(),
		profilePanel:  component.NewUserProfilePanel(),
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
func (s *ThreadScreen) InInsertMode() bool {
	return s.focus == focusComposer || s.reactionPicker != nil || s.linkPicker != nil || s.pager != nil
}

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

	case avatarResultMsg:
		if msg.userID == s.profilePanel.UserID() {
			s.profilePanel.SetAvatar(msg.avatar)
		}
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
					return threadErrorMsg{err: err}
				}
				// Refresh thread replies to show the reaction
				msgs, err := s.client.GetThreadReplies(s.channelID, s.threadTS)
				if err != nil {
					return threadErrorMsg{err: err}
				}
				return threadMessagesMsg{messages: msgs}
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

func (s *ThreadScreen) updateProfilePanel() tea.Cmd {
	if !s.profileVisible {
		return nil
	}
	focused := s.messageList.FocusedMessage()
	if focused == nil {
		return nil
	}
	user, err := s.client.ResolveUser(focused.UserID)
	if err != nil {
		slog.Error("thread profile resolve error", "user", focused.UserID, "error", err)
		return nil
	}
	slog.Debug("thread profile resolved", "name", user.Name, "email", user.Email)
	s.profilePanel.SetUser(user)

	if user.ImageURL != "" {
		avatarWidth := s.profilePanelWidth() - 5
		if avatarWidth > 16 {
			avatarWidth = 16
		}
		return fetchAvatarCmd(user.ID, user.ImageURL, avatarWidth)
	}
	return nil
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

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter", "l"))):
		if s.profilePanel.IsAvatarFocused() {
			if url := s.profilePanel.AvatarURL(); url != "" {
				_ = openBrowser(url)
			}
		}
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
	s.statusBar.SetStatus("")
	s.statusBar.SetError("")
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
		return s, s.updateProfilePanel()

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
		if focused == nil {
			return s, nil
		}
		urls := focused.AllURLs()
		if len(urls) == 1 {
			_ = openBrowser(urls[0])
			return s, nil
		}
		if len(urls) > 1 {
			picker := component.NewLinkPicker(focused, s.mainWidth()-10)
			s.linkPicker = &picker
			return s, nil
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
		threadTS := s.threadTS
		return s, func() tea.Msg {
			for _, r := range reactions {
				if allMine {
					_ = s.client.RemoveReaction(chID, ts, r.Name)
				} else if !r.HasMe {
					_ = s.client.AddReaction(chID, ts, r.Name)
				}
			}
			msgs, err := s.client.GetThreadReplies(chID, threadTS)
			if err != nil {
				return threadErrorMsg{err: err}
			}
			return threadMessagesMsg{messages: msgs}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
		s.focus = focusComposer
		s.editingTS = ""
		return s, s.composer.Focus()

	case key.Matches(msg, key.NewBinding(key.WithKeys("e", "H"))):
		focused := s.messageList.FocusedMessage()
		if focused == nil {
			return s, nil
		}
		
		isHistoryKey := msg.String() == "H"
		
		if focused.UserID == s.client.GetSelfID() && !isHistoryKey {
			s.focus = focusComposer
			s.editingTS = focused.Timestamp
			s.composer.SetValue(focused.Text)
			return s, s.composer.Focus()
		}
		
		// View mode (for others' messages or if H was pressed)
		content := s.formatter.Format(focused.Text)
		if len(focused.EditHistory) > 0 {
			var h strings.Builder
			h.WriteString(lipgloss.NewStyle().Bold(true).Underline(true).Render("Edit History:"))
			h.WriteString("\n\n")
			for i, edit := range focused.EditHistory {
				ts := s.formatter.FormatTimestamp(edit.Timestamp)
				h.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(fmt.Sprintf("[%s]", ts)))
				h.WriteString("\n")
				h.WriteString(s.formatter.Format(edit.Text))
				h.WriteString("\n\n")
				if i < len(focused.EditHistory)-1 {
					h.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("237")).Render(strings.Repeat("─", s.mainWidth()-4)))
					h.WriteString("\n\n")
				}
			}
			h.WriteString(lipgloss.NewStyle().Bold(true).Underline(true).Render("Current Version:"))
			h.WriteString("\n\n")
			h.WriteString(s.formatter.Format(focused.Text))
			content = h.String()
		}

		msgHeight := s.height - 2
		if msgHeight < 3 {
			msgHeight = 3
		}
		p := component.NewPager(content, s.mainWidth(), msgHeight)
		s.pager = &p
		return s, nil
	}

	return s, tea.Batch(cmds...)
}

func (s *ThreadScreen) handleComposerKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
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
					return threadErrorMsg{err: err}
				}
				msgs, err := s.client.GetThreadReplies(s.channelID, s.threadTS)
				if err != nil {
					return threadErrorMsg{err: err}
				}
				return threadMessagesMsg{messages: msgs}
			}
		}
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
		messageView = lipgloss.Place(mw, msgHeight, lipgloss.Center, lipgloss.Center, s.linkPicker.View())
	} else if s.reactionPicker != nil {
		msgHeight := s.height - 2
		if msgHeight < 3 {
			msgHeight = 3
		}
		messageView = lipgloss.Place(mw, msgHeight, lipgloss.Center, lipgloss.Center, s.reactionPicker.View())
	}

	s.recalcSizes()

	var main string
	if s.pager != nil {
		main = headerBar + "\n" + messageView
	} else if s.focus == focusComposer {
		main = headerBar + "\n" +
			messageView + "\n" +
			s.composer.View() + modeIndicator
	} else {
		main = headerBar + "\n" + messageView
	}

	if !s.profileVisible {
		return main
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, main, s.profilePanel.View())
}

func (s *ThreadScreen) StatusBarView() string {
	return s.statusBar.View()
}

func (s *ThreadScreen) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.recalcSizes()
}

func (s *ThreadScreen) SetStatusBarWidth(w int) { s.statusBar.SetWidth(w) }

func (s *ThreadScreen) recalcSizes() {
	mw := s.mainWidth()
	headerHeight := 2
	msgHeight := s.height - headerHeight
	if s.focus == focusComposer {
		msgHeight -= s.composer.Height()
	}
	if msgHeight < 3 {
		msgHeight = 3
	}
	s.messageList.SetSize(mw, msgHeight)
	s.composer.SetWidth(mw)
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
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit/view")),
		key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "history")),
		key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "profile")),
		key.NewBinding(key.WithKeys("r"), key.WithHelp("r/+", "react")),
		key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "react all")),
		key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank")),
		key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "navigate")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open link")),
		key.NewBinding(key.WithKeys("escape", "ctrl+[", "h"), key.WithHelp("esc/h", "back")),
	}
}
