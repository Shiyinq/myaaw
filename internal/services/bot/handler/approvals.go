package handler

import (
	"context"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/channel"
	api "myaaw/internal/channel/api"

	"github.com/gofiber/fiber/v3"
)

// approvalDecisionTimeout bounds how long a decision request waits for the run
// to finish (or to ask for another approval).
const approvalDecisionTimeout = 10 * time.Minute

// apiApprovals tracks pending approval decisions for the direct API channel.
var apiApprovals = approvals.NewRegistry()

// apiRun is one non-streaming chat run that may pause for approvals.
type apiRun struct {
	timeline *api.Timeline
	done     chan apiRunResult
	first    chan approvals.Request
}

type apiRunResult struct {
	out *channel.OutgoingMessage
	err error
}

var (
	apiRunsMu         sync.Mutex
	apiRunsByApproval = map[string]*apiRun{}
)

func trackRun(approvalID string, run *apiRun) {
	apiRunsMu.Lock()
	apiRunsByApproval[approvalID] = run
	apiRunsMu.Unlock()
}

func takeRun(approvalID string) *apiRun {
	apiRunsMu.Lock()
	defer apiRunsMu.Unlock()
	run := apiRunsByApproval[approvalID]
	delete(apiRunsByApproval, approvalID)
	return run
}

// untrackRun drops every pending entry for a finished run so abandoned
// approval_required responses do not leak memory.
func untrackRun(run *apiRun) {
	apiRunsMu.Lock()
	defer apiRunsMu.Unlock()
	for id, tracked := range apiRunsByApproval {
		if tracked == run {
			delete(apiRunsByApproval, id)
		}
	}
}

// sseApprovalRequester surfaces approvals as SSE events on the streaming API.
type sseApprovalRequester struct {
	emit func(channel.StreamChunk)
}

func (r *sseApprovalRequester) Request(ctx context.Context, req approvals.Request) (approvals.Decision, error) {
	decisions := apiApprovals.Register(req.ID)
	defer apiApprovals.Remove(req.ID)

	r.emit(channel.StreamChunk{
		Kind:        "approval_required",
		ApprovalID:  req.ID,
		ToolName:    req.ToolName,
		ToolAction:  req.Action,
		ArgsSummary: req.ArgsSummary,
		SubAgent:    req.SubAgent,
	})

	select {
	case decision := <-decisions:
		r.emit(channel.StreamChunk{
			Kind:       "approval_resolved",
			ApprovalID: req.ID,
			Decision:   string(decision),
		})
		return decision, nil
	case <-ctx.Done():
		r.emit(channel.StreamChunk{
			Kind:       "approval_resolved",
			ApprovalID: req.ID,
			Decision:   string(approvals.Deny),
			Reason:     "timed out",
		})
		return approvals.Deny, ctx.Err()
	}
}

// blockingApprovalRequester pauses a non-streaming run and hands the request to
// the handler, which returns it to the caller as `approval_required`.
type blockingApprovalRequester struct {
	run *apiRun
}

func (r *blockingApprovalRequester) Request(ctx context.Context, req approvals.Request) (approvals.Decision, error) {
	decisions := apiApprovals.Register(req.ID)
	defer apiApprovals.Remove(req.ID)

	select {
	case r.run.first <- req:
	case <-ctx.Done():
		return approvals.Deny, ctx.Err()
	}

	select {
	case decision := <-decisions:
		return decision, nil
	case <-ctx.Done():
		return approvals.Deny, ctx.Err()
	}
}

func approvalRequestPayload(req approvals.Request) fiber.Map {
	payload := fiber.Map{
		"id":           req.ID,
		"run_id":       req.RunID,
		"tool_name":    req.ToolName,
		"action":       req.Action,
		"args_summary": req.ArgsSummary,
	}
	if req.SubAgent != "" {
		payload["subagent"] = req.SubAgent
	}
	return fiber.Map{
		"status":   "approval_required",
		"approval": payload,
	}
}

// ApprovalDecision resolves a pending approval request. For a streaming run it
// only releases the decision; for a paused non-streaming run it waits for the
// resumed run and returns the final ChatResponse (or the next approval).
// @Summary      Resolve a tool approval
// @Description  Answers a pending approval request created by /api/chat or /api/chat/stream.
// @Tags         Chat
// @Accept       json
// @Produce      json
// @Param        id   path string true "Approval ID"
// @Param        request body object true "Decision: allow_once | allow_session | allow_always | deny"
// @Success      200
// @Failure      400 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /api/approvals/{id} [post]
func (s *BotHandlerImpl) ApprovalDecision(c fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("id"))
	var body struct {
		Decision string `json:"decision"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid JSON request body",
		})
	}
	decision := approvals.Decision(strings.ToLower(strings.TrimSpace(body.Decision)))
	switch decision {
	case approvals.AllowOnce, approvals.AllowSession, approvals.AllowAlways, approvals.Deny:
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "decision must be allow_once, allow_session, allow_always, or deny",
		})
	}

	run := takeRun(id)
	if run == nil {
		if apiApprovals.Resolve(id, decision) {
			return c.JSON(fiber.Map{"ok": true, "decision": string(decision)})
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "approval not found or already resolved",
		})
	}

	if !apiApprovals.Resolve(id, decision) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "approval not found or already resolved",
		})
	}

	select {
	case res := <-run.done:
		return s.writeChatResult(c, run, res)
	case req := <-run.first:
		trackRun(req.ID, run)
		return c.JSON(approvalRequestPayload(req))
	case <-time.After(approvalDecisionTimeout):
		// The run continues in the background; callers can reconnect or retry.
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"ok":       true,
			"decision": string(decision),
			"status":   "resumed",
		})
	}
}
