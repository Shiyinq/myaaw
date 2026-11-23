package provider

import (
	"encoding/json"
	"regexp"
	"strings"
)

// InteractionBlockOpen and InteractionBlockClose frame structured interaction
// metadata (button clicks, form submissions, quick replies, checklists) carried
// inside a user message so the model gets exact machine-readable values while
// the WebUI renders a dedicated interaction badge (like cron/subagent).
const (
	InteractionBlockOpen  = "<interaction"
	InteractionBlockClose = "</interaction>"
)

// Known interaction kinds used in <interaction kind="...">.
const (
	InteractionButton     = "button"
	InteractionQuickReply = "quick_reply"
	InteractionForm       = "form"
	InteractionChecklist  = "checklist"
)

// ExtraMessage is one batch of rich UI blocks emitted by the `extra_messages`
// built-in tool. It is presentation metadata persisted with the transcript so
// a reopened chat renders the same interactive/display blocks and their states,
// and is never sent raw to LLM providers.
type ExtraMessage struct {
	ID        string       `json:"id" bson:"id"`
	Blocks    []ExtraBlock `json:"blocks" bson:"blocks"`
	CreatedAt string       `json:"created_at,omitempty" bson:"created_at,omitempty"`
}

// ExtraBlock is one display or interactive UI block inside an ExtraMessage.
type ExtraBlock struct {
	ID          string            `json:"id" bson:"id"`
	Type        string            `json:"type" bson:"type"`
	Interactive bool              `json:"interactive,omitempty" bson:"interactive,omitempty"`
	SingleUse   *bool             `json:"single_use,omitempty" bson:"single_use,omitempty"`
	Data        json.RawMessage   `json:"data" bson:"data"`
	State       *InteractionState `json:"state,omitempty" bson:"state,omitempty"`
}

// IsSingleUse reports whether an interactive block locks after its first
// submission. Interactive blocks default to single-use unless explicitly set
// to false.
func (b ExtraBlock) IsSingleUse() bool {
	if b.SingleUse == nil {
		return b.Interactive
	}
	return *b.SingleUse
}

// InteractionState persists whether an interactive block has been answered so
// page reloads and concurrent tabs keep it locked and show the submitted result.
type InteractionState struct {
	Status     string         `json:"status" bson:"status"` // "pending" | "submitted"
	AnsweredAt string         `json:"answered_at,omitempty" bson:"answered_at,omitempty"`
	Actor      string         `json:"actor,omitempty" bson:"actor,omitempty"`
	Summary    string         `json:"summary,omitempty" bson:"summary,omitempty"`
	Payload    map[string]any `json:"payload,omitempty" bson:"payload,omitempty"`
}

// CloneExtraMessages returns a deep copy of ExtraMessages so concurrent
// checkpoints and UI serialization never alias mutable slices or raw JSON.
func CloneExtraMessages(src []ExtraMessage) []ExtraMessage {
	if len(src) == 0 {
		return nil
	}
	out := make([]ExtraMessage, len(src))
	for i, msg := range src {
		msg.Blocks = cloneExtraBlocks(msg.Blocks)
		out[i] = msg
	}
	return out
}

func cloneExtraBlocks(src []ExtraBlock) []ExtraBlock {
	if len(src) == 0 {
		return nil
	}
	out := make([]ExtraBlock, len(src))
	for i, block := range src {
		if block.SingleUse != nil {
			v := *block.SingleUse
			block.SingleUse = &v
		}
		if block.Data != nil {
			block.Data = append(json.RawMessage(nil), block.Data...)
		}
		if block.State != nil {
			st := *block.State
			if block.State.Payload != nil {
				if raw, err := json.Marshal(block.State.Payload); err == nil {
					var cloned map[string]any
					if json.Unmarshal(raw, &cloned) == nil {
						st.Payload = cloned
					}
				}
			}
			block.State = &st
		}
		out[i] = block
	}
	return out
}

// IsInteractionKind reports whether kind is a recognized interaction badge kind.
func IsInteractionKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case InteractionButton, InteractionQuickReply, InteractionForm, InteractionChecklist:
		return true
	default:
		return false
	}
}

// InteractionBlock formats a structured interaction envelope followed by a
// clean human-readable summary. The LLM reads both; the WebUI strips the
// <interaction> XML wrapper and renders a dedicated interaction badge.
func InteractionBlock(kind string, payload map[string]any, summary string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	if !IsInteractionKind(k) {
		k = InteractionButton
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte("{}")
	}

	var b strings.Builder
	b.WriteString(InteractionBlockOpen)
	b.WriteString(` kind="`)
	b.WriteString(k)
	b.WriteString("\">\n")
	b.Write(raw)
	b.WriteString("\n")
	b.WriteString(InteractionBlockClose)

	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		b.WriteString("\n")
		b.WriteString(trimmed)
	}
	return b.String()
}

var interactionBlockPattern = regexp.MustCompile(`(?is)<interaction\b[^>]*\bkind="(?:button|quick_reply|form|checklist)"[^>]*>.*?</interaction>[ \t]*\n?`)

// HasInteractionBlock reports whether text contains an <interaction> envelope.
func HasInteractionBlock(text string) bool {
	return strings.Contains(text, InteractionBlockOpen) && strings.Contains(text, InteractionBlockClose)
}

// StripInteractionBlocks removes machine-readable <interaction>...</interaction>
// JSON envelopes while preserving the trailing human-readable summary so titles,
// turn rail labels, previews, and CLI transcripts stay clean.
func StripInteractionBlocks(text string) string {
	if !HasInteractionBlock(text) {
		return text
	}
	return strings.TrimSpace(interactionBlockPattern.ReplaceAllString(text, ""))
}
