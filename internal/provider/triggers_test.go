package provider

import (
	"strings"
	"testing"
)

// The wording is the whole contract with the model, so it is pinned here: any
// change must show up as a deliberate diff.
func TestTriggerInstructionCopy(t *testing.T) {
	cron := TriggerInstruction(TriggerCron)
	for _, want := range []string{
		"Scheduled turn.",
		"The user does not see this block, only your reply.",
		"deliver it in its language",
		"keeping every time, number, name,",
		"Task to run: do it, then report the result",
		"Never mention the schedule or this block.",
		"say what failed in one line.",
	} {
		if !strings.Contains(cron, want) {
			t.Errorf("cron instruction is missing %q:\n%s", want, cron)
		}
	}
	// The old header line is gone: the block is the marker now.
	if strings.Contains(cron, "[SYSTEM TRIGGER") {
		t.Errorf("cron instruction still carries the old header:\n%s", cron)
	}

	// Heartbeat adds only the rules the tick prompt cannot state about itself.
	heartbeat := TriggerInstruction(TriggerHeartbeat)
	if !strings.Contains(heartbeat, "HEARTBEAT_OK") || !strings.Contains(heartbeat, "Never mention the tick") {
		t.Fatalf("heartbeat instruction = %q", heartbeat)
	}

	subagent := TriggerInstruction(TriggerSubagent)
	if !strings.HasPrefix(subagent, "(NOTE:") || !strings.Contains(subagent, "sub-agent has completed") {
		t.Fatalf("subagent instruction changed: %q", subagent)
	}
}

// The rules and the content share the one tag, so a client hides both by hiding
// the tag and shows a badge instead.
func TestTriggerBlock(t *testing.T) {
	block := TriggerBlock(TriggerCron, "Time to drink water")

	want := `<trigger kind="cron">` + "\n" +
		cronInstruction + "\n\n" +
		"Time to drink water\n" +
		`</trigger>`
	if block != want {
		t.Fatalf("block = %q, want %q", block, want)
	}
	if strings.Contains(block, "<system-info>") {
		t.Fatalf("block still nests a second tag: %q", block)
	}

	// A heartbeat folds its tick prompt into the rules.
	heartbeat := TriggerBlock(TriggerHeartbeat, "You are in HEARTBEAT checking mode.")
	if !strings.HasPrefix(heartbeat, `<trigger kind="heartbeat">`+"\n"+"You are in HEARTBEAT checking mode.") {
		t.Fatalf("heartbeat block = %q", heartbeat)
	}

	if got := TriggerBlock("unknown", "x"); got != "" {
		t.Fatalf("unknown kind = %q, want empty", got)
	}
}

// Stripping a block must leave the content behind: that is what a title, a
// preview, and the CLI show.
func TestStripTriggerBlocks(t *testing.T) {
	stored := TriggerBlock(TriggerCron, "Time to drink water")
	if got := StripTriggerBlocks(stored); got != "Time to drink water" {
		t.Fatalf("strip = %q, want the content only", got)
	}

	subagent := TriggerBlock(TriggerSubagent, "Report: 3 files synced.")
	if got := StripTriggerBlocks(subagent); got != "Report: 3 files synced." {
		t.Fatalf("subagent strip = %q", got)
	}

	// A heartbeat carries no content: its prompt is rules, so it strips to nothing.
	heartbeat := TriggerBlock(TriggerHeartbeat, "You are in HEARTBEAT checking mode.")
	if got := StripTriggerBlocks(heartbeat); got != "" {
		t.Fatalf("heartbeat strip = %q, want empty", got)
	}

	if got := StripTriggerBlocks("just a message"); got != "just a message" {
		t.Fatalf("plain text changed: %q", got)
	}
}

func TestStripContextBlocksKeepsTriggerContent(t *testing.T) {
	stored := "<userinfo>\nPlatform: API\n</userinfo>\n\n" +
		TriggerBlock(TriggerCron, "Time to drink water")
	if got := StripContextBlocks(stored); got != "Time to drink water" {
		t.Fatalf("strip = %q, want the content only", got)
	}

	// A message with no context blocks is returned untouched.
	const plain = "hello [SYSTEM TRIGGER: not a block]"
	if got := StripContextBlocks(plain); got != plain {
		t.Fatalf("plain text changed: %q", got)
	}
}

func TestHasTriggerBlock(t *testing.T) {
	if !HasTriggerBlock(TriggerBlock(TriggerCron, "x")) {
		t.Error("block not detected")
	}
	// Turns stored before the block existed are left alone; they are not markers.
	if HasTriggerBlock("[SYSTEM TRIGGER: HEARTBEAT]\nHEARTBEAT_OK") {
		t.Error("the old header is not a trigger block")
	}
	if HasTriggerBlock("just a message") {
		t.Error("plain text reported as a trigger")
	}
}
