package agent

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"myaaw/internal/channel"
)

// contextNameLimit matches the longest name the platforms allow (Telegram group
// titles and topic names are capped at 128; Discord channel names at 100), so a
// real name is never shortened. It only guards against pathological input.
const contextNameLimit = 128

// RenderUserInfo builds the <userinfo> block prepended to a user message.
//
// Everything in it is per-turn or per-speaker data: the platform, the room
// (kind, name, audience), the platform ids, the speaker, and the time. It lives
// in the user message rather than the system prompt so the provider's prefix
// cache can reuse the system prompt and the whole conversation history; the
// block itself is written once and stored with the message.
func RenderUserInfo(msg *channel.IncomingMessage, now time.Time) string {
	if msg == nil {
		return ""
	}
	line := userInfoLine(msg, now)
	if line == "" {
		return ""
	}
	return "<userinfo>\n" + line + "\n</userinfo>"
}

func userInfoLine(msg *channel.IncomingMessage, now time.Time) string {
	platform := platformLabel(msg.Channel)
	if platform == "" {
		// No platform means an internal or test message; there is nothing to say.
		return ""
	}
	parts := make([]string, 0, 6)
	parts = append(parts, "Platform: "+platform)
	if room := roomLabel(msg); room != "" {
		parts = append(parts, "Chat: "+room)
	}
	if msg.ChatID != "" && chatIDVisible(msg) {
		parts = append(parts, "ChatID: "+sanitizeContext(msg.ChatID))
	}
	if msg.TopicID != "" {
		parts = append(parts, "TopicID: "+sanitizeContext(msg.TopicID))
	}
	if speaker := speakerLabel(msg); speaker != "" {
		parts = append(parts, "Speaker: "+speaker)
	}
	if msg.UserID > 0 {
		// The id that used to live in the system prompt. On API turns the control
		// plane injects it from the authenticated token.
		parts = append(parts, fmt.Sprintf("UserID: %d", msg.UserID))
	}
	if session := strings.TrimSpace(msg.ConversationID); session != "" && session != "NEW" {
		parts = append(parts, "Session: "+sanitizeContext(session))
	}
	parts = append(parts, "Current time: "+now.UTC().Format("2006-01-02 15:04:05")+" UTC")

	// No line-level truncation: names are bounded at the source, and ids, the
	// session, and the clock must always arrive complete.
	return sanitizeContext(strings.Join(parts, " · "))
}

// platformLabel turns a channel name into the label the model reads.
func platformLabel(channelName string) string {
	switch strings.ToLower(strings.TrimSpace(channelName)) {
	case "telegram":
		return "Telegram"
	case "discord":
		return "Discord"
	case "cli":
		return "CLI"
	case "api":
		return "API"
	case "a2a":
		return "A2A"
	case "subagent":
		return "Sub-agent"
	case "voice-classic":
		return "Voice"
	case "":
		return ""
	default:
		return truncateRunes(sanitizeContext(channelName), contextNameLimit)
	}
}

// roomLabel describes where the turn happened: kind, space name, and audience.
// Local and API sessions have no room, so they render no Chat line.
func roomLabel(msg *channel.IncomingMessage) string {
	kind := chatKind(msg)
	if kind == "" {
		return ""
	}
	space := spaceLabel(msg)
	audience := audienceLabel(msg)

	switch {
	case space != "" && audience != "":
		return space + " (" + kind + ", " + audience + ")"
	case space != "":
		return space + " (" + kind + ")"
	case audience != "":
		return kind + " (" + audience + ")"
	default:
		return kind
	}
}

func chatKind(msg *channel.IncomingMessage) string {
	chatType := strings.ToLower(strings.TrimSpace(msg.ChatType))
	switch strings.ToLower(strings.TrimSpace(msg.Channel)) {
	case "telegram":
		switch chatType {
		case "private":
			return "direct message"
		case "group":
			return "group"
		case "supergroup":
			if msg.IsThread {
				return "forum topic"
			}
			if msg.IsForum {
				return "group with topics"
			}
			return "group"
		}
		if msg.IsThread {
			return "forum topic"
		}
		return ""
	case "discord":
		switch chatType {
		case "dm":
			return "direct message"
		case "guild", "":
			if msg.IsThread {
				return "thread"
			}
			return "server channel"
		}
		return ""
	case "cli":
		return "local session"
	case "api":
		return "API session"
	case "a2a":
		return "agent-to-agent"
	case "subagent":
		return "internal run"
	}
	return ""
}

// spaceLabel is the human name of the room, e.g. "Myaaw Server › #general".
func spaceLabel(msg *channel.IncomingMessage) string {
	chatName := truncateRunes(sanitizeContext(msg.ChatName), contextNameLimit)
	topicName := truncateRunes(sanitizeContext(msg.TopicName), contextNameLimit)

	switch strings.ToLower(strings.TrimSpace(msg.Channel)) {
	case "telegram", "discord":
		switch {
		case chatName != "" && topicName != "":
			return chatName + " › " + topicName
		case topicName != "":
			return topicName
		default:
			return chatName
		}
	}
	return ""
}

// audienceLabel reports whether other people can read the answer.
func audienceLabel(msg *channel.IncomingMessage) string {
	if msg.Scope != "" {
		return "shared"
	}
	switch strings.ToLower(strings.TrimSpace(msg.Channel)) {
	case "telegram":
		if strings.EqualFold(strings.TrimSpace(msg.ChatType), "private") {
			return "only this user"
		}
	case "discord":
		if strings.EqualFold(strings.TrimSpace(msg.ChatType), "dm") {
			return "only this user"
		}
	}
	return ""
}

// chatIDVisible keeps a Telegram DM from repeating the user id: chat.id and the
// sender id are the same value there.
func chatIDVisible(msg *channel.IncomingMessage) bool {
	if strings.EqualFold(strings.TrimSpace(msg.Channel), "telegram") {
		return !strings.EqualFold(strings.TrimSpace(msg.ChatType), "private")
	}
	return true
}

func speakerLabel(msg *channel.IncomingMessage) string {
	switch strings.ToLower(strings.TrimSpace(msg.Channel)) {
	case "cli", "api", "subagent":
		return ""
	}

	name := truncateRunes(sanitizeContext(msg.SenderName), contextNameLimit)
	handle := truncateRunes(sanitizeContext(msg.SenderUsername), contextNameLimit)
	if handle != "" {
		handle = "@" + strings.TrimPrefix(handle, "@")
	}

	switch {
	case name != "" && handle != "":
		return name + " (" + handle + ")"
	case name != "":
		return name
	case handle != "":
		return handle
	default:
		return ""
	}
}

// sanitizeContext keeps a platform-provided name on one line. Channel and user
// names are untrusted input, not markup.
func sanitizeContext(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	lastSpace := false
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		// A room or user name must never be able to close the <userinfo> tag
		// early and turn the rest of its name into prompt text.
		if r == '<' || r == '>' {
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
