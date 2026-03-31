package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Token  string
	TeamID string

	Polling  PollConfig
	Display  DisplayConfig
	Channels ChannelConfig
}

type PollConfig struct {
	ActiveChannel  time.Duration
	ChannelList    time.Duration
	Thread         time.Duration
	IdleMultiplier int
}

type DisplayConfig struct {
	TimestampFormat string
	DateSeparator   bool
	MessageLimit    int
	ShowBotMessages bool
	EmojiStyle      string
	Theme           string
}

type ChannelConfig struct {
	Pinned []string
	Hidden []string
	Types  []string
}

type fileConfig struct {
	Polling  filePollConfig  `toml:"polling"`
	Display  fileDisplay     `toml:"display"`
	Channels fileChannels    `toml:"channels"`
}

type filePollConfig struct {
	ActiveChannel  int `toml:"active_channel"`
	ChannelList    int `toml:"channel_list"`
	Thread         int `toml:"thread"`
	IdleMultiplier int `toml:"idle_multiplier"`
}

type fileDisplay struct {
	TimestampFormat string `toml:"timestamp_format"`
	DateSeparator   *bool  `toml:"date_separator"`
	MessageLimit    int    `toml:"message_limit"`
	ShowBotMessages *bool  `toml:"show_bot_messages"`
	EmojiStyle      string `toml:"emoji_style"`
	Theme           string `toml:"theme"`
}

type fileChannels struct {
	Pinned []string `toml:"pinned"`
	Hidden []string `toml:"hidden"`
	Types  []string `toml:"types"`
}

func defaults() Config {
	return Config{
		Polling: PollConfig{
			ActiveChannel:  3 * time.Second,
			ChannelList:    30 * time.Second,
			Thread:         5 * time.Second,
			IdleMultiplier: 2,
		},
		Display: DisplayConfig{
			TimestampFormat: "3:04 PM",
			DateSeparator:   true,
			MessageLimit:    50,
			ShowBotMessages: true,
			EmojiStyle:      "unicode",
			Theme:           "default",
		},
		Channels: ChannelConfig{
			Types: []string{"public_channel", "private_channel", "im", "mpim"},
		},
	}
}

func Load() (*Config, error) {
	cfg := defaults()

	cfg.Token = os.Getenv("SLACK_TOKEN")
	if cfg.Token == "" {
		return nil, fmt.Errorf("SLACK_TOKEN environment variable is required")
	}
	cfg.TeamID = os.Getenv("SLACK_TEAM")

	configPath := os.Getenv("LAZYSLACK_CONFIG")
	if configPath == "" {
		configDir, err := os.UserConfigDir()
		if err == nil {
			configPath = filepath.Join(configDir, "lazyslack", "config.toml")
		}
	}

	if configPath != "" {
		loadFromFile(&cfg, configPath)
	}

	return &cfg, nil
}

func loadFromFile(cfg *Config, path string) {
	var fc fileConfig
	_, err := toml.DecodeFile(path, &fc)
	if err != nil {
		return // silently ignore missing/invalid config file
	}

	if fc.Polling.ActiveChannel > 0 {
		cfg.Polling.ActiveChannel = time.Duration(fc.Polling.ActiveChannel) * time.Second
	}
	if fc.Polling.ChannelList > 0 {
		cfg.Polling.ChannelList = time.Duration(fc.Polling.ChannelList) * time.Second
	}
	if fc.Polling.Thread > 0 {
		cfg.Polling.Thread = time.Duration(fc.Polling.Thread) * time.Second
	}
	if fc.Polling.IdleMultiplier > 0 {
		cfg.Polling.IdleMultiplier = fc.Polling.IdleMultiplier
	}

	if fc.Display.TimestampFormat != "" {
		cfg.Display.TimestampFormat = fc.Display.TimestampFormat
	}
	if fc.Display.DateSeparator != nil {
		cfg.Display.DateSeparator = *fc.Display.DateSeparator
	}
	if fc.Display.MessageLimit > 0 {
		cfg.Display.MessageLimit = fc.Display.MessageLimit
	}
	if fc.Display.ShowBotMessages != nil {
		cfg.Display.ShowBotMessages = *fc.Display.ShowBotMessages
	}
	if fc.Display.EmojiStyle != "" {
		cfg.Display.EmojiStyle = fc.Display.EmojiStyle
	}
	if fc.Display.Theme != "" {
		cfg.Display.Theme = fc.Display.Theme
	}

	if len(fc.Channels.Pinned) > 0 {
		cfg.Channels.Pinned = fc.Channels.Pinned
	}
	if len(fc.Channels.Hidden) > 0 {
		cfg.Channels.Hidden = fc.Channels.Hidden
	}
	if len(fc.Channels.Types) > 0 {
		cfg.Channels.Types = fc.Channels.Types
	}
}
