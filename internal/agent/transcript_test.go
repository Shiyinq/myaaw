package agent

import (
	"strings"
	"testing"

	"myaaw/internal/provider"
)

func TestEstimateTokensCountsImagesFlat(t *testing.T) {
	msg := provider.Message{Role: "tool", Content: []provider.ContentItem{{
		Type: "image_url", ImageURL: &provider.ImageInfo{URL: "data:image/png;base64,AAAA"},
	}}}
	if got := EstimateTokens([]provider.Message{msg}); got < imageTokenCost {
		t.Fatalf("tokens = %d; want at least %d", got, imageTokenCost)
	}
}

func imageToolMessage(dataSize int) provider.Message {
	return provider.Message{
		Role: "tool",
		Name: "screenshot",
		Content: []provider.ContentItem{
			{Type: "text", Text: "captured"},
			{Type: "image_url", ImageURL: &provider.ImageInfo{
				URL: "data:image/png;base64," + strings.Repeat("A", dataSize),
			}},
		},
	}
}

func imageSteps(count, dataSize int) []provider.Message {
	messages := make([]provider.Message, 0, count*2)
	for i := 0; i < count; i++ {
		messages = append(messages,
			provider.Message{Role: "assistant", Content: "thinking"},
			imageToolMessage(dataSize),
		)
	}
	return messages
}

func countImages(messages []provider.Message) int {
	total := 0
	for i := range messages {
		count, _ := messageImageUsage(&messages[i])
		total += count
	}
	return total
}

func TestEvictTranscriptImagesByCount(t *testing.T) {
	messages := imageSteps(150, 16)
	kept := EvictTranscriptImages(messages, 100, 0)

	if got := countImages(kept); got != 100 {
		t.Fatalf("images = %d, want 100", got)
	}
	// The newest batch is never evicted.
	if count, _ := messageImageUsage(&kept[len(kept)-1]); count != 1 {
		t.Fatal("newest image batch was evicted")
	}
	// Evicted tool messages keep their text and gain a placeholder.
	if text := provider.MessageText(kept[1]); !strings.Contains(text, "captured") || !strings.Contains(text, imagePlaceholder) {
		t.Fatalf("evicted message = %q", text)
	}
}

func TestEvictTranscriptImagesByBytes(t *testing.T) {
	messages := imageSteps(10, 1000)
	kept := EvictTranscriptImages(messages, 0, 5000)

	bytes := 0
	for i := range kept {
		_, b := messageImageUsage(&kept[i])
		bytes += b
	}
	if bytes > 5000 {
		t.Fatalf("image bytes = %d, want <= 5000", bytes)
	}
	if got := countImages(kept); got == 0 || got >= 10 {
		t.Fatalf("images = %d, want some but not all", got)
	}
}

func TestEvictTranscriptImagesKeepsNewestOverBudget(t *testing.T) {
	messages := imageSteps(1, 10000)
	kept := EvictTranscriptImages(messages, 0, 100)
	if got := countImages(kept); got != 1 {
		t.Fatalf("images = %d; the newest batch must survive", got)
	}
}

func TestEvictTranscriptImagesIsIdempotent(t *testing.T) {
	messages := imageSteps(20, 16)
	first := EvictTranscriptImages(messages, 5, 0)
	countAfterFirst := countImages(first)
	second := EvictTranscriptImages(first, 5, 0)
	if countImages(second) != countAfterFirst {
		t.Fatalf("second eviction changed the transcript: %d -> %d", countAfterFirst, countImages(second))
	}
	if countAfterFirst != 5 {
		t.Fatalf("images after eviction = %d, want 5", countAfterFirst)
	}
}

func TestMigrateLegacyTranscriptExpandsTraceOnce(t *testing.T) {
	legacy := []provider.Message{
		{Role: "user", Content: "hello"},
		{
			Role:    "assistant",
			Content: "final answer",
			Trace: []provider.ReactStep{
				{Thought: "think", Action: "first", ActionInput: `{"x":"1"}`, Observation: "one", StepGroup: 1},
				{Thought: "think", Action: "second", ActionInput: `{"x":"2"}`, Observation: "two", StepGroup: 1},
			},
		},
	}

	migrated := MigrateLegacyTranscript(legacy)
	if len(migrated) != 5 {
		t.Fatalf("migrated length = %d, want 5", len(migrated))
	}
	if migrated[0].Role != "user" {
		t.Fatalf("migrated[0] role = %q", migrated[0].Role)
	}
	if migrated[1].Role != "assistant" || len(migrated[1].ToolCalls) != 2 {
		t.Fatalf("migrated[1] is not an assistant with 2 tool calls: %+v", migrated[1])
	}
	if migrated[2].Role != "tool" || migrated[2].ToolCallID != migrated[1].ToolCalls[0].ID {
		t.Fatalf("first tool result does not match first tool call")
	}
	if migrated[3].Role != "tool" || migrated[3].ToolCallID != migrated[1].ToolCalls[1].ID {
		t.Fatalf("second tool result does not match second tool call")
	}
	if migrated[1].ToolCalls[0].ID == migrated[1].ToolCalls[1].ID {
		t.Fatal("synthetic tool call IDs are not unique")
	}
	if migrated[4].Role != "assistant" || migrated[4].Content != "final answer" {
		t.Fatalf("final assistant answer was not preserved")
	}
	for i, msg := range migrated {
		if msg.Version != transcriptVersion {
			t.Fatalf("migrated[%d] version = %d", i, msg.Version)
		}
	}

	again := MigrateLegacyTranscript(migrated)
	if len(again) != len(migrated) {
		t.Fatalf("migration is not idempotent: %d -> %d", len(migrated), len(again))
	}
}

