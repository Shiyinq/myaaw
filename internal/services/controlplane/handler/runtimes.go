package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agentruntime "myaaw/internal/agent/runtime"
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// RuntimesResponse is returned by GET /runtimes.
type RuntimesResponse struct {
	Runtimes []agentruntime.DetectedRuntime `json:"runtimes"`
}

// RuntimeInstallRequest selects which compiled-in official install method to run.
type RuntimeInstallRequest struct {
	Method string `json:"method"`
}

// RuntimeAuthSessionRequest opens a scoped interactive CLI login/logout PTY.
type RuntimeAuthSessionRequest struct {
	Mode     string `json:"mode"`
	Headless bool   `json:"headless"`
	Cols     uint16 `json:"cols"`
	Rows     uint16 `json:"rows"`
}

// GetRuntimes lists every supported agent runtime preset (native Myaaw plus
// external CLIs such as Antigravity, Claude Code, OMP, Pi, Codex,
// OpenCode, and Custom ACP) along with live host PATH detection.
// @Summary      Discover available agent runtimes
// @Tags         Agents
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} RuntimesResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /runtimes [get]
func (h *Handler) GetRuntimes(c fiber.Ctx) error {
	return c.JSON(RuntimesResponse{
		Runtimes: agentruntime.Discover(c.Context()),
	})
}

// GetManagedRuntimes returns the full external runtime management view for
// Global Settings (install status, official install/update methods, CLI auth
// status, global defaults with masked secrets, and agents using each runtime).
// @Summary      List external runtimes with install, auth, and global defaults
// @Tags         Runtimes
// @Produce      json
// @Security     BearerAuth
// @Param        probe_auth query boolean false "Probe installed CLIs for authentication status (default true unless 0/false)"
// @Param        refresh    query boolean false "Bypass the 60s auth probe cache"
// @Success      200 {object} service.ManagedRuntimesResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /runtimes/manage [get]
func (h *Handler) GetManagedRuntimes(c fiber.Ctx) error {
	if h.runtimes == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "runtime management unavailable"})
	}
	probeRaw := strings.ToLower(strings.TrimSpace(c.Query("probe_auth")))
	probeAuth := probeRaw != "0" && probeRaw != "false" && probeRaw != "no"
	refreshRaw := strings.ToLower(strings.TrimSpace(c.Query("refresh")))
	refresh := refreshRaw == "1" || refreshRaw == "true" || refreshRaw == "yes"
	res, err := h.runtimes.List(c.Context(), probeAuth, refresh)
	if err != nil {
		return runtimeManageError(c, err)
	}
	return c.JSON(res)
}

// GetManagedRuntime returns the refreshed management view (including live auth
// probe) for one external runtime preset.
// @Summary      Get external runtime status and global defaults
// @Tags         Runtimes
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Runtime preset ID"
// @Success      200 {object} service.ManagedRuntimeView
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /runtimes/{id}/manage [get]
func (h *Handler) GetManagedRuntime(c fiber.Ctx) error {
	if h.runtimes == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "runtime management unavailable"})
	}
	view, err := h.runtimes.GetOne(c.Context(), c.Params("id"))
	if err != nil {
		return runtimeManageError(c, err)
	}
	return c.JSON(view)
}

// SaveRuntimeDefaults updates the global default configuration for one external
// runtime preset (and optionally propagates defaults to existing agents).
// @Summary      Save global defaults for an external runtime
// @Tags         Runtimes
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string                       true "Runtime preset ID"
// @Param        body body service.RuntimeDefaultsInput true "Global runtime defaults"
// @Success      200 {object} service.ManagedRuntimeView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /runtimes/{id}/defaults [put]
func (h *Handler) SaveRuntimeDefaults(c fiber.Ctx) error {
	if h.runtimes == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "runtime management unavailable"})
	}
	var body service.RuntimeDefaultsInput
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	view, err := h.runtimes.SaveDefaults(c.Context(), c.Params("id"), body)
	if err != nil {
		return runtimeManageError(c, err)
	}
	return c.JSON(view)
}

