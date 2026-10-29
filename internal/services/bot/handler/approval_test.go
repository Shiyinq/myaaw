package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/channel"
	"myaaw/internal/services/bot/service"

	"github.com/gofiber/fiber/v3"
)

// approvalBotService pauses on the first approval request, then returns a
// response that records the decision.
type approvalBotService struct {
	service.BotService
	decision approvals.Decision
}

func (f *approvalBotService) BotStreamContext(
	ctx context.Context,
	_ *channel.IncomingMessage,
	_ func(channel.StreamChunk),
) (*channel.OutgoingMessage, error) {
	requester := approvals.RequesterFromContext(ctx)
	if requester == nil {
		return nil, fmt.Errorf("missing approval requester")
	}
	decision, err := requester.Request(ctx, approvals.Request{
		ID:          "appr-test-1",
		RunID:       "run-1",
		Channel:     "api",
		UserID:      "7",
		ToolName:    "bash",
		ArgsSummary: `{"command":"ls"}`,
	})
	if err != nil {
		return nil, err
	}
	f.decision = decision
	return &channel.OutgoingMessage{
		Text:           "done:" + string(decision),
		ConversationID: "conv-1",
		RunID:          "run-1",
	}, nil
}

func TestAPIChatApprovalFlow(t *testing.T) {
	svc := &approvalBotService{}
	app := newAPIAppWith(t, svc)

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"user_id":7,"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	var first struct {
		Status   string `json:"status"`
		Approval struct {
			ID       string `json:"id"`
			ToolName string `json:"tool_name"`
		} `json:"approval"`
	}
	if err := json.Unmarshal(body, &first); err != nil {
		t.Fatalf("unmarshal: %v, body = %s", err, body)
	}
	if first.Status != "approval_required" || first.Approval.ID == "" || first.Approval.ToolName != "bash" {
		t.Fatalf("first response = %+v", first)
	}

	req2 := httptest.NewRequest("POST", "/api/approvals/"+first.Approval.ID, strings.NewReader(`{"decision":"allow_once","user_id":7}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != fiber.StatusOK {
		t.Fatalf("decision status = %d, body = %s", resp2.StatusCode, body2)
	}

	var final ChatResponse
	if err := json.Unmarshal(body2, &final); err != nil {
		t.Fatalf("unmarshal final: %v, body = %s", err, body2)
	}
	if final.Text != "done:allow_once" {
		t.Fatalf("final = %+v", final)
	}
	if svc.decision != approvals.AllowOnce {
		t.Fatalf("run saw decision %q", svc.decision)
	}
}

func TestApprovalDecisionUnknownID(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	req := httptest.NewRequest("POST", "/api/approvals/does-not-exist", strings.NewReader(`{"decision":"deny","user_id":0}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestApprovalDecisionRequiresUser(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	req := httptest.NewRequest("POST", "/api/approvals/whatever", strings.NewReader(`{"decision":"deny"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestApprovalDecisionHeaderUser(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	req := httptest.NewRequest("POST", "/api/approvals/does-not-exist", strings.NewReader(`{"decision":"deny"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(approvals.UserIDHeader, "7")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d; want 404 once the user is known", resp.StatusCode)
	}
}

func TestApprovalDecisionWrongUser(t *testing.T) {
	svc := &approvalBotService{}
	app := newAPIAppWith(t, svc)

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"user_id":7,"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var first struct {
		Approval struct {
			ID string `json:"id"`
		} `json:"approval"`
	}
	if err := json.Unmarshal(body, &first); err != nil || first.Approval.ID == "" {
		t.Fatalf("first response = %s (err=%v)", body, err)
	}

	req2 := httptest.NewRequest("POST", "/api/approvals/"+first.Approval.ID, strings.NewReader(`{"decision":"allow_once","user_id":99}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != fiber.StatusForbidden {
		body2, _ := io.ReadAll(resp2.Body)
		t.Fatalf("status = %d, body = %s; want 403", resp2.StatusCode, body2)
	}
}

func TestApprovalDecisionRejectsInvalidDecision(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	req := httptest.NewRequest("POST", "/api/approvals/whatever", strings.NewReader(`{"decision":"maybe"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
