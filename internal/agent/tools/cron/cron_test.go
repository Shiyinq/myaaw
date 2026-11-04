package cron

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"myaaw/internal/agent/tools"
	"myaaw/internal/agentdir"
)

// fakeCLI puts a stand-in `myaaw` on PATH so the tool's subprocess arguments and
// environment can be inspected without touching a real agent layout.
func fakeCLI(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	binDir := t.TempDir()
	script := filepath.Join(binDir, "myaaw")
	body := "#!/bin/sh\nprintf 'agent-env=%s\\n' \"$MYAAW_AGENT\"\nprintf '%s\\n' \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func hasPair(fields []string, key, value string) bool {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == key && fields[i+1] == value {
			return true
		}
	}
	return false
}

func TestCallToolBindsRunningAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("agent-b"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	t.Cleanup(func() { _ = agentdir.SetActive(agentdir.DefaultAgentName) })
	fakeCLI(t)

	out := NewCronTool().CallTool(`{
		"action": "add",
		"name": "Daily Report",
		"message": "Summarize the day",
		"at": "10m"
	}`, &tools.ToolsContext{
		Channel:        "telegram",
		ChatID:         "123456789",
		TopicID:        "42",
		ConversationID: "0f4e8b3a-1111-4222-8333-944455556666",
		Scope:          "tg:123456789:42",
	})

	if strings.HasPrefix(out, "Error") {
		t.Fatalf("CallTool failed: %s", out)
	}
	fields := strings.Fields(out)
	if !strings.Contains(out, "agent-env=agent-b") {
		t.Errorf("MYAAW_AGENT not passed to the CLI: %s", out)
	}
	for _, want := range [][2]string{
		{"--agent", "agent-b"},
		{"--channel", "telegram"},
		{"--to", "123456789"},
		{"--thread-id", "42"},
		{"--session-id", "0f4e8b3a-1111-4222-8333-944455556666"},
	} {
		if !hasPair(fields, want[0], want[1]) {
			t.Errorf("missing %s %s in: %v", want[0], want[1], fields)
		}
	}
}

// A Discord DM targets the user id, but the chat context exposes the DM channel
// id as ChatID, and models copy it. The job must still open the right DM.
func TestCallToolRewritesDMChannelIDToUser(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("agent-b"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	t.Cleanup(func() { _ = agentdir.SetActive(agentdir.DefaultAgentName) })
	fakeCLI(t)

	out := NewCronTool().CallTool(`{
		"action": "add",
		"name": "Reminder makan",
		"message": "Waktunya makan",
		"at": "2m",
		"channel": "discord",
		"to": "111222333444555666",
		"chat_type": "dm"
	}`, &tools.ToolsContext{
		Channel:        "discord",
		ChatID:         "111222333444555666",
		ChatType:       "dm",
		UserID:         "222333444555666777",
		ConversationID: "0f4e8b3a-1111-4222-8333-944455556666",
		Scope:          "",
	})

	fields := strings.Fields(out)
	if !hasPair(fields, "--to", "222333444555666777") {
		t.Errorf("DM target was not rewritten to the user id: %v", fields)
	}
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "--to" && fields[i+1] == "111222333444555666" {
			t.Fatalf("DM channel id was kept as the recipient: %v", fields)
		}
	}
}

// A reminder requested in one chat but aimed at another must not borrow the
// source room: its topic, session, and chat type belong to the target instead.
func TestCallToolDoesNotBorrowSourceRoomForAnotherChat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("agent-b"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	t.Cleanup(func() { _ = agentdir.SetActive(agentdir.DefaultAgentName) })
	fakeCLI(t)

	out := NewCronTool().CallTool(`{
		"action": "add",
		"name": "Sapa group",
		"message": "Hai semuanya",
		"at": "2m",
		"channel": "telegram",
		"to": "-1001234567890"
	}`, &tools.ToolsContext{
		Channel:        "telegram",
		ChatID:         "123456789",
		ChatType:       "private",
		UserID:         "123456789",
		ConversationID: "0f4e8b3a-1111-4222-8333-944455556666",
		Scope:          "",
	})

	fields := strings.Fields(out)
	if !hasPair(fields, "--to", "-1001234567890") {
		t.Errorf("target lost: %v", fields)
	}
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "--session-id", "--thread-id", "--chat-type":
			t.Errorf("source room field %s leaked into a cross-chat job: %v", fields[i], fields)
		}
	}
}

// A Discord guild target is not a DM just because the request came from a DM.
func TestCallToolDropsInheritedDiscordChatTypeForAnotherChannel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("agent-b"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	t.Cleanup(func() { _ = agentdir.SetActive(agentdir.DefaultAgentName) })
	fakeCLI(t)

	out := NewCronTool().CallTool(`{
		"action": "add",
		"name": "Daily report",
		"message": "Report time",
		"every": "1h",
		"channel": "discord",
		"to": "222333444555666777"
	}`, &tools.ToolsContext{
		Channel:        "discord",
		ChatID:         "111222333444555666",
		ChatType:       "dm",
		UserID:         "111222333444555666",
		ConversationID: "0f4e8b3a-1111-4222-8333-944455556666",
		Scope:          "",
	})

	fields := strings.Fields(out)
	if hasPair(fields, "--chat-type", "dm") {
		t.Errorf("DM chat type leaked into a guild target: %v", fields)
	}
}

func TestCallToolUsesExplicitTargetOverContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("agent-b"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	t.Cleanup(func() { _ = agentdir.SetActive(agentdir.DefaultAgentName) })
	fakeCLI(t)

	out := NewCronTool().CallTool(`{
		"action": "add",
		"name": "Daily Report",
		"message": "Summarize the day",
		"every": "1h",
		"channel": "discord",
		"to": "222333444555666777",
		"session_id": "1a2b3c4d-1111-4222-8333-944455556666"
	}`, &tools.ToolsContext{
		Channel:        "telegram",
		ChatID:         "123456789",
		ConversationID: "0f4e8b3a-1111-4222-8333-944455556666",
		Scope:          "tg:123456789",
	})

	fields := strings.Fields(out)
	if !hasPair(fields, "--channel", "discord") || !hasPair(fields, "--to", "222333444555666777") {
		t.Errorf("explicit target not used: %v", fields)
	}
	if !hasPair(fields, "--session-id", "1a2b3c4d-1111-4222-8333-944455556666") {
		t.Errorf("explicit session not used: %v", fields)
	}
}
