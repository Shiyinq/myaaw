package handler

import (
	"errors"
	"strconv"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListConversations returns recent conversations for an agent.
// @Summary      List conversations
// @Tags         Conversations
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        user_id query int false "Filter by user ID"
// @Param        limit query int false "Maximum results (default 50)"
// @Success      200 {object} ConversationsResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations [get]
func (h *Handler) ListConversations(c fiber.Ctx) error {
	userID, _ := strconv.Atoi(c.Query("user_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	conversations, err := h.conversations.List(c.Params("name"), userID, limit)
	if err != nil {
		return conversationError(c, err)
	}
	return c.JSON(fiber.Map{"conversations": conversations})
}

// GetConversation returns one conversation with its transcript.
// @Summary      Get conversation
// @Tags         Conversations
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Success      200 {object} ConversationResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id} [get]
func (h *Handler) GetConversation(c fiber.Ctx) error {
	conversation, err := h.conversations.Get(c.Params("name"), c.Params("id"))
	if err != nil {
		return conversationError(c, err)
	}
	return c.JSON(conversation)
}

func conversationError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrConversationNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