func TestCompactTranscriptKeepsSystemAndLatestTurn(t *testing.T) {
	big := strings.Repeat("x", 2000)
	messages := []provider.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "old question " + big},
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "old-1", Type: "function", Function: provider.FunctionCall{Name: "bash", Arguments: `{"x":"1"}`}}}},
		{Role: "tool", ToolCallID: "old-1", Name: "bash", Content: "old output " + big},
		{Role: "assistant", Content: "old answer " + big},
		{Role: "user", Content: "new question"},
		{Role: "assistant", Content: "new answer"},
	}

	compacted := CompactTranscript(messages, 50)
	if len(compacted) != 3 {
		t.Fatalf("compacted length = %d, want 3: %+v", len(compacted), compacted)
	}
	if compacted[0].Role != "system" {
		t.Fatalf("system message was dropped")
	}
	if compacted[1].Role != "user" || compacted[1].Content != "new question" {
		t.Fatalf("latest user turn was not kept")
	}
	if compacted[2].Role != "assistant" || compacted[2].Content != "new answer" {
		t.Fatalf("latest assistant turn was not kept")
	}
}

func TestCompactTranscriptWithStatsReportsWhatWasDropped(t *testing.T) {
	big := strings.Repeat("x", 2000)
	messages := []provider.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "old question " + big},
		{Role: "assistant", Content: "old answer " + big},
		{Role: "user", Content: "new question"},
	}

	compacted, stats := CompactTranscriptWithStats(messages, 50)
	if !stats.Changed {
		t.Fatalf("stats did not report a change: %+v", stats)
	}
	if stats.Budget != 50 {
		t.Fatalf("budget = %d", stats.Budget)
	}
	if stats.TokensBefore <= stats.Budget {
		t.Fatalf("tokens before = %d, want over budget", stats.TokensBefore)
	}
	if stats.TokensAfter > stats.TokensBefore {
		t.Fatalf("tokens after = %d, before = %d", stats.TokensAfter, stats.TokensBefore)
	}
	if stats.DroppedMessages != len(messages)-len(compacted) {
		t.Fatalf("dropped messages = %d, want %d", stats.DroppedMessages, len(messages)-len(compacted))
	}
	if stats.DroppedTokens != stats.TokensBefore-stats.TokensAfter {
		t.Fatalf("dropped tokens = %d", stats.DroppedTokens)
	}
}

func TestCompactTranscriptWithStatsIsQuietWithinBudget(t *testing.T) {
	messages := []provider.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "hello"},
	}

	compacted, stats := CompactTranscriptWithStats(messages, 100000)
	if stats.Changed || stats.DroppedMessages != 0 || stats.DroppedTokens != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if len(compacted) != len(messages) {
		t.Fatalf("compacted length = %d", len(compacted))
	}
}

func TestCompactTranscriptNeverSplitsToolPairs(t *testing.T) {
	big := strings.Repeat("x", 200)
	messages := []provider.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "question " + big},
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "c1", Type: "function", Function: provider.FunctionCall{Name: "bash", Arguments: `{"x":"1"}`}}}},
		{Role: "tool", ToolCallID: "c1", Name: "bash", Content: "result " + big},
		{Role: "assistant", Content: "answer"},
		{Role: "user", Content: "latest"},
	}

	// Budget that fits the first turn but not both; compaction walks backward
	// and must keep the assistant/tool pair together when it includes the turn.
	compacted := CompactTranscript(messages, 120)
	seenTool := false
	seenCall := false
	for _, msg := range compacted {
		if msg.Role == "tool" {
			seenTool = true
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			seenCall = true
		}
	}
	if seenTool != seenCall {
		t.Fatalf("compaction split a tool call/result pair: %+v", compacted)
	}
}
