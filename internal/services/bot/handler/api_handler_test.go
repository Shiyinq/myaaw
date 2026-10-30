package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myaaw/internal/channel"
	apiAdapter "myaaw/internal/channel/api"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/service"

	"github.com/gofiber/fiber/v3"
)

// fakeBotService only implements the streaming entry point used by the API
// handlers. The embedded interface keeps the remaining methods satisfied.
type fakeBotService struct {
	service.BotService
	out    *channel.OutgoingMessage
	chunks []channel.StreamChunk
	err    error
}

func (f *fakeBotService) BotStreamContext(
	_ context.Context,
	_ *channel.IncomingMessage,
	onChunk func(channel.StreamChunk),
) (*channel.OutgoingMessage, error) {
	for _, chunk := range f.chunks {
		onChunk(chunk)
	}
	return f.out, f.err
}

func newAPIApp(t *testing.T, fake *fakeBotService) *fiber.App {
	return newAPIAppWith(t, fake)
}

func newAPIAppWith(t *testing.T, svc service.BotService) *fiber.App {
	t.Helper()

	registry := channel.NewRegistry()
	registry.Register(apiAdapter.NewAPIAdapter())

	h := NewBotHandler(svc, registry)
	app := fiber.New()
	app.Post("/api/chat", h.APIChat)
	app.Post("/api/chat/stream", h.APIChatStream)
	app.Post("/api/approvals/:id", h.ApprovalDecision)
	return app
}

func newFakeBotService() *fakeBotService {
	return &fakeBotService{
		chunks: []channel.StreamChunk{
			{Kind: "thought", Thought: "checking"},
			{
				Kind:          "tool",
				ToolCallID:    "call_1",
				ToolName:      "bash",
				ToolArguments: `{"command":"ls"}`,
			},
			{
				Kind:           "tool_result",
				ToolCallID:     "call_1",
				ToolName:       "bash",
				ToolResultText: "file1.txt",
			},
			{Kind: "text", Text: "done"},
		},
		out: &channel.OutgoingMessage{
			Text:           "done",
			ConversationID: "conv-1",
			RunID:          "run_test",
			Usage: provider.Usage{
				PromptTokens:     10,
				CompletionTokens: 4,
				TotalTokens:      14,
			},
		},
	}
}

func TestAPIChatReturnsTimeline(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"user_id":7,"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	var got ChatResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v, body = %s", err, body)
	}

	if got.Text != "done" || got.ConversationID != "conv-1" || got.RunID != "run_test" {
		t.Errorf("text/conversation/run = %q/%q/%q", got.Text, got.ConversationID, got.RunID)
	}
	if got.Usage == nil || got.Usage.TotalTokens != 14 {
		t.Errorf("usage = %+v", got.Usage)
	}
	if len(got.Timeline) != 3 {
		t.Fatalf("timeline = %+v", got.Timeline)
	}
	if got.Timeline[0].Kind != "thought" || got.Timeline[0].Text != "checking" {
		t.Errorf("timeline[0] = %+v", got.Timeline[0])
	}
	if got.Timeline[1].Kind != "tool" || got.Timeline[1].Result != "file1.txt" {
		t.Errorf("timeline[1] = %+v", got.Timeline[1])
	}
	if got.Timeline[2].Kind != "text" || got.Timeline[2].Text != "done" {
		t.Errorf("timeline[2] = %+v", got.Timeline[2])
	}
}

func TestAPIChatStreamEmitsTimelineEvents(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	req := httptest.NewRequest("POST", "/api/chat/stream", strings.NewReader(`{"user_id":7,"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	stream := string(body)
	for _, want := range []string{
		"event: thought",
		"event: tool",
		"event: tool_result",
		"event: chunk",
		"event: done",
		`"arguments":"{\"command\":\"ls\"}"`,
		`"result":"file1.txt"`,
		`"conversation_id":"conv-1"`,
		`"run_id":"run_test"`,
	} {
		if !strings.Contains(stream, want) {
			t.Errorf("stream missing %q:\n%s", want, stream)
		}
	}
}

func TestAPIChatInvalidConversation(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{service.ErrConversationNotFound, fiber.StatusNotFound},
		{service.ErrConversationForbidden, fiber.StatusForbidden},
	}

	for _, tc := range cases {
		app := newAPIApp(t, &fakeBotService{err: tc.err})

		req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"user_id":7,"text":"hi","conversation_id":"missing"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != tc.status {
			t.Fatalf("err %v: status = %d, want %d", tc.err, resp.StatusCode, tc.status)
		}
	}
}

func TestAPIChatAllowsMissingUserID(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
