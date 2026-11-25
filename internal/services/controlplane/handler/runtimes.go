package handler

import (
	agentruntime "myaaw/internal/agent/runtime"

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
