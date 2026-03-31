package slack

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Formatter struct {
	cache    *Cache
	emojiMap map[string]string
	tsFormat string
}

func NewFormatter(cache *Cache, tsFormat string) *Formatter {
	return &Formatter{
		cache:    cache,
		emojiMap: defaultEmojiMap(),
		tsFormat: tsFormat,
	}
}

// Format converts Slack mrkdwn to plain styled text
func (f *Formatter) Format(text string) string {
	text = f.resolveUserMentions(text)
	text = f.resolveChannelLinks(text)
	text = f.resolveURLs(text)
	text = f.resolveEmoji(text)
	text = f.resolveHTMLEntities(text)
	return text
}

// FormatTimestamp converts a Slack timestamp to a human-readable time
func (f *Formatter) FormatTimestamp(ts string) string {
	parts := strings.SplitN(ts, ".", 2)
	if len(parts) == 0 {
		return ts
	}
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ts
	}
	t := time.Unix(sec, 0).Local()

	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format(f.tsFormat)
	}
	if t.Year() == now.Year() {
		return t.Format("Jan 2 " + f.tsFormat)
	}
	return t.Format("Jan 2, 2006 " + f.tsFormat)
}

// FormatEmoji converts a shortcode to unicode or returns :shortcode:
func (f *Formatter) FormatEmoji(name string) string {
	if emoji, ok := f.emojiMap[name]; ok {
		return emoji
	}
	return ":" + name + ":"
}

// FormatDate formats a Slack timestamp as a date string
func (f *Formatter) FormatDate(ts string) string {
	parts := strings.SplitN(ts, ".", 2)
	if len(parts) == 0 {
		return ""
	}
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ""
	}
	return time.Unix(sec, 0).Local().Format("Monday, January 2, 2006")
}

var (
	reUserMention   = regexp.MustCompile(`<@(U[A-Z0-9]+)>`)
	reChannelLink   = regexp.MustCompile(`<#(C[A-Z0-9]+)\|([^>]+)>`)
	reURL           = regexp.MustCompile(`<(https?://[^|>]+)\|([^>]+)>`)
	reURLNoLabel    = regexp.MustCompile(`<(https?://[^>]+)>`)
	reEmojiShort    = regexp.MustCompile(`:([a-z0-9_+-]+):`)
)

func (f *Formatter) resolveUserMentions(text string) string {
	return reUserMention.ReplaceAllStringFunc(text, func(match string) string {
		parts := reUserMention.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		userID := parts[1]
		if user := f.cache.GetUser(userID); user != nil {
			name := user.DisplayName
			if name == "" {
				name = user.Name
			}
			return fmt.Sprintf("@%s", name)
		}
		return fmt.Sprintf("@%s", userID)
	})
}

func (f *Formatter) resolveChannelLinks(text string) string {
	return reChannelLink.ReplaceAllString(text, "#$2")
}

func (f *Formatter) resolveURLs(text string) string {
	text = reURL.ReplaceAllString(text, "$2")
	text = reURLNoLabel.ReplaceAllString(text, "$1")
	return text
}

func (f *Formatter) resolveEmoji(text string) string {
	return reEmojiShort.ReplaceAllStringFunc(text, func(match string) string {
		parts := reEmojiShort.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		return f.FormatEmoji(parts[1])
	})
}

func (f *Formatter) resolveHTMLEntities(text string) string {
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	return text
}

