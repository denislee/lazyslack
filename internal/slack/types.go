package slack

type Channel struct {
	ID          string
	Name        string
	IsIM        bool
	IsMPIM      bool
	IsPrivate   bool
	Topic       string
	Purpose     string
	UnreadCount int
	LastReadTS  string
	LatestTS    string
}

type Message struct {
	Timestamp  string
	UserID     string
	Username   string
	Text       string
	ThreadTS   string
	ReplyCount int
	Reactions  []Reaction
	Edited     bool
	Files      []File
	IsBot      bool
}

type Reaction struct {
	Name  string
	Count int
	Users []string
	HasMe bool
}

type User struct {
	ID          string
	Name        string
	DisplayName string
	IsBot       bool
	Presence    string // "active" or "away"
	StatusEmoji string
	StatusText  string
}

type File struct {
	Name     string
	URL      string
	Mimetype string
}

type UserGroup struct {
	ID     string
	Handle string
	Name   string
}

type Thread struct {
	ID            string // Unique key: channelID:threadTS
	ChannelID     string
	ChannelName   string
	ThreadTS      string
	Message       Message // Parent message
	LastReplyTS   string
	UnreadReplies int
}

type SearchResult struct {
	ChannelID   string
	ChannelName string
	IsIM        bool
	Message     Message
}
