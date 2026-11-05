package provider

import (
	"regexp"
	"strings"
)

// A trigger turn starts on its own (a cron job, a heartbeat tick, a finished
// sub-agent) rather than from something a person typed. The model needs to know
// that; people do not, so the whole turn travels inside a block clients can
// strip, the same way <userinfo> and <attached> do.
//
//	<trigger kind="cron">
//	…rules for this kind…
//	the prompt the user scheduled
//	</trigger>
//
// Everything sits inside the one tag: a client replaces the block with a badge,
// and nothing in the body ever reaches the screen.
const (
	TriggerCron      = "cron"
	TriggerHeartbeat = "heartbeat"
	TriggerSubagent  = "subagent"

	// TriggerBlockOpen starts the block. The kind travels as an attribute so a
	// client can draw a label without knowing the wording.
	TriggerBlockOpen  = "<trigger"
	TriggerBlockClose = "</trigger>"
)

var (
	triggerBlockPattern = regexp.MustCompile(`(?s)<trigger\b[^>]*>.*?</trigger>[ \t]*\n?`)
	triggerKindPattern  = regexp.MustCompile(`<trigger\b[^>]*\bkind="([^"]+)"`)
)

// The copy below is the whole instruction the model gets for a trigger kind.
// Keep each rule on its own line: the model follows short, flat lists far more
// reliably than a paragraph.
const (
	cronInstruction = `Scheduled turn. The user does not see this block, only your reply.
- Message to send: deliver it in its language, keeping every time, number, name,
  and link as written; rewrite only enough to read naturally.
- Task to run: do it, then report the result in two or three lines.
- Never mention the schedule or this block. One message; if it cannot happen,
  say what failed in one line.`

	// The tick prompt arrives as the content and is all rules, so it is joined to
	// this line (see TriggerBlock). It only adds what the prompt cannot say about
	// itself.
	heartbeatInstruction = `Never mention the tick, the schedule, or this block. Stay silent with HEARTBEAT_OK when there is nothing worth saying.`

	// The sub-agent copy is deliberately unchanged from the pre-tag wording.
	subagentInstruction = `(NOTE: The user does NOT see this trigger message. A background sub-agent has completed its task. Review its report below, inform the user about the completion, and summarize the key findings/results. If you started multiple sub-agents, check the batch progress.)`
)

// IsTriggerKind reports whether kind is one the block understands.
func IsTriggerKind(kind string) bool {
	switch kind {
	case TriggerCron, TriggerHeartbeat, TriggerSubagent:
		return true
	}
	return false
}

// TriggerInstruction is the model-only rules for a trigger kind.
func TriggerInstruction(kind string) string {
	switch kind {
	case TriggerCron:
		return cronInstruction
	case TriggerHeartbeat:
		return heartbeatInstruction
	case TriggerSubagent:
		return subagentInstruction
	}
	return ""
}

// TriggerBlock renders one whole turn: the kind, the rules, and the content.
// An unknown kind returns "" so a caller can leave the text alone.
//
// A heartbeat is special: its "content" is the tick prompt, which is nothing but
// rules, so it folds into the rules and the body has no content half.
func TriggerBlock(kind, content string) string {
	if !IsTriggerKind(kind) {
		return ""
	}

	instruction := strings.TrimSpace(TriggerInstruction(kind))
	content = strings.TrimSpace(content)
	if kind == TriggerHeartbeat {
		instruction = joinInstruction(content, instruction)
		content = ""
	}

	var b strings.Builder
	b.WriteString(TriggerBlockOpen)
	b.WriteString(` kind="`)
	b.WriteString(kind)
	b.WriteString(`">`)

	if body := joinInstruction(instruction, content); body != "" {
		b.WriteString("\n")
		b.WriteString(body)
	}

	b.WriteString("\n")
	b.WriteString(TriggerBlockClose)
	return b.String()
}

// joinInstruction puts the second part after the first, separated by a blank line.
func joinInstruction(first, second string) string {
	switch {
	case first == "":
		return second
	case second == "":
		return first
	default:
		return first + "\n\n" + second
	}
}

// HasTriggerBlock reports whether text carries the trigger marker.
func HasTriggerBlock(text string) bool {
	return strings.Contains(text, TriggerBlockOpen)
}

// StripTriggerBlocks replaces every trigger block with the content it carries, so
// a title, a preview, or the CLI shows what the turn was about and never the rules.
func StripTriggerBlocks(text string) string {
	if !strings.Contains(text, TriggerBlockOpen) {
		return text
	}
	return strings.TrimSpace(triggerBlockPattern.ReplaceAllStringFunc(text, triggerContent))
}

// triggerContent pulls the part of the block that came from the scheduler.
func triggerContent(block string) string {
	open := strings.Index(block, ">")
	closing := strings.LastIndex(block, TriggerBlockClose)
	if open < 0 || closing < open {
		return ""
	}

	kind := ""
	if match := triggerKindPattern.FindStringSubmatch(block); match != nil {
		kind = match[1]
	}
	// A heartbeat is rules all the way down, so nothing in it is worth showing.
	if kind == TriggerHeartbeat {
		return ""
	}

	body := strings.TrimSpace(block[open+1 : closing])
	return strings.TrimSpace(strings.TrimPrefix(body, strings.TrimSpace(TriggerInstruction(kind))))
}
