package provider

import (
	"regexp"
	"strings"
)

// Context tags wrap data the agent adds in front of a user message:
//
//   - <attached> lists uploaded file paths so the model can read them with its
//     tools; clients show them as chips instead of raw paths.
//   - <userinfo> carries the per-turn channel context (platform, room, ids,
//     speaker, time); clients hide it entirely.
//
// Both are stored with the message so the transcript stays byte-stable for
// provider prefix caching.
const (
	AttachmentBlockOpen  = "<attached>"
	AttachmentBlockClose = "</attached>"
	UserInfoBlockOpen    = "<userinfo>"
	UserInfoBlockClose   = "</userinfo>"
)

var (
	attachmentBlockPattern = regexp.MustCompile(`(?s)<attached>.*?</attached>[ \t]*\n?`)
	userInfoBlockPattern   = regexp.MustCompile(`(?s)<userinfo>.*?</userinfo>[ \t]*\n?`)
	leadingContextPattern  = regexp.MustCompile(`(?s)^\s*(<userinfo>.*?</userinfo>|<attached>.*?</attached>)`)
)

// StripAttachmentBlock removes the attachment block from message text so
// derived text, such as a conversation title or a resumed transcript, only
// shows what the user actually typed.
func StripAttachmentBlock(text string) string {
	if !strings.Contains(text, AttachmentBlockOpen) {
		return text
	}
	return strings.TrimSpace(attachmentBlockPattern.ReplaceAllString(text, ""))
}

// StripContextBlocks removes every block the agent adds for the model. Use it
// wherever a message is shown to a person: titles, previews, turn labels, and the
// CLI.
func StripContextBlocks(text string) string {
	if !strings.Contains(text, UserInfoBlockOpen) &&
		!strings.Contains(text, AttachmentBlockOpen) &&
		!HasTriggerBlock(text) &&
		!strings.Contains(text, SelectionsBlockOpen) {
		return text
	}
	// Selections go first: a quoted reply may itself contain a word that looks
	// like another context tag, and that text must disappear with the quote.
	if selectionItemPattern.MatchString(text) {
		text = selectionsBlockPattern.ReplaceAllString(text, "")
	}
	text = userInfoBlockPattern.ReplaceAllString(text, "")
	text = attachmentBlockPattern.ReplaceAllString(text, "")
	text = StripTriggerBlocks(text)
	return strings.TrimSpace(text)
}

// SplitContextBlocks separates the leading <userinfo>/<attached> blocks from the
// text the person typed. The blocks are returned in their original order, so an
// edit can put them back exactly as they were.
func SplitContextBlocks(text string) (blocks string, body string) {
	rest := text
	kept := make([]string, 0, 2)
	for {
		match := leadingContextPattern.FindStringSubmatch(rest)
		if match == nil {
			break
		}
		kept = append(kept, strings.TrimSpace(match[1]))
		rest = rest[len(match[0]):]
	}
	return strings.Join(kept, "\n\n"), strings.TrimLeft(rest, "\n")
}

// JoinContextBlocks puts the blocks back in front of the message body.
func JoinContextBlocks(blocks, body string) string {
	blocks = strings.TrimSpace(blocks)
	body = strings.TrimLeft(body, "\n")
	switch {
	case blocks == "":
		return body
	case body == "":
		return blocks
	default:
		return blocks + "\n\n" + body
	}
}