// InstallRuntime runs a compiled-in official vendor installer for the given
// runtime preset and streams live progress over Server-Sent Events.
// @Summary      Install an external runtime CLI from its official source
// @Description  Streams SSE events: ready, line, done, error. Only allowlisted official vendor recipes are accepted.
// @Tags         Runtimes
// @Accept       json
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        id   path string                true  "Runtime preset ID"
// @Param        body body RuntimeInstallRequest false "Official install method selection"
// @Success      200 {string} string "SSE stream"
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /runtimes/{id}/install [post]
func (h *Handler) InstallRuntime(c fiber.Ctx) error {
	if h.runtimes == nil || !h.runtimes.Enabled() {
		return runtimeManageError(c, service.ErrRuntimeManagementDisabled)
	}
	presetID := c.Params("id")
	var body RuntimeInstallRequest
	if len(c.Body()) > 0 {
		if err := c.Bind().JSON(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
		}
	}
	ctx := c.Context()

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

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
		_ = writeEvent("ready", map[string]string{"preset": presetID, "action": "install"})
		view, err := h.runtimes.Install(ctx, presetID, body.Method, func(line string) {
			_ = writeEvent("line", map[string]string{"line": line})
		})
		if err != nil {
			_ = writeEvent("error", map[string]string{"error": err.Error()})
			return
		}
		_ = writeEvent("done", view)
	})
}

// UpdateRuntime runs the compiled-in official update command for an installed
// external runtime CLI and streams output over Server-Sent Events.
// @Summary      Update an installed external runtime CLI
// @Description  Streams SSE events: ready, line, done, error.
// @Tags         Runtimes
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        id path string true "Runtime preset ID"
// @Success      200 {string} string "SSE stream"
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /runtimes/{id}/update [post]
func (h *Handler) UpdateRuntime(c fiber.Ctx) error {
	if h.runtimes == nil || !h.runtimes.Enabled() {
		return runtimeManageError(c, service.ErrRuntimeManagementDisabled)
	}
	presetID := c.Params("id")
	ctx := c.Context()

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

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
		_ = writeEvent("ready", map[string]string{"preset": presetID, "action": "update"})
		view, err := h.runtimes.Update(ctx, presetID, func(line string) {
			_ = writeEvent("line", map[string]string{"line": line})
		})
		if err != nil {
			_ = writeEvent("error", map[string]string{"error": err.Error()})
			return
		}
		_ = writeEvent("done", view)
	})
}

// OpenRuntimeAuthSession starts a scoped interactive PTY running the official
// login or logout subcommand of an external runtime CLI.
// @Summary      Start an interactive CLI login or logout session
// @Tags         Runtimes
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string                    true  "Runtime preset ID"
// @Param        body body RuntimeAuthSessionRequest false "Login/logout mode and initial terminal dimensions"
// @Success      200 {object} service.RuntimeAuthSessionInfo
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      429 {object} common.ErrorResponse
// @Router       /runtimes/{id}/auth/session [post]
func (h *Handler) OpenRuntimeAuthSession(c fiber.Ctx) error {
	if h.runtimes == nil || h.runtimes.PTY() == nil {
		return runtimeManageError(c, service.ErrRuntimeManagementDisabled)
	}
	var body RuntimeAuthSessionRequest
	if len(c.Body()) > 0 {
		if err := c.Bind().JSON(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
		}
	}
	info, err := h.runtimes.PTY().Open(c.Params("id"), body.Mode, body.Headless, body.Cols, body.Rows)
	if err != nil {
		return runtimeManageError(c, err)
	}
	return c.JSON(info)
}

