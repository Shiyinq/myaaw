package telegram

import "encoding/json"

// HasChatContent reports whether an update carries something the agent can
// process: text, a photo, a document, or a voice message.
//
// Telegram also delivers service messages (topic created, member joined, group
// renamed, pinned message, ...) and membership updates that have no content.
// Passing those to the agent makes it answer an empty prompt, which is how the
// bot ended up replying "Masih kosong..." after a topic was created.
func HasChatContent(payload json.RawMessage) bool {
	var update struct {
		Message *struct {
			Text     string    `json:"text"`
			Photo    []Photo   `json:"photo"`
			Document *Document `json:"document"`
			Voice    *Voice    `json:"voice"`
		} `json:"message"`
	}
	if err := json.Unmarshal(payload, &update); err != nil {
		return false
	}
	if update.Message == nil {
		// Membership, reaction, and edited-message updates are ignored: the
		// adapter only understands the "message" shape.
		return false
	}

	m := update.Message
	if m.Voice != nil || len(m.Photo) > 0 || m.Document != nil {
		return true
	}
	return m.Text != ""
}
