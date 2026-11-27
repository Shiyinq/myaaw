package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"strings"
	"time"

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
	Cols           uint16 `json:"cols"`
	Rows           uint16 `json:"rows"`
	Cwd            string `json:"cwd,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}

// OpenTerminal spawns a login shell for the agent.
// @Summary      Open a terminal
// @Description  Runs a real PTY on the API host, in the agent's home directory or an attached project working directory. Disabled unless the API was started with MYAAW_TERMINAL=1 (myaaw api restart --terminal).
// @Tags         Terminal
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body terminalSizeRequest false "Initial size and optional cwd/conversation_id"
// @Success      200 {object} service.TerminalInfo
// @Failure      400 {object} common.ErrorResponse
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

	agentName := c.Params("name")
	cwd := strings.TrimSpace(body.Cwd)
	if cwd == "" && strings.TrimSpace(body.ConversationID) != "" {
		resolved, err := h.conversations.ResolveWorkingDir(agentName, strings.TrimSpace(body.ConversationID), h.requestUserID(c))
		if err != nil {
			return conversationError(c, err)
		}
		cwd = resolved
	}

	info, err := h.terminals.OpenInDir(agentName, body.Cols, body.Rows, cwd)
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

	stream, err := h.terminals.Attach(name, id)
	if err != nil {
		return terminalError(c, err)
	}

	c.Set("Content-Type", "application/octet-stream")
	c.Set("Cache-Control", "no-store")
	c.Set("X-Accel-Buffering", "no")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		// Detaching does not kill the shell: it keeps running for the grace window
		// so a navigation or reload can attach to it again.
		defer stream.Close()

		// Flush before the first byte: a shell sitting at a prompt produces nothing, and
		// the client should not wait to learn that the stream is open.
		if err := w.Flush(); err != nil {
			return
		}

		for chunk := range stream.Output() {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	})
}

// StreamTerminalsMux multiplexes output for multiple terminal sessions over one
// SSE stream so multi-terminal UIs stay well under browser HTTP/1.1 connection limits.
// @Summary      Multiplexed terminal output stream (SSE)
// @Description  Streams base64-encoded PTY output and close events for multiple terminal sessions over a single text/event-stream connection, with a 1.5s heartbeat ping for fast tab-close detection.
// @Tags         Terminal
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        ids query string true "Comma-separated terminal session IDs"
// @Success      200 {string} string "SSE stream of TerminalMuxEvent JSON payloads"
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/terminal-stream [get]
func (h *Handler) StreamTerminalsMux(c fiber.Ctx) error {
	name := c.Params("name")
	rawIDs := strings.TrimSpace(c.Query("ids"))
	if rawIDs == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ids query parameter is required"})
	}
	ids := strings.Split(rawIDs, ",")

	mux, err := h.terminals.AttachMany(name, ids)
	if err != nil {
		return terminalError(c, err)
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-store")
	c.Set("X-Accel-Buffering", "no")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer mux.Close()

		// Flush an initial SSE comment immediately so fetch resolves headers right away.
		if _, err := w.WriteString(": ok\n\n"); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}

		// Short SSE heartbeat (1.5s) so fasthttp detects a closed browser tab quickly
		// and releases its attached stream slots for immediate eviction/reopening.
		keepalive := time.NewTicker(1500 * time.Millisecond)
		defer keepalive.Stop()

		events := mux.Events()
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return
				}
				payload, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				if _, err := w.WriteString("data: "); err != nil {
					return
				}
				if _, err := w.Write(payload); err != nil {
					return
				}
				if _, err := w.WriteString("\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			case <-keepalive.C:
				if _, err := w.WriteString(": ping\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})
}

// GetTerminal describes a live session, which is how the dashboard reattaches to
// the same shell after a reload or a trip to another page.
// @Summary      Get a terminal session
// @Tags         Terminal
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Session ID"
// @Success      200 {object} service.TerminalInfo
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/terminal/{id} [get]
func (h *Handler) GetTerminal(c fiber.Ctx) error {
	info, err := h.terminals.Info(c.Params("name"), c.Params("id"))
	if err != nil {
		return terminalError(c, err)
	}
	return c.JSON(info)
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
	case errors.Is(err, service.ErrInvalidWorkingDir):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrTerminalDisabled), errors.Is(err, service.ErrTerminalNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrTerminalLimit):
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