// StreamRuntimeAuthSession streams raw PTY output bytes for a scoped CLI auth session.
// @Summary      Stream interactive CLI auth PTY output
// @Tags         Runtimes
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Param        id  path string true "Runtime preset ID"
// @Param        sid path string true "Auth session ID"
// @Success      200 {string} string "raw PTY output"
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /runtimes/{id}/auth/session/{sid}/stream [get]
func (h *Handler) StreamRuntimeAuthSession(c fiber.Ctx) error {
	if h.runtimes == nil || h.runtimes.PTY() == nil {
		return runtimeManageError(c, service.ErrRuntimeManagementDisabled)
	}
	presetID, sid := c.Params("id"), c.Params("sid")
	stream, err := h.runtimes.PTY().Attach(presetID, sid)
	if err != nil {
		return runtimeManageError(c, err)
	}

	c.Set("Content-Type", "application/octet-stream")
	c.Set("Cache-Control", "no-store")
	c.Set("X-Accel-Buffering", "no")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer stream.Close()
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

// InputRuntimeAuthSession forwards raw user keystrokes into a scoped CLI auth PTY.
// @Summary      Send input to an interactive CLI auth PTY
// @Tags         Runtimes
// @Accept       application/octet-stream
// @Produce      json
// @Security     BearerAuth
// @Param        id  path string true "Runtime preset ID"
// @Param        sid path string true "Auth session ID"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /runtimes/{id}/auth/session/{sid}/input [post]
func (h *Handler) InputRuntimeAuthSession(c fiber.Ctx) error {
	if h.runtimes == nil || h.runtimes.PTY() == nil {
		return runtimeManageError(c, service.ErrRuntimeManagementDisabled)
	}
	if err := h.runtimes.PTY().Write(c.Params("id"), c.Params("sid"), c.Body()); err != nil {
		return runtimeManageError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ResizeRuntimeAuthSession updates the window size of a scoped CLI auth PTY.
// @Summary      Resize an interactive CLI auth PTY
// @Tags         Runtimes
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string              true "Runtime preset ID"
// @Param        sid  path string              true "Auth session ID"
// @Param        body body terminalSizeRequest true "Window dimensions"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /runtimes/{id}/auth/session/{sid}/resize [post]
func (h *Handler) ResizeRuntimeAuthSession(c fiber.Ctx) error {
	if h.runtimes == nil || h.runtimes.PTY() == nil {
		return runtimeManageError(c, service.ErrRuntimeManagementDisabled)
	}
	var body terminalSizeRequest
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if err := h.runtimes.PTY().Resize(c.Params("id"), c.Params("sid"), body.Cols, body.Rows); err != nil {
		return runtimeManageError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// CloseRuntimeAuthSession terminates a scoped CLI auth PTY session.
// @Summary      Close an interactive CLI auth PTY session
// @Tags         Runtimes
// @Produce      json
// @Security     BearerAuth
// @Param        id  path string true "Runtime preset ID"
// @Param        sid path string true "Auth session ID"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /runtimes/{id}/auth/session/{sid} [delete]
func (h *Handler) CloseRuntimeAuthSession(c fiber.Ctx) error {
	if h.runtimes == nil || h.runtimes.PTY() == nil {
		return runtimeManageError(c, service.ErrRuntimeManagementDisabled)
	}
	if err := h.runtimes.PTY().Close(c.Params("id"), c.Params("sid")); err != nil {
		return runtimeManageError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// GetAgentRuntimeOptions probes the target agent's configured external CLI
// runtime to dynamically discover available models, thinking/reasoning levels,
// native session modes, and Myaaw approval routing modes.
// @Summary      Discover live runtime options for an agent
// @Tags         Agents
// @Produce      json
// @Security     BearerAuth
// @Param        name     path   string  true   "Agent name"
// @Param        refresh  query  boolean false  "Bypass the 60s cache and re-probe the CLI"
// @Success      200 {object} agentruntime.RuntimeOptions
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/runtime/options [get]
func (h *Handler) GetAgentRuntimeOptions(c fiber.Ctx) error {
	name := c.Params("name")
	refreshRaw := strings.ToLower(strings.TrimSpace(c.Query("refresh")))
	refresh := refreshRaw == "1" || refreshRaw == "true" || refreshRaw == "yes"
	opts, err := h.config.RuntimeOptions(c.Context(), name, refresh)
	if err != nil {
		return configError(c, err)
	}
	return c.JSON(opts)
}

// ProbeRuntimeOptions discovers live runtime options for a runtime selection
// that is not saved yet, so the New Agent form and Global Runtime Settings can
// offer dynamic models, thinking levels, and approval modes.
// @Summary      Discover live runtime options for an unsaved runtime selection
// @Tags         Agents
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        refresh  query  boolean false  "Bypass the 60s cache and re-probe the CLI"
// @Param        body     body   service.OnboardRuntimeInput true "Runtime selection to probe"
// @Success      200 {object} agentruntime.RuntimeOptions
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /runtime/options [post]
func (h *Handler) ProbeRuntimeOptions(c fiber.Ctx) error {
	var req service.OnboardRuntimeInput
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	refreshRaw := strings.ToLower(strings.TrimSpace(c.Query("refresh")))
	refresh := refreshRaw == "1" || refreshRaw == "true" || refreshRaw == "yes"
	opts, err := h.config.ProbeRuntimeOptions(c.Context(), req, refresh)
	if err != nil {
		return configError(c, err)
	}
	return c.JSON(opts)
}

func runtimeManageError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrRuntimeManagementDisabled):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrRuntimeNotFound),
		errors.Is(err, service.ErrRuntimeAuthSessionNotFound),
		errors.Is(err, agentruntime.ErrUnsupportedRuntimePreset):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidConfig),
		errors.Is(err, service.ErrRuntimeBinaryMissing),
		errors.Is(err, agentruntime.ErrUnsupportedInstallMethod),
		errors.Is(err, agentruntime.ErrInstallToolMissing),
		errors.Is(err, agentruntime.ErrUnsupportedAuthAction):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrRuntimeBusy):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrRuntimeAuthSessionLimit):
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
