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
	StatusEmoji string
	StatusText  string
}

type File struct {
	Name     string
	URL      string
	Mimetype string
}

type SearchResult struct {
	ChannelID   string
	ChannelName string
	Message     Message
}
