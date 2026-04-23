package slack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Cache struct {
	mu         sync.RWMutex
	users      map[string]*User
	channels   map[string]*Channel
	messages   map[string][]Message
	threads    map[string][]Message // key: "channelID:threadTS"
	usergroups map[string]*UserGroup
}

func NewCache() *Cache {
	return &Cache{
		users:      make(map[string]*User),
		channels:   make(map[string]*Channel),
		messages:   make(map[string][]Message),
		threads:    make(map[string][]Message),
		usergroups: make(map[string]*UserGroup),
	}
}

func getCachePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		// Fallback to config dir or tmp if cache dir isn't available
		cacheDir = os.TempDir()
	}
	dir := filepath.Join(cacheDir, "lazyslack")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "channels.json")
}

func (c *Cache) LoadChannelsFromDisk() ([]Channel, error) {
	data, err := os.ReadFile(getCachePath())
	if err != nil {
		return nil, err
	}
	var channels []Channel
	if err := json.Unmarshal(data, &channels); err != nil {
		return nil, err
	}
	c.SetChannels(channels)
	return channels, nil
}

func (c *Cache) SaveChannelsToDisk(channels []Channel) error {
	data, err := json.Marshal(channels)
	if err != nil {
		return err
	}
	return os.WriteFile(getCachePath(), data, 0600)
}

func getMessagesCachePath(channelID string) string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	dir := filepath.Join(cacheDir, "lazyslack", "messages")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, channelID+".json")
}

func (c *Cache) LoadMessagesFromDisk(channelID string) ([]Message, error) {
	data, err := os.ReadFile(getMessagesCachePath(channelID))
	if err != nil {
		return nil, err
	}
	var messages []Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	c.SetMessages(channelID, messages)
	return messages, nil
}

func (c *Cache) SaveMessagesToDisk(channelID string, messages []Message) error {
	data, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	return os.WriteFile(getMessagesCachePath(channelID), data, 0600)
}

func (c *Cache) GetUser(id string) *User {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.users[id]
}

func (c *Cache) SetUser(user *User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.users[user.ID] = user
}

func (c *Cache) SetChannels(channels []Channel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range channels {
		id := channels[i].ID
		newCh := channels[i]
		if existing, ok := c.channels[id]; ok {
			// Keep the newest timestamp we've seen
			if existing.LatestTS > newCh.LatestTS {
				newCh.LatestTS = existing.LatestTS
			}
			// conversations.list doesn't reliably populate unread fields. If
			// the fresh entry lacks LastReadTS, trust whatever conversations.info
			// last wrote into the cache instead of clobbering it with zeros.
			if newCh.LastReadTS == "" && existing.LastReadTS != "" {
				newCh.UnreadCount = existing.UnreadCount
				newCh.LastReadTS = existing.LastReadTS
			}
		}
		c.channels[id] = &newCh
	}
}

func (c *Cache) GetChannel(id string) *Channel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.channels[id]
}

// SetChannelUnread overwrites the unread-tracking fields for a channel under
// the write lock. Used by enrichment paths that fetch authoritative values
// from conversations.info.
func (c *Cache) SetChannelUnread(id string, unreadCount int, lastReadTS, latestTS string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	existing, ok := c.channels[id]
	if !ok {
		return
	}
	updated := *existing
	updated.UnreadCount = unreadCount
	updated.LastReadTS = lastReadTS
	if latestTS != "" {
		updated.LatestTS = latestTS
	}
	c.channels[id] = &updated
}

// AdvanceChannelLatestTS bumps a channel's LatestTS under the write lock, but
// only if the incoming timestamp is newer. Returns true when the cache was
// actually updated so the caller can skip unnecessary UI refreshes.
func (c *Cache) AdvanceChannelLatestTS(id, ts string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	existing, ok := c.channels[id]
	if !ok || ts <= existing.LatestTS {
		return false
	}
	updated := *existing
	updated.LatestTS = ts
	c.channels[id] = &updated
	return true
}

func (c *Cache) GetAllChannels() []Channel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	channels := make([]Channel, 0, len(c.channels))
	for _, ch := range c.channels {
		channels = append(channels, *ch)
	}
	return channels
}

func (c *Cache) SetMessages(channelID string, msgs []Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages[channelID] = msgs
}

func (c *Cache) GetMessages(channelID string) []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.messages[channelID]
}

func (c *Cache) SetThread(channelID, threadTS string, msgs []Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.threads[channelID+":"+threadTS] = msgs
}

func (c *Cache) GetThread(channelID, threadTS string) []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.threads[channelID+":"+threadTS]
}

func (c *Cache) SetUserGroups(groups []UserGroup) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range groups {
		c.usergroups[groups[i].ID] = &groups[i]
	}
}

func (c *Cache) GetUserGroup(id string) *UserGroup {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.usergroups[id]
}
