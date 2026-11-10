package handler

import (
	"encoding/json"
	"errors"
	"strings"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListChatFolders returns the user's chat folders with a chat count each.
// @Summary      List chat folders
// @Description  Folders are scoped to the authenticated user (or the user_id query).
// @Tags         Chat folders
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} service.ChatFolderList
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/chat-folders [get]
func (h *Handler) ListChatFolders(c fiber.Ctx) error {
	folders, err := h.conversations.ListFolders(c.Params("name"), h.requestUserID(c))
	if err != nil {
		return folderError(c, err)
	}
	return c.JSON(service.ChatFolderList{Folders: folders})
}

type folderNameRequest struct {
	Name string `json:"name"`
}

// CreateChatFolder creates a folder for grouping conversations.
// @Summary      Create a chat folder
// @Description  Names are trimmed, unique per user ignoring case, and limited to 64 characters. A user may keep up to 100 folders.
// @Tags         Chat folders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name  path string             true "Agent name"
// @Param        body  body folderNameRequest  true "Folder name"
// @Success      201 {object} service.ChatFolder
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /agents/{name}/chat-folders [post]
func (h *Handler) CreateChatFolder(c fiber.Ctx) error {
	var body folderNameRequest
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}
	folder, err := h.conversations.CreateFolder(c.Params("name"), h.requestUserID(c), body.Name)
	if err != nil {
		return folderError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(folder)
}

// RenameChatFolder changes a folder's name; its conversations stay inside.
// @Summary      Rename a chat folder
// @Tags         Chat folders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name   path string             true "Agent name"
// @Param        folder path string             true "Folder ID"
// @Param        body   body folderNameRequest  true "New folder name"
// @Success      200 {object} service.ChatFolder
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /agents/{name}/chat-folders/{folder} [patch]
func (h *Handler) RenameChatFolder(c fiber.Ctx) error {
	var body folderNameRequest
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}
	folder, err := h.conversations.RenameFolder(
		c.Params("name"), c.Params("folder"), h.requestUserID(c), body.Name,
	)
	if err != nil {
		return folderError(c, err)
	}
	return c.JSON(folder)
}

// DeleteChatFolder deletes a folder and every conversation inside it.
// @Summary      Delete a chat folder
// @Description  Removes the folder and its conversations in one transaction. deleted_ids lets a client leave an open transcript.
// @Tags         Chat folders
// @Produce      json
// @Security     BearerAuth
// @Param        name   path string true "Agent name"
// @Param        folder path string true "Folder ID"
// @Success      200 {object} service.ChatFolderDelete
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/chat-folders/{folder} [delete]
func (h *Handler) DeleteChatFolder(c fiber.Ctx) error {
	result, err := h.conversations.DeleteFolder(
		c.Params("name"), c.Params("folder"), h.requestUserID(c),
	)
	if err != nil {
		return folderError(c, err)
	}
	return c.JSON(result)
}

type pinRequest struct {
	Pinned *bool `json:"pinned"`
}

// PinConversation pins or unpins one conversation.
// @Summary      Pin a conversation
// @Description  Pinning sets pinned_at to now; unpinning clears it. It never changes updated_at.
// @Tags         Conversations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string     true "Agent name"
// @Param        id   path string     true "Conversation ID"
// @Param        body body pinRequest true "Pinned state"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/pin [put]
func (h *Handler) PinConversation(c fiber.Ctx) error {
	var body pinRequest
	if err := c.Bind().JSON(&body); err != nil || body.Pinned == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "pinned is required"})
	}
	if err := h.conversations.SetPinned(
		c.Params("name"), c.Params("id"), h.requestUserID(c), *body.Pinned,
	); err != nil {
		return conversationFolderError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// moveRequest is the body of the move endpoint; the handler parses the raw
// JSON so a missing key (`400`) is distinguishable from an explicit null
// (move to Recents).
type moveRequest struct {
	FolderID *string `json:"folder_id"`
}

// MoveConversation moves a conversation into a folder, or to Recents with a
// null folder_id.
// @Summary      Move a conversation
// @Description  folder_id must belong to the same user; null moves the chat out of its folder.
// @Tags         Conversations
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string      true "Agent name"
// @Param        id   path string      true "Conversation ID"
// @Param        body body moveRequest true "Target folder (null for Recents)"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      403 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/conversations/{id}/folder [put]
func (h *Handler) MoveConversation(c fiber.Ctx) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(c.Body(), &payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON"})
	}
	raw, ok := payload["folder_id"]
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder_id is required"})
	}

	var folderID *string
	if string(raw) != "null" {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder_id must be a string or null"})
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder_id must not be empty"})
		}
		folderID = &value
	}

	if err := h.conversations.SetFolder(
		c.Params("name"), c.Params("id"), h.requestUserID(c), folderID,
	); err != nil {
		return conversationFolderError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func folderError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrFolderNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "folder not found"})
	case errors.Is(err, service.ErrFolderNameTaken), errors.Is(err, service.ErrTooManyFolders):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidFolderName):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}

func conversationFolderError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrConversationNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
	case errors.Is(err, service.ErrConversationForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrFolderNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "folder not found"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
