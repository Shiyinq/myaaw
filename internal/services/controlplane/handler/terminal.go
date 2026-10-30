package handler

import (
	"bufio"
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// TerminalStatus reports whether the control plane allows shells.
// @Summary      Terminal status
// @Tags         Terminal
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} map[string]bool
// @Failure      401 {object} common.ErrorResponse
// @Router       /agents/{name}/terminal [get]
func (h *Handler) TerminalStatus(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"enabled": h.terminals.Enabled()})
}

type terminalSizeRequest struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// OpenTerminal spawns a login shell for the agent.
// @Summary      Open a terminal
// @Description  Runs a real PTY on the API host, in the agent's home directory. Disabled unless the API was started with MYAAW_TERMINAL=1 (myaaw api restart --terminal).
// @Tags         Terminal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body terminalSizeRequest false "Initial size"
// @Success      200 {object} service.TerminalInfo
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      429 {object} common.ErrorResponse
// @Router       /agents/{name}/terminal [post]
func (h *Handler) OpenTerminal(c fiber.Ctx) error {
	var body terminalSizeRequest
	if len(c.Body()) > 0 {
		if err := c.Bind().JSON(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
		}
	}

	info, err := h.terminals.Open(c.Params("name"), body.Cols, body.Rows)
	if err != nil {
		return terminalError(c, err)
	}
	return c.JSON(info)
}

// StreamTerminal streams raw PTY output as a chunked binary response.
// @Summary      Stream terminal output
// @Tags         Terminal
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Session ID"
// @Success      200 {string} string "raw PTY output"
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/terminal/{id}/stream [get]
func (h *Handler) StreamTerminal(c fiber.Ctx) error {
	// Params must be read before the handler returns: fasthttp runs the stream
	// writer later, when the ctx has already been recycled.
	name, id := c.Params("name"), c.Params("id")

	reader, err := h.terminals.Reader(name, id)
	if err != nil {
		return terminalError(c, err)
	}

	c.Set("Content-Type", "application/octet-stream")
	c.Set("Cache-Control", "no-store")
	c.Set("X-Accel-Buffering", "no")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		// The stream is the session's only consumer. Once it ends - tab closed,
		// tab refreshed, write error, or the shell exiting - nobody is left to
		// talk to the PTY, so free the per-agent slot right away instead of
		// leaving it for the idle sweep.
		defer func() { _ = h.terminals.Close(name, id) }()

		buffer := make([]byte, 32*1024)
		for {
			read, readErr := reader.Read(buffer)
			if read > 0 {
				if _, writeErr := w.Write(buffer[:read]); writeErr != nil {
					return
				}
				if flushErr := w.Flush(); flushErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	})
}

// TouchTerminal keeps a session alive while its tab is open.
// @Summary      Terminal heartbeat
// @Description  Refreshes the idle clock. A session is reaped after two minutes without input, resize, or heartbeat.
// @Tags         Terminal
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Session ID"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/terminal/{id}/heartbeat [post]
func (h *Handler) TouchTerminal(c fiber.Ctx) error {
	if err := h.terminals.Touch(c.Params("name"), c.Params("id")); err != nil {
		return terminalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// TerminalInput writes raw client bytes to the shell.
// @Summary      Send terminal input
// @Tags         Terminal
// @Accept       application/octet-stream
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Session ID"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/terminal/{id}/input [post]
func (h *Handler) TerminalInput(c fiber.Ctx) error {
	if err := h.terminals.Write(c.Params("name"), c.Params("id"), c.Body()); err != nil {
		return terminalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ResizeTerminal applies a new window size.
// @Summary      Resize terminal
// @Tags         Terminal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Session ID"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/terminal/{id}/resize [post]
func (h *Handler) ResizeTerminal(c fiber.Ctx) error {
	var body terminalSizeRequest
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if err := h.terminals.Resize(c.Params("name"), c.Params("id"), body.Cols, body.Rows); err != nil {
		return terminalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// CloseTerminal kills the shell and forgets the session.
// @Summary      Close a terminal
// @Tags         Terminal
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Session ID"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /agents/{name}/terminal/{id} [delete]
func (h *Handler) CloseTerminal(c fiber.Ctx) error {
	if err := h.terminals.Close(c.Params("name"), c.Params("id")); err != nil {
		return terminalError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func terminalError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrTerminalDisabled), errors.Is(err, service.ErrTerminalNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrTerminalLimit):
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
