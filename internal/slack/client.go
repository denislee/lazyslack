package slack

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	slackapi "github.com/slack-go/slack"
)

var slackErrors = map[string]string{
	"restricted_action_read_only_channel": "This channel is read-only",
	"restricted_action":                   "You don't have permission to do this",
	"channel_not_found":                   "Channel not found",
	"not_in_channel":                      "You're not in this channel",
	"is_archived":                         "This channel is archived",
	"msg_too_long":                        "Message is too long",
	"no_text":                             "Message cannot be empty",
	"rate_limited":                        "Rate limited — try again shortly",
	"invalid_auth":                        "Invalid auth token",
	"account_inactive":                    "Account is inactive",
	"token_revoked":                       "Token has been revoked",
	"not_authed":                          "Not authenticated",
	"already_reacted":                     "You already reacted with this emoji",
	"no_reaction":                         "You haven't reacted with this emoji",
	"too_many_reactions":                  "Too many reactions on this message",
}

func friendlyError(err error) error {
	var slackErr slackapi.SlackErrorResponse
	if errors.As(err, &slackErr) {
		if msg, ok := slackErrors[slackErr.Err]; ok {
			return fmt.Errorf("%s", msg)
		}
	}
	// Also check for plain string errors from the SDK
	errStr := err.Error()
	if msg, ok := slackErrors[errStr]; ok {
		return fmt.Errorf("%s", msg)
	}
	return err
}

type Client struct {
	api    *slackapi.Client
	cache  *Cache
	selfID string
}

func NewClient(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}
	api := slackapi.New(token)
	return &Client{
		api:   api,
		cache: NewCache(),
	}, nil
}

func (c *Client) AuthTest() (string, error) {
	resp, err := c.api.AuthTest()
	if err != nil {
		return "", fmt.Errorf("auth test failed: %w", err)
	}
	return resp.UserID, nil
}

func (c *Client) SetSelfID(id string) {
	c.selfID = id
}

func (c *Client) GetSelfID() string {
	return c.selfID
}

func (c *Client) Cache() *Cache {
	return c.cache
}

func (c *Client) GetChannels(types []string) ([]Channel, error) {
	slog.Debug("GetChannels starting", "types", types)
	var allChannels []Channel
	cursor := ""
	page := 0

	for {
		page++
		params := &slackapi.GetConversationsParameters{
			Types:           types,
			ExcludeArchived: true,
			Limit:           200,
			Cursor:          cursor,
		}
		channels, nextCursor, err := c.api.GetConversations(params)
		if err != nil {
			slog.Error("GetChannels API error", "page", page, "error", err)
			return nil, fmt.Errorf("get conversations: %w", err)
		}
		slog.Debug("GetChannels page fetched", "page", page, "count", len(channels))

		for _, ch := range channels {
			name := ch.Name
			if ch.IsIM {
				user, err := c.ResolveUser(ch.User)
				if err == nil {
					name = user.DisplayName
					if name == "" {
						name = user.Name
					}
				} else {
					name = ch.User
				}
			}

			allChannels = append(allChannels, Channel{
				ID:        ch.ID,
				Name:      name,
				IsIM:      ch.IsIM,
				IsMPIM:    ch.IsMpIM,
				IsPrivate: ch.IsPrivate,
				Topic:     ch.Topic.Value,
				Purpose:   ch.Purpose.Value,
			})
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	slog.Info("GetChannels list fetched", "total", len(allChannels), "pages", page)

	// conversations.list doesn't reliably return unread counts.
	// Enrich with conversations.info which always includes them.
	c.enrichWithUnreadCounts(allChannels)

	unreadCount := 0
	for _, ch := range allChannels {
		if ch.UnreadCount > 0 {
			unreadCount++
		}
	}
	slog.Info("GetChannels done", "total", len(allChannels), "with_unread", unreadCount)

	c.cache.SetChannels(allChannels)
	_ = c.cache.SaveChannelsToDisk(allChannels) // Best-effort caching
	return allChannels, nil
}

// enrichWithUnreadCounts calls conversations.info for each channel to get
// reliable unread counts. Uses concurrent workers with a semaphore.
// The slack-go library handles rate-limit retries automatically.
func (c *Client) enrichWithUnreadCounts(channels []Channel) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // 10 concurrent workers

	for i := range channels {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			info, err := c.api.GetConversationInfo(&slackapi.GetConversationInfoInput{
				ChannelID: channels[idx].ID,
			})
			if err != nil {
				slog.Debug("conversations.info error", "channel", channels[idx].Name, "error", err)
				return
			}
			channels[idx].UnreadCount = info.UnreadCountDisplay
			channels[idx].LastReadTS = info.LastRead
		}(i)
	}
	wg.Wait()
}

func (c *Client) GetMessages(channelID string, limit int, oldest string) ([]Message, error) {
	params := &slackapi.GetConversationHistoryParameters{
		ChannelID: channelID,
		Limit:     limit,
	}
	if oldest != "" {
		params.Oldest = oldest
	}

	history, err := c.api.GetConversationHistory(params)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}

	messages := make([]Message, 0, len(history.Messages))
	for _, msg := range history.Messages {
		messages = append(messages, c.convertMessage(msg))
	}

	// Slack returns newest first, reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	c.cache.SetMessages(channelID, messages)
	return messages, nil
}

func (c *Client) GetThreadReplies(channelID, threadTS string) ([]Message, error) {
	msgs, _, _, err := c.api.GetConversationReplies(&slackapi.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTS,
	})
	if err != nil {
		return nil, fmt.Errorf("get replies: %w", err)
	}

	replies := make([]Message, 0, len(msgs))
	for _, msg := range msgs {
		replies = append(replies, c.convertMessage(msg))
	}

	c.cache.SetThread(channelID, threadTS, replies)
	return replies, nil
}

