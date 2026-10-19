package handler

import (
	"errors"
	"strconv"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListConversations returns a page of conversations for an agent.
// @Summary      List conversations
// @Tags         Conversations
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        user_id query int false "Filter by user ID"
// @Param        limit query int false "Page size (default 50, max 200)"
// @Param        offset query int false "Offset (default 0)"
// @Success      200 {object} service.ConversationPage
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations [get]
func (h *Handler) ListConversations(c fiber.Ctx) error {
	userID, _ := strconv.Atoi(c.Query("user_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))

	page, err := h.conversations.List(c.Params("name"), userID, limit, offset)
	if err != nil {
		return conversationError(c, err)
	}
	return c.JSON(page)
}

// GetConversation returns one conversation with a page of its transcript.
// @Summary      Get conversation
// @Tags         Conversations
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation ID"
// @Param        limit query int false "Message page size (0 = all, max 500)"
// @Param        offset query int false "Message offset (default 0)"
// @Success      200 {object} service.ConversationDetail
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id} [get]
func (h *Handler) GetConversation(c fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))

	detail, err := h.conversations.Get(c.Params("name"), c.Params("id"), limit, offset)
	if err != nil {
		return conversationError(c, err)
	}
	return c.JSON(detail)
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
