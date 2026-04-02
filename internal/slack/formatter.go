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
	text = f.resolveGroupMentions(text)
	text = f.resolveSpecialMentions(text)
	text = f.resolveChannelLinks(text)
	text = f.resolveURLs(text)
	text = f.resolveEmoji(text)
	text = f.resolveHTMLEntities(text)

	// Use placeholders to protect code from other formatting
	var codes []string
	text = reCodeBlock.ReplaceAllStringFunc(text, func(match string) string {
		codes = append(codes, match)
		return fmt.Sprintf("___CODE_BLOCK_%d___", len(codes)-1)
	})
	text = reInlineCode.ReplaceAllStringFunc(text, func(match string) string {
		codes = append(codes, match)
		return fmt.Sprintf("___INLINE_CODE_%d___", len(codes)-1)
	})

	// Apply styles to non-code text
	text = f.resolveEnvironments(text)
	text = f.resolveAlertStatus(text)
	text = f.resolveBold(text)
	text = f.resolveItalic(text)
	text = f.resolveStrike(text)

	// Put code back with styling
	for i, code := range codes {
		var styled string
		if strings.HasPrefix(code, "```") {
			// Code block: background only, no extra padding
			content := reCodeBlock.FindStringSubmatch(code)[1]
			styled = "\x1b[48;5;235m\x1b[38;5;252m" + content + "\x1b[39m\x1b[49m"
			text = strings.Replace(text, fmt.Sprintf("___CODE_BLOCK_%d___", i), styled, 1)
		} else {
			// Inline code: background and slight padding
			content := reInlineCode.FindStringSubmatch(code)[1]
			styled = "\x1b[48;5;236m\x1b[38;5;252m " + content + " \x1b[39m\x1b[49m"
			text = strings.Replace(text, fmt.Sprintf("___INLINE_CODE_%d___", i), styled, 1)
		}
	}

	return text
}

// FormatTimestamp converts a Slack timestamp to a human-readable time with age
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
	var formatted string
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		formatted = t.Format(f.tsFormat)
	} else if t.Year() == now.Year() {
		formatted = t.Format("Jan 2 " + f.tsFormat)
	} else {
		formatted = t.Format("Jan 2, 2006 " + f.tsFormat)
	}

	return formatted + " (" + FormatAge(now.Sub(t)) + ")"
}

// FormatTimestampAge converts a Slack timestamp to a relative age string.
func (f *Formatter) FormatTimestampAge(ts string) string {
	parts := strings.SplitN(ts, ".", 2)
	if len(parts) == 0 {
		return ts
	}
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ts
	}
	return FormatAge(time.Since(time.Unix(sec, 0)))
}

// FormatAge returns a human-readable relative time string.
func FormatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	case d < 365*24*time.Hour:
		months := int(d.Hours() / 24 / 30)
		if months <= 1 {
			return "1mo ago"
		}
		return fmt.Sprintf("%dmo ago", months)
	default:
		years := int(d.Hours() / 24 / 365)
		if years == 1 {
			return "1y ago"
		}
		return fmt.Sprintf("%dy ago", years)
	}
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

func (f *Formatter) GetUser(userID string) *User {
	return f.cache.GetUser(userID)
}

