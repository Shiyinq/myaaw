package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
	api "myaaw/internal/channel/api"
	"myaaw/internal/utils"

	"github.com/gofiber/fiber/v3"
)

type ChatResponse struct {
	Text  string         `json:"text"`
	Trace []TraceStep    `json:"trace,omitempty"`
	Usage *UsageResponse `json:"usage,omitempty"`
}

type TraceStep struct {
	Action      string `json:"action"`
	Observation string `json:"observation,omitempty"`
}

type UsageResponse struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// APIChat
// @Summary		API Chat
// @Description	Direct REST API chat endpoint (non-streaming)
// @Tags		API
// @Produce		json
// @Accept		json
// @Param		request	body		api.ChatRequest	true	"Chat request with user_id and text"
// @Success		200		{object}	ChatResponse
// @Failure     400    	{object}   	common.ErrorResponse
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

	out, err := s.botService.Bot(msg)
	if err != nil {
		log.Printf("API chat error for user ID %v: %v", msg.UserID, err)
		formattedError := utils.FormatErrorMessage(err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": formattedError,
		})
	}

	resp := ChatResponse{
		Text: out.Text,
	}

	if len(out.Trace) > 0 {
		for _, step := range out.Trace {
			resp.Trace = append(resp.Trace, TraceStep{
				Action:      step.Action,
				Observation: step.Observation,
			})
		}
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
// @Param		request	body		api.ChatRequest	true	"Chat request with user_id and text"
// @Success		200
// @Failure     400    	{object}   	common.ErrorResponse
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

	c.SendStreamWriter(func(w *bufio.Writer) {
		writeSSE := func(event string, data interface{}) {
			jsonData, err := json.Marshal(data)
			if err != nil {
				log.Printf("SSE marshal error: %v", err)
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData))
			w.Flush()
		}

		out, err := s.botService.BotStream(msg, func(chunk channel.StreamChunk) {
			if len(chunk.ToolCalls) > 0 {
				writeSSE("tool", fiber.Map{
					"name": chunk.ToolCalls[0].Function.Name,
				})
			} else if chunk.Text != "" {
				writeSSE("chunk", fiber.Map{
					"text": chunk.Text,
				})
			}

			if len(chunk.Trace) > 0 {
				var traceSteps []TraceStep
				for _, step := range chunk.Trace {
					traceSteps = append(traceSteps, TraceStep{
						Action:      step.Action,
						Observation: step.Observation,
					})
				}
				writeSSE("trace", traceSteps)
			}
		})

		if err != nil {
			log.Printf("API stream error for user ID %v: %v", msg.UserID, err)
			writeSSE("error", fiber.Map{
				"error": utils.FormatErrorMessage(err),
			})
			writeSSE("done", fiber.Map{"ok": false})
			return
		}

		doneData := fiber.Map{
			"ok":   true,
			"text": out.Text,
		}
		if out.Usage.TotalTokens > 0 {
			doneData["usage"] = UsageResponse{
				PromptTokens:     out.Usage.PromptTokens,
				CompletionTokens: out.Usage.CompletionTokens,
				TotalTokens:      out.Usage.TotalTokens,
			}
		}
		writeSSE("done", doneData)
	})

	return nil
}