func (c *Client) SendMessage(channelID, text string) error {
	_, _, err := c.api.PostMessage(
		channelID,
		slackapi.MsgOptionText(text, false),
	)
	if err != nil {
		return fmt.Errorf("send message: %w", friendlyError(err))
	}
	return nil
}

func (c *Client) SendThreadReply(channelID, threadTS, text string) error {
	_, _, err := c.api.PostMessage(
		channelID,
		slackapi.MsgOptionText(text, false),
		slackapi.MsgOptionTS(threadTS),
	)
	if err != nil {
		return fmt.Errorf("send reply: %w", friendlyError(err))
	}
	return nil
}

func (c *Client) AddReaction(channelID, timestamp, emoji string) error {
	ref := slackapi.ItemRef{
		Channel:   channelID,
		Timestamp: timestamp,
	}
	if err := c.api.AddReaction(emoji, ref); err != nil {
		return friendlyError(err)
	}
	return nil
}

func (c *Client) RemoveReaction(channelID, timestamp, emoji string) error {
	ref := slackapi.ItemRef{
		Channel:   channelID,
		Timestamp: timestamp,
	}
	if err := c.api.RemoveReaction(emoji, ref); err != nil {
		return friendlyError(err)
	}
	return nil
}

func (c *Client) MarkChannel(channelID, ts string) error {
	if err := c.api.MarkConversation(channelID, ts); err != nil {
		return fmt.Errorf("mark channel: %w", friendlyError(err))
	}
	return nil
}

func (c *Client) Search(query string) ([]SearchResult, error) {
	params := slackapi.SearchParameters{
		Sort:          "timestamp",
		SortDirection: "desc",
		Count:         20,
	}
	msgs, err := c.api.SearchMessages(query, params)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	results := make([]SearchResult, 0, len(msgs.Matches))
	for _, match := range msgs.Matches {
		channelName := match.Channel.Name
		isIM := strings.HasPrefix(match.Channel.ID, "D")
		// DM channel names from search are user IDs — resolve to display names
		if isIM && strings.HasPrefix(channelName, "U") {
			if user, err := c.ResolveUser(channelName); err == nil {
				channelName = user.DisplayName
				if channelName == "" {
					channelName = user.Name
				}
			}
		}
		results = append(results, SearchResult{
			ChannelID:   match.Channel.ID,
			ChannelName: channelName,
			IsIM:        isIM,
			Message: Message{
				Timestamp: match.Timestamp,
				UserID:    match.User,
				Username:  match.Username,
				Text:      match.Text,
			},
		})
	}
	return results, nil
}

func (c *Client) ResolveUser(userID string) (*User, error) {
	if user := c.cache.GetUser(userID); user != nil {
		return user, nil
	}

	info, err := c.api.GetUserInfo(userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	user := &User{
		ID:          info.ID,
		Name:        info.Name,
		DisplayName: info.Profile.DisplayName,
		IsBot:       info.IsBot,
		StatusEmoji: info.Profile.StatusEmoji,
		StatusText:  info.Profile.StatusText,
	}
	if user.DisplayName == "" {
		user.DisplayName = info.RealName
	}

	c.cache.SetUser(user)
	return user, nil
}

func (c *Client) GetUserGroups() ([]UserGroup, error) {
	groups, err := c.api.GetUserGroups()
	if err != nil {
		return nil, fmt.Errorf("get usergroups: %w", err)
	}

	result := make([]UserGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, UserGroup{
			ID:     g.ID,
			Handle: g.Handle,
			Name:   g.Name,
		})
	}
	c.cache.SetUserGroups(result)
	return result, nil
}

func (c *Client) convertMessage(msg slackapi.Message) Message {
	username := msg.Username
	if username == "" {
		if user, err := c.ResolveUser(msg.User); err == nil {
			username = user.DisplayName
			if username == "" {
				username = user.Name
			}
		} else {
			username = msg.User
		}
	}

	reactions := make([]Reaction, 0, len(msg.Reactions))
	for _, r := range msg.Reactions {
		hasMe := false
		for _, u := range r.Users {
			if u == c.selfID {
				hasMe = true
				break
			}
		}
		reactions = append(reactions, Reaction{
			Name:  r.Name,
			Count: r.Count,
			Users: r.Users,
			HasMe: hasMe,
		})
	}

	files := make([]File, 0, len(msg.Files))
	for _, f := range msg.Files {
		files = append(files, File{
			Name:     f.Name,
			URL:      f.URLPrivate,
			Mimetype: f.Mimetype,
		})
	}

	text := msg.Text
	if text == "" {
		text = c.extractAttachmentText(msg.Attachments)
	}

	return Message{
		Timestamp:  msg.Timestamp,
		UserID:     msg.User,
		Username:   username,
		Text:       text,
		ThreadTS:   msg.ThreadTimestamp,
		ReplyCount: msg.ReplyCount,
		Reactions:  reactions,
		Edited:     msg.Edited != nil,
		Files:      files,
		IsBot:      msg.BotID != "",
	}
}

func (c *Client) extractAttachmentText(attachments []slackapi.Attachment) string {
	var parts []string
	for _, a := range attachments {
		var lines []string
		if a.Pretext != "" {
			lines = append(lines, a.Pretext)
		}
		if a.Title != "" {
			lines = append(lines, a.Title)
		}
		if a.Text != "" {
			lines = append(lines, a.Text)
		}
		if len(lines) > 0 {
			parts = append(parts, strings.Join(lines, "\n"))
		}
	}
	return strings.Join(parts, "\n\n")
}