var (
	reUserMention    = regexp.MustCompile(`<@(U[A-Z0-9]+)>`)
	reChannelLink    = regexp.MustCompile(`<#(C[A-Z0-9]+)\|([^>]+)>`)
	reURL            = regexp.MustCompile(`<(https?://[^|>]+)\|([^>]+)>`)
	reURLNoLabel     = regexp.MustCompile(`<(https?://[^>]+)>`)
	reEmojiShort     = regexp.MustCompile(`:([a-z0-9_+-]+):`)
	reSubteamLabel   = regexp.MustCompile(`<!subteam\^(S[A-Z0-9]+)\|@([^>]+)>`)
	reSubteamNoLabel = regexp.MustCompile(`<!subteam\^(S[A-Z0-9]+)>`)
	reSpecialMention = regexp.MustCompile(`<!(here|channel|everyone)(\|[^>]*)?>`)
	reBold2          = regexp.MustCompile(`(^|[\s(\[])\*\*([^\s\*](?:.*?[^\s\*])?)\*\*`)
	reBold1          = regexp.MustCompile(`(^|[\s(\[])\*([^\s\*](?:.*?[^\s\*])?)\*`)
	reItalic2        = regexp.MustCompile(`(^|[\s(\[])__([^\s_](?:.*?[^\s_])?)__`)
	reItalic1        = regexp.MustCompile(`(^|[\s(\[])_([^\s_](?:.*?[^\s_])?)_`)
	reStrike         = regexp.MustCompile(`(^|[\s(\[])~([^\s~](?:.*?[^\s~])?)~`)
	reInlineCode     = regexp.MustCompile("`([^`]+)`")
	reCodeBlock      = regexp.MustCompile("(?s)```(.+?)```")
	reEnvironments   = regexp.MustCompile(`(?i)staging|production`)
	reAlertStatus    = regexp.MustCompile(`RESOLVED|FIRING`)
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

func (f *Formatter) resolveGroupMentions(text string) string {
	// First handle subteam mentions with labels: <!subteam^S123|@handle>
	text = reSubteamLabel.ReplaceAllString(text, "@$2")
	// Then handle subteam mentions without labels: <!subteam^S123>
	text = reSubteamNoLabel.ReplaceAllStringFunc(text, func(match string) string {
		parts := reSubteamNoLabel.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		groupID := parts[1]
		if group := f.cache.GetUserGroup(groupID); group != nil {
			return "@" + group.Handle
		}
		return "@" + groupID
	})
	return text
}

func (f *Formatter) resolveSpecialMentions(text string) string {
	return reSpecialMention.ReplaceAllString(text, "@$1")
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

func (f *Formatter) resolveBold(text string) string {
	text = reBold2.ReplaceAllString(text, "$1\x1b[1m$2\x1b[22m")
	return reBold1.ReplaceAllString(text, "$1\x1b[1m$2\x1b[22m")
}

func (f *Formatter) resolveItalic(text string) string {
	text = reItalic2.ReplaceAllString(text, "$1\x1b[3m$2\x1b[23m")
	return reItalic1.ReplaceAllString(text, "$1\x1b[3m$2\x1b[23m")
}

func (f *Formatter) resolveStrike(text string) string {
	return reStrike.ReplaceAllString(text, "$1\x1b[9m$2\x1b[29m")
}

func (f *Formatter) resolveEnvironments(text string) string {
	indices := reEnvironments.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return text
	}

	var sb strings.Builder
	last := 0
	for _, idx := range indices {
		start, end := idx[0], idx[1]
		sb.WriteString(text[last:start])

		isStart := start == 0 || !isAlphanumeric(text[start-1])
		isEnd := end == len(text) || !isAlphanumeric(text[end])

		word := text[start:end]
		if isStart && isEnd {
			color := "81" // light blue for staging
			if strings.ToLower(word) == "production" {
				color = "166" // dark orange
			}
			sb.WriteString(fmt.Sprintf("\x1b[38;5;%sm%s\x1b[39m", color, word))
		} else {
			sb.WriteString(word)
		}
		last = end
	}
	sb.WriteString(text[last:])
	return sb.String()
}

func (f *Formatter) resolveAlertStatus(text string) string {
	indices := reAlertStatus.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return text
	}

	var sb strings.Builder
	last := 0
	for _, idx := range indices {
		start, end := idx[0], idx[1]
		sb.WriteString(text[last:start])

		isStart := start == 0 || !isAlphanumeric(text[start-1])
		isEnd := end == len(text) || !isAlphanumeric(text[end])

		word := text[start:end]
		if isStart && isEnd {
			upper := strings.ToUpper(word)
			if upper == "RESOLVED" {
				sb.WriteString("\x1b[32m● " + word + "\x1b[39m") // green dot + text
			} else {
				sb.WriteString("\x1b[31m● " + word + "\x1b[39m") // red dot + text
			}
		} else {
			sb.WriteString(word)
		}
		last = end
	}
	sb.WriteString(text[last:])
	return sb.String()
}

func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// ExtractURLs returns all URLs found in Slack mrkdwn text.
func ExtractURLs(text string) []string {
	var urls []string
	seen := make(map[string]bool)

	// <https://url|label> format
	for _, m := range reURL.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 && !seen[m[1]] {
			urls = append(urls, m[1])
			seen[m[1]] = true
		}
	}
	// <https://url> format (no label)
	for _, m := range reURLNoLabel.FindAllStringSubmatch(text, -1) {
		if len(m) >= 2 && !seen[m[1]] {
			urls = append(urls, m[1])
			seen[m[1]] = true
		}
	}
	return urls
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
		"heart":              "\u2764",
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
		"point_up":           "\u261D",
		"point_down":         "\U0001F447",
		"point_left":         "\U0001F448",
		"point_right":        "\U0001F449",
		"muscle":             "\U0001F4AA",
		"white_check_mark":   "\u2705",
		"heavy_check_mark":   "\u2714",
		"x":                  "\u274C",
		"warning":            "\u26A0",
		"question":           "\u2753",
		"exclamation":        "\u2757",
		"bulb":               "\U0001F4A1",
		"memo":               "\U0001F4DD",
		"wrench":             "\U0001F527",
		"gear":               "\u2699",
		"bug":                "\U0001F41B",
		"star":               "\u2B50",
		"sparkles":           "\u2728",
		"zap":               "\u26A1",
		"sunny":              "\u2600",
		"cloud":              "\u2601",
		"umbrella":           "\u2602",
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
		"scissors":           "\u2702",
		"hammer":             "\U0001F528",
		"hammer_and_wrench":  "\U0001F6E0",
		"hourglass":          "\u231B",
		"stopwatch":          "\u23F1",
		"alarm_clock":        "\u23F0",
		"calendar":           "\U0001F4C5",
		"pushpin":            "\U0001F4CC",
		"round_pushpin":      "\U0001F4CD",
		"mag":                "\U0001F50D",
		"bell":               "\U0001F514",
		"no_bell":            "\U0001F515",
		"speech_balloon":     "\U0001F4AC",
		"thought_balloon":    "\U0001F4AD",
		"arrow_up":           "\u2B06",
		"arrow_down":         "\u2B07",
		"arrow_left":         "\u2B05",
		"arrow_right":        "\u27A1",
		"heavy_plus_sign":    "\u2795",
		"heavy_minus_sign":   "\u2796",
		"wavy_dash":          "\u3030",
		"slightly_smiling_face": "\U0001F642",
		"upside_down_face":   "\U0001F643",
		"stuck_out_tongue":   "\U0001F61B",
	}
}
