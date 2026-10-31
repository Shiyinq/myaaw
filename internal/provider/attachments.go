package provider

import (
	"regexp"
	"strings"
)

// Attachment tags wrap the file paths prepended to a user message. The model
// reads the files itself; clients show them as chips instead of raw paths.
const (
	AttachmentBlockOpen  = "<attached>"
	AttachmentBlockClose = "</attached>"
)

var attachmentBlockPattern = regexp.MustCompile(`(?s)<attached>.*?</attached>[ \t]*\n?`)

// StripAttachmentBlock removes the attachment block from message text so
// derived text, such as a conversation title or a resumed transcript, only
// shows what the user actually typed.
func StripAttachmentBlock(text string) string {
	if !strings.Contains(text, AttachmentBlockOpen) {
		return text
	}
	return strings.TrimSpace(attachmentBlockPattern.ReplaceAllString(text, ""))
}