func defaultEmojiMap() map[string]string {
	return map[string]string{
		"thumbsup":           "\U0001F44D",
		"+1":                 "\U0001F44D",
		"thumbsdown":         "\U0001F44E",
		"-1":                 "\U0001F44E",
		"heart":              "\u2764\uFE0F",
		"smile":              "\U0001F604",
		"laughing":           "\U0001F606",
		"grinning":           "\U0001F600",
		"joy":                "\U0001F602",
		"rofl":               "\U0001F923",
		"wink":               "\U0001F609",
		"blush":              "\U0001F60A",
		"thinking_face":      "\U0001F914",
		"eyes":               "\U0001F440",
		"fire":               "\U0001F525",
		"100":                "\U0001F4AF",
		"tada":               "\U0001F389",
		"rocket":             "\U0001F680",
		"wave":               "\U0001F44B",
		"pray":               "\U0001F64F",
		"clap":               "\U0001F44F",
		"raised_hands":       "\U0001F64C",
		"ok_hand":            "\U0001F44C",
		"point_up":           "\u261D\uFE0F",
		"point_down":         "\U0001F447",
		"point_left":         "\U0001F448",
		"point_right":        "\U0001F449",
		"muscle":             "\U0001F4AA",
		"white_check_mark":   "\u2705",
		"heavy_check_mark":   "\u2714\uFE0F",
		"x":                  "\u274C",
		"warning":            "\u26A0\uFE0F",
		"question":           "\u2753",
		"exclamation":        "\u2757",
		"bulb":               "\U0001F4A1",
		"memo":               "\U0001F4DD",
		"wrench":             "\U0001F527",
		"gear":               "\u2699\uFE0F",
		"bug":                "\U0001F41B",
		"star":               "\u2B50",
		"sparkles":           "\u2728",
		"zap":               "\u26A1",
		"sunny":              "\u2600\uFE0F",
		"cloud":              "\u2601\uFE0F",
		"umbrella":           "\u2602\uFE0F",
		"coffee":             "\u2615",
		"beer":               "\U0001F37A",
		"pizza":              "\U0001F355",
		"taco":               "\U0001F32E",
		"green_heart":        "\U0001F49A",
		"blue_heart":         "\U0001F499",
		"purple_heart":       "\U0001F49C",
		"broken_heart":       "\U0001F494",
		"skull":              "\U0001F480",
		"ghost":              "\U0001F47B",
		"robot_face":         "\U0001F916",
		"see_no_evil":        "\U0001F648",
		"hear_no_evil":       "\U0001F649",
		"speak_no_evil":      "\U0001F64A",
		"sob":                "\U0001F62D",
		"cry":                "\U0001F622",
		"angry":              "\U0001F620",
		"rage":               "\U0001F621",
		"sweat_smile":        "\U0001F605",
		"sweat":              "\U0001F613",
		"grimacing":          "\U0001F62C",
		"relieved":           "\U0001F60C",
		"unamused":           "\U0001F612",
		"disappointed":       "\U0001F61E",
		"confused":           "\U0001F615",
		"sleeping":           "\U0001F634",
		"sunglasses":         "\U0001F60E",
		"nerd_face":          "\U0001F913",
		"party_popper":       "\U0001F389",
		"confetti_ball":      "\U0001F38A",
		"balloon":            "\U0001F388",
		"gift":               "\U0001F381",
		"trophy":             "\U0001F3C6",
		"medal":              "\U0001F3C5",
		"crown":              "\U0001F451",
		"gem":                "\U0001F48E",
		"lock":               "\U0001F512",
		"key":                "\U0001F511",
		"link":               "\U0001F517",
		"paperclip":          "\U0001F4CE",
		"scissors":           "\u2702\uFE0F",
		"hammer":             "\U0001F528",
		"hammer_and_wrench":  "\U0001F6E0\uFE0F",
		"hourglass":          "\u231B",
		"stopwatch":          "\u23F1\uFE0F",
		"alarm_clock":        "\u23F0",
		"calendar":           "\U0001F4C5",
		"pushpin":            "\U0001F4CC",
		"round_pushpin":      "\U0001F4CD",
		"mag":                "\U0001F50D",
		"bell":               "\U0001F514",
		"no_bell":            "\U0001F515",
		"speech_balloon":     "\U0001F4AC",
		"thought_balloon":    "\U0001F4AD",
		"arrow_up":           "\u2B06\uFE0F",
		"arrow_down":         "\u2B07\uFE0F",
		"arrow_left":         "\u2B05\uFE0F",
		"arrow_right":        "\u27A1\uFE0F",
		"heavy_plus_sign":    "\u2795",
		"heavy_minus_sign":   "\u2796",
		"wavy_dash":          "\u3030\uFE0F",
		"slightly_smiling_face": "\U0001F642",
		"upside_down_face":   "\U0001F643",
		"stuck_out_tongue":   "\U0001F61B",
	}
}
