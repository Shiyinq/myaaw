package handler

import (
	agentruntime "myaaw/internal/agent/runtime"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// RuntimesResponse is returned by GET /runtimes.
type RuntimesResponse struct {
	Runtimes []agentruntime.DetectedRuntime `json:"runtimes"`
}

// GetRuntimes lists every supported agent runtime preset (native Myaaw plus
// external CLIs such as Antigravity, Claude Code, OMP, Pi, Codex, Gemini CLI,
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
