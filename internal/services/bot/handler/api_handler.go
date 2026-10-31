package handler

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"myaaw/internal/agent/approvals"
	api "myaaw/internal/channel/api"
	"myaaw/internal/services/bot/service"
	"myaaw/internal/utils"
	"strings"

	"github.com/gofiber/fiber/v3"
)

type ChatResponse struct {
	Text           string               `json:"text"`
	ConversationID string               `json:"conversation_id,omitempty"`
	RunID          string               `json:"run_id,omitempty"`
	Timeline       []api.TimelineItem   `json:"timeline,omitempty"`
	Usage          *UsageResponse       `json:"usage,omitempty"`
	Attachments    []AttachmentResponse `json:"attachments,omitempty"`
}

// AttachmentResponse is one delivered file, base64-encoded for API clients.
type AttachmentResponse struct {
	FileName   string `json:"file_name"`
	MimeType   string `json:"mime_type,omitempty"`
	DataBase64 string `json:"data_base64"`
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
	// The run must survive the HTTP request returning an approval prompt.
	base := approvals.WithRequester(context.WithoutCancel(c.Context()), &blockingApprovalRequester{run: run})
	runCtx, _, release := runContext(base, req.RequestID)

	go func() {
		defer release()
		out, runErr := s.botService.BotStreamContext(runCtx, msg, timeline.HandleChunk)
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
		resp.Attachments = append(resp.Attachments, AttachmentResponse{
			FileName:   att.FileName,
			MimeType:   att.MimeType,
			DataBase64: base64.StdEncoding.EncodeToString(att.Data),
		})
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

	// The stream handler runs after this function returns, so the run context is
	// released inside the writer, not by a defer here.
	streamCtx, cancel, release := runContext(c.Context(), req.RequestID)

	c.SendStreamWriter(func(w *bufio.Writer) {
		defer release()

		// A failed write means the client is gone (closed tab, refreshed page).
		// Stopping the run keeps it from finishing in the background.
		writeSSE := func(event string, data interface{}) {
			jsonData, err := json.Marshal(data)
			if err != nil {
				log.Printf("SSE marshal error: %v", err)
				return
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData)); err != nil {
				stopRun(cancel)
				return
			}
			if err := w.Flush(); err != nil {
				stopRun(cancel)
			}
		}

		timeline := api.NewTimeline(func(event api.SSEEvent) {
			writeSSE(event.Name, event.Data)
		})

		requester := &sseApprovalRequester{emit: timeline.HandleChunk}
		runCtx := approvals.WithRequester(streamCtx, requester)
		out, err := s.botService.BotStreamContext(runCtx, msg, timeline.HandleChunk)
		if err != nil {
			log.Printf("API stream error for user ID %v: %v", msg.UserID, err)
			writeSSE("error", fiber.Map{
				"error": utils.FormatErrorMessage(err),
			})
			writeSSE("done", fiber.Map{"ok": false})
			return
		}

		timeline.Done(out)
	})

	return nil
}
