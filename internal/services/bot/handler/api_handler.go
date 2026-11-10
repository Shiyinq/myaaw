package handler

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"myaaw/internal/agent"
	"myaaw/internal/agent/approvals"
	"myaaw/internal/channel"
	api "myaaw/internal/channel/api"
	"myaaw/internal/services/bot/service"
	"myaaw/internal/utils"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// requestIDFor picks the id a run is registered under. Clients that do not send
// one get a generated id, which the stream announces so they can reattach.
func requestIDFor(requestID string) string {
	if trimmed := strings.TrimSpace(requestID); trimmed != "" {
		return trimmed
	}
	return uuid.NewString()
}

type ChatResponse struct {
	Text           string               `json:"text"`
	ConversationID string               `json:"conversation_id,omitempty"`
	RunID          string               `json:"run_id,omitempty"`
	Timeline       []api.TimelineItem   `json:"timeline,omitempty"`
	Usage          *UsageResponse       `json:"usage,omitempty"`
	Attachments    []AttachmentResponse `json:"attachments,omitempty"`
}

// AttachmentResponse is one delivered file. Small tool attachments may carry
// base64 bytes; path-based attachments name a virtual path the client fetches.
type AttachmentResponse struct {
	FileName   string `json:"file_name"`
	MimeType   string `json:"mime_type,omitempty"`
	DataBase64 string `json:"data_base64,omitempty"`
	Path       string `json:"path,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Size       int64  `json:"size,omitempty"`
}

type UsageResponse struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatStopRequest identifies a running chat for POST /api/chat/stop.
type ChatStopRequest struct {
	RequestID string `json:"request_id"`
}

// APIChatStop
// @Summary		Stop a running chat
// @Description	Cancels the run started with the same request_id. Whatever the model produced is kept in the transcript, and the run stops spending provider tokens.
// @Tags		API
// @Accept		json
// @Produce	json
// @Param		request	body		ChatStopRequest	true	"Request id of the run to stop"
// @Success		200		{object}	map[string]bool
// @Failure		400		{object}	common.ErrorResponse
// @Router		/api/chat/stop [post]
func (s *BotHandlerImpl) APIChatStop(c fiber.Ctx) error {
	var req ChatStopRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid JSON request body",
		})
	}

	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "request_id is required",
		})
	}

	return c.JSON(fiber.Map{"stopped": activeRuns.stop(requestID)})
}

// APIChat
// @Summary		API Chat
// @Description	Direct REST API chat endpoint (non-streaming)
// @Tags		API
// @Produce		json
// @Accept		json
// @Param		request	body		api.ChatRequest	true	"Chat request; user_id is optional and defaults to 0"
// @Success		200		{object}	ChatResponse
// @Failure     400     {object}    common.ErrorResponse
// @Failure     403     {object}    common.ErrorResponse
// @Failure     404     {object}    common.ErrorResponse
// @Failure     500     {object}    common.ErrorResponse
// @Router		/api/chat [post]
func (s *BotHandlerImpl) APIChat(c fiber.Ctx) error {
	var req api.ChatRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid JSON request body",
		})
	}

	raw, _ := json.Marshal(req)
	apiAdapter := s.channelRegistry.Get("api")
	if apiAdapter == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "API channel not configured",
		})
	}

	msg, err := apiAdapter.ParseIncoming(raw)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	log.Printf("API chat request from user ID %v", msg.UserID)

	timeline := api.NewTimeline(nil)
	run := &apiRun{
		timeline: timeline,
		done:     make(chan apiRunResult, 1),
		first:    make(chan approvals.Request, 1),
	}
	// The run outlives the request so an approval prompt can pause it, and so the
	// client can still stop it through its request id.
	chatRun := activeRuns.start(requestIDFor(req.RequestID))
	runCtx := approvals.WithRequester(chatRun.Context(), &blockingApprovalRequester{run: run})

	go func() {
		defer chatRun.finish()
		out, runErr := s.botService.BotStreamContext(runCtx, msg, func(chunk channel.StreamChunk) {
			if chunk.Kind == "conversation" {
				chatRun.setConversation(chunk.ConversationID)
			}
			timeline.HandleChunk(chunk)
		})
		run.done <- apiRunResult{out: out, err: runErr}
		untrackRun(run)
	}()

	select {
	case res := <-run.done:
		return s.writeChatResult(c, run, res)
	case req := <-run.first:
		trackRun(req.ID, run)
		return c.JSON(approvalRequestPayload(req))
	}
}

// writeChatResult serializes a completed (or failed) non-streaming chat run.
func (s *BotHandlerImpl) writeChatResult(c fiber.Ctx, run *apiRun, res apiRunResult) error {
	if res.err != nil {
		log.Printf("API chat error: %v", res.err)
		formattedError := utils.FormatErrorMessage(res.err)
		status := fiber.StatusInternalServerError
		switch {
		case errors.Is(res.err, service.ErrConversationNotFound):
			status = fiber.StatusNotFound
		case errors.Is(res.err, service.ErrConversationForbidden):
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{
			"error": formattedError,
		})
	}

	out := res.out
	resp := ChatResponse{
		Text:           out.Text,
		ConversationID: out.ConversationID,
		RunID:          out.RunID,
		Timeline:       run.timeline.Items(),
	}
	for _, att := range out.Attachments {
		item := AttachmentResponse{
			FileName: att.FileName,
			MimeType: att.MimeType,
			Path:     att.Path,
			Kind:     att.Kind,
			Size:     att.Size,
		}
		if len(att.Data) > 0 {
			item.DataBase64 = base64.StdEncoding.EncodeToString(att.Data)
		}
		resp.Attachments = append(resp.Attachments, item)
	}
	if out.Usage.TotalTokens > 0 {
		resp.Usage = &UsageResponse{
			PromptTokens:     out.Usage.PromptTokens,
			CompletionTokens: out.Usage.CompletionTokens,
			TotalTokens:      out.Usage.TotalTokens,
		}
	}
	return c.Status(fiber.StatusOK).JSON(resp)
}

// APIChatStream
// @Summary		API Chat Stream
// @Description	Direct REST API chat endpoint with Server-Sent Events streaming
// @Tags		API
// @Produce		text/event-stream
// @Accept		json
// @Param		request	body		api.ChatRequest	true	"Chat request; user_id is optional and defaults to 0"
// @Success		200
// @Failure     400     {object}    common.ErrorResponse
// @Failure     403     {object}    common.ErrorResponse
// @Failure     404     {object}    common.ErrorResponse
// @Failure     500     {object}    common.ErrorResponse
// @Router		/api/chat/stream [post]
func (s *BotHandlerImpl) APIChatStream(c fiber.Ctx) error {
	var req api.ChatRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid JSON request body",
		})
	}

	// Parse incoming using adapter
	raw, _ := json.Marshal(req)
	apiAdapter := s.channelRegistry.Get("api")
	if apiAdapter == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "API channel not configured",
		})
	}

	msg, err := apiAdapter.ParseIncoming(raw)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	log.Printf("API stream chat request from user ID %v", msg.UserID)

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	// The run is owned by the broker, not by this request: the connection only
	// attaches to its event stream. A refresh, a closed tab or a logout leaves
	// the answer running, and GET /api/chat/run/:id/stream attaches again later.
	run := activeRuns.start(requestIDFor(req.RequestID))
	// Clients that did not choose an id can reattach with the one we picked.
	run.publish("run", fiber.Map{"request_id": run.requestID})

	timeline := api.NewTimeline(func(event api.SSEEvent) {
		run.publish(event.Name, event.Data)
	})

	go func() {
		defer run.finish()

		requester := &sseApprovalRequester{emit: timeline.HandleChunk}
		runCtx := approvals.WithRequester(run.Context(), requester)
		out, err := s.botService.BotStreamContext(runCtx, msg, func(chunk channel.StreamChunk) {
			if chunk.Kind == "conversation" {
				run.setConversation(chunk.ConversationID)
			}
			timeline.HandleChunk(chunk)
		})
		if err != nil {
			log.Printf("API stream error for user ID %v: %v", msg.UserID, err)
			run.publish("error", fiber.Map{
				"error": utils.FormatErrorMessage(err),
			})
			run.publish("done", fiber.Map{
				"ok":      false,
				"stopped": agent.StoppedByClient(run.Context()),
			})
			return
		}

		timeline.Done(out)
	}()

	c.SendStreamWriter(func(w *bufio.Writer) {
		run.attach(w, 0)
	})

	return nil
}

// APIChatRuns
// @Summary		List runs
// @Description	Lists the runs a client can attach to, newest first. Finished runs stay listed for a short while so a reconnecting tab can read their tail.
// @Tags			API
// @Produce		json
// @Param			conversation_id query string false "Only runs of this conversation"
// @Success		200 {object} map[string][]RunInfo
// @Router			/api/chat/runs [get]
func (s *BotHandlerImpl) APIChatRuns(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"runs": activeRuns.list(c.Query("conversation_id"))})
}

// APIChatRunStream
// @Summary		Attach to a run
// @Description	Streams the events of a running (or recently finished) run, replaying everything after `after`. The run keeps going when this connection drops.
// @Tags			API
// @Produce		text/event-stream
// @Param			id path string true "Request id of the run"
// @Param			after query int false "Skip events before this index" default(0)
// @Success		200
// @Failure		404 {object} common.ErrorResponse
// @Router			/api/chat/run/{id}/stream [get]
func (s *BotHandlerImpl) APIChatRunStream(c fiber.Ctx) error {
	run, ok := activeRuns.get(c.Params("id"))
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "run not found",
		})
	}

	after := 0
	if value := c.Query("after"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "after must be a number",
			})
		}
		after = parsed
	}
	if after < 0 {
		after = 0
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	c.SendStreamWriter(func(w *bufio.Writer) {
		run.attach(w, after)
	})

	return nil
}
