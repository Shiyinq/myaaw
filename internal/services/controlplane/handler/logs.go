package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListLogs returns the agent log files.
// @Summary      List log files
// @Tags         Logs
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} LogsResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/logs [get]
func (h *Handler) ListLogs(c fiber.Ctx) error {
	files, err := h.logs.List(c.Params("name"))
	if err != nil {
		return logError(c, err)
	}
	return c.JSON(fiber.Map{"files": files})
}

// StreamLogs tails one log file over Server-Sent Events.
// @Summary      Tail a log file
// @Description  Sends existing lines, then new lines as they are written. Events: ready, line.
// @Tags         Logs
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        file query string true "Log file name"
// @Success      200 {string} string "SSE stream"
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/logs/stream [get]
func (h *Handler) StreamLogs(c fiber.Ctx) error {
	fileName := c.Query("file")
	logPath, err := h.logs.Resolve(c.Params("name"), fileName)
	if err != nil {
		return logError(c, err)
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		writeEvent := func(event string, data any) error {
			payload, err := json.Marshal(data)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
				return err
			}
			return w.Flush()
		}

		if err := writeEvent("ready", map[string]string{"file": fileName}); err != nil {
			return
		}
		_ = h.logs.Tail(c.Context(), logPath, func(line string) error {
			return writeEvent("line", map[string]string{"line": line})
		})
	})
}

func logError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrPathNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "log file not found"})
	case errors.Is(err, service.ErrInvalidPath):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
