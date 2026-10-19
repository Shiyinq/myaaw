package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListAgents returns all initialized agents.
// @Summary      List agents
// @Description  Returns every initialized agent with its default flag, running state, port, and base URL.
// @Tags         Agents
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} AgentsResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      500 {object} common.ErrorResponse
// @Router       /agents [get]
func (h *Handler) ListAgents(c fiber.Ctx) error {
	agents, err := h.agents.List()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(fiber.Map{"agents": agents})
}

// GetAgent returns one agent.
// @Summary      Get agent
// @Tags         Agents
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} service.AgentInfo
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name} [get]
func (h *Handler) GetAgent(c fiber.Ctx) error {
	agent, err := h.agents.Get(c.Params("name"))
	if err != nil {
		return proxyError(c, err)
	}
	return c.JSON(agent)
}

// Chat proxies a non-streaming chat request to the selected agent server.
// @Summary      Chat with an agent
// @Tags         Chat
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body ChatRequest true "Chat request"
// @Success      200 {object} ChatResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/chat [post]
func (h *Handler) Chat(c fiber.Ctx) error {
	if err := h.proxy.Chat(c, c.Params("name")); err != nil {
		return proxyError(c, err)
	}
	return nil
}

// ChatStream proxies a streaming chat request to the selected agent server.
// @Summary      Stream chat with an agent
// @Description  Relays the agent's Server-Sent Events (thought, chunk, tool, tool_result, done, error).
// @Tags         Chat
// @Accept       json
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body ChatRequest true "Chat request"
// @Success      200 {string} string "SSE stream"
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/chat/stream [post]
func (h *Handler) ChatStream(c fiber.Ctx) error {
	if err := h.proxy.ChatStream(c, c.Params("name")); err != nil {
		return proxyError(c, err)
	}
	return nil
}

func proxyError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "agent not found",
		})
	case errors.Is(err, service.ErrAgentUnavailable):
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "agent server is not running",
		})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
}
