package handler

import (
	"errors"
	"strings"

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

// BotsSummaryResponse is returned by GET /bots/summary.
type BotsSummaryResponse struct {
	Bots []service.BotSidebarSummary `json:"bots"`
}

// BotsSummary returns the latest DM preview, recent conversations (for unread
// tracking), and active DM runs for all agents in a single request.
// @Summary      Batch bot sidebar summaries
// @Description  Aggregates latest DM preview, recent conversations, and active DM runs for every agent in one request.
// @Tags         Agents
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} BotsSummaryResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      500 {object} common.ErrorResponse
// @Router       /bots/summary [get]
func (h *Handler) BotsSummary(c fiber.Ctx) error {
	agents, err := h.agents.List()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	summaries := service.SummarizeBots(c.Context(), h.conversations, agents, h.auth.UserID())
	return c.JSON(BotsSummaryResponse{Bots: summaries})
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
// @Description  Proxies to the agent server. user_id defaults to the authenticated control-plane user when omitted or 0.
// @Tags         Chat
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body ChatRequest true "Chat request"
// @Success      200 {object} ChatResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/chat [post]
func (h *Handler) Chat(c fiber.Ctx) error {
	if err := h.proxy.Chat(c, c.Params("name"), h.auth.UserID()); err != nil {
		return proxyError(c, err)
	}
	return nil
}

// ChatStream proxies a streaming chat request to the selected agent server.
// @Summary      Stream chat with an agent
// @Description  Relays the agent's Server-Sent Events (thought, chunk, tool, tool_result, done, error). user_id defaults to the authenticated control-plane user when omitted or 0.
// @Tags         Chat
// @Accept       json
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body ChatRequest true "Chat request"
// @Success      200 {string} string "SSE stream"
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/chat/stream [post]
func (h *Handler) ChatStream(c fiber.Ctx) error {
	if err := h.proxy.ChatStream(c, c.Params("name"), h.auth.UserID()); err != nil {
		return proxyError(c, err)
	}
	return nil
}

// ChatStop proxies a stop request to the selected agent.
// @Summary      Stop a running chat
// @Description  Cancels the run started with the same request_id. The partial transcript is kept and the run stops spending provider tokens.
// @Tags         Chat
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body object true "Request id: {\"request_id\":\"...\"}"
// @Success      200 {object} map[string]bool
// @Failure      400 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/chat/stop [post]
func (h *Handler) ChatStop(c fiber.Ctx) error {
	if err := h.proxy.ChatStop(c, c.Params("name"), h.auth.UserID()); err != nil {
		return proxyError(c, err)
	}
	return nil
}

// ChatRuns proxies the list of runs a client can attach to.
// @Summary      List chat runs
// @Description  Lists the runs of the agent, newest first. Finished runs stay listed for a while so a reloaded tab can still replay their tail.
// @Tags         Chat
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        conversation_id query string false "Only runs of this conversation"
// @Success      200 {object} map[string]interface{}
// @Failure      404 {object} common.ErrorResponse
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/chat/runs [get]
func (h *Handler) ChatRuns(c fiber.Ctx) error {
	if err := h.proxy.ChatRuns(c, c.Params("name")); err != nil {
		return proxyError(c, err)
	}
	return nil
}

// ChatRunStream attaches to one run and relays its events.
// @Summary      Attach to a chat run
// @Description  Streams the events of a running (or recently finished) run, replaying everything after `after`. The run keeps going when this connection drops.
// @Tags         Chat
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id   path string true "Request id of the run"
// @Param        after query int false "Skip events before this index" default(0)
// @Success      200
// @Failure      404 {object} common.ErrorResponse
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/chat/run/{id}/stream [get]
func (h *Handler) ChatRunStream(c fiber.Ctx) error {
	if err := h.proxy.ChatRunStream(c, c.Params("name"), c.Params("id")); err != nil {
		return proxyError(c, err)
	}
	return nil
}

// EventsAll multiplexes conversation and approval SSE events for all running
// (or requested) agents over a single stream.
// @Summary      Multiplexed conversation events
// @Description  Server-Sent Events multiplexing `conversation.updated`, `approval.required`, and `approval.resolved` across multiple agents over one connection.
// @Tags         Chat
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        agents query string false "Comma-separated agent names (defaults to all running agents)"
// @Success      200
// @Router       /events [get]
func (h *Handler) EventsAll(c fiber.Ctx) error {
	var names []string
	if raw := strings.TrimSpace(c.Query("agents")); raw != "" {
		names = strings.Split(raw, ",")
	}
	return h.proxy.EventsAll(c, names)
}

// Events streams conversation updates for the selected agent.
// @Summary      Conversation events
// @Description  Server-Sent Events: a `conversation.updated` event is emitted when a cron, heartbeat, or sub-agent turn lands. The client re-reads the transcript.
// @Tags         Chat
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/events [get]
func (h *Handler) Events(c fiber.Ctx) error {
	if err := h.proxy.Events(c, c.Params("name")); err != nil {
		return proxyError(c, err)
	}
	return nil
}

// ApprovalDecision proxies a tool approval decision to the selected agent.
// @Summary      Resolve a tool approval
// @Description  Forwards the decision to the agent that owns the pending run.
// @Tags         Chat
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id   path string true "Approval ID"
// @Param        request body object true "Decision: allow_once | allow_session | allow_always | deny"
// @Success      200
// @Failure      404 {object} common.ErrorResponse
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/approvals/{id}/decision [post]
func (h *Handler) ApprovalDecision(c fiber.Ctx) error {
	if err := h.proxy.ApprovalDecision(c, c.Params("name"), c.Params("id"), h.auth.UserID()); err != nil {
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
