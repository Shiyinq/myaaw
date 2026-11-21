package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

const groupSSEKeepaliveInterval = 20 * time.Second

// ListGroups returns all multi-agent chat groups sorted by latest activity.
// @Summary      List groups
// @Tags         Groups
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} map[string][]service.GroupSummary
// @Failure      401 {object} common.ErrorResponse
// @Router       /groups [get]
func (h *Handler) ListGroups(c fiber.Ctx) error {
	groups, err := h.groups.List()
	if err != nil {
		return groupError(c, err)
	}
	return c.JSON(fiber.Map{"groups": groups})
}

// CreateGroup creates a new multi-agent chat group.
// @Summary      Create group
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body service.CreateGroupRequest true "Group configuration"
// @Success      201 {object} service.Group
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Router       /groups [post]
func (h *Handler) CreateGroup(c fiber.Ctx) error {
	var req service.CreateGroupRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	group, err := h.groups.Create(req)
	if err != nil {
		return groupError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(group)
}

// GetGroup returns a group's metadata, a page of its transcript, and live run state.
// @Summary      Get group
// @Tags         Groups
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Param        limit query int false "Message page size (0 = all, max 500)"
// @Param        offset query int false "Message offset (default 0, negative starts from the end)"
// @Param        before_id query string false "Return up to limit messages strictly older than this message ID"
// @Success      200 {object} service.GroupDetail
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /groups/{id} [get]
func (h *Handler) GetGroup(c fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	beforeID := strings.TrimSpace(c.Query("before_id"))

	detail, err := h.groups.GetPage(c.Params("id"), limit, offset, beforeID)
	if err != nil {
		return groupError(c, err)
	}
	return c.JSON(detail)
}

// UpdateGroup updates a group's name, description, members, discussion mode, or max_turns.
// @Summary      Update group
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Param        request body service.UpdateGroupRequest true "Group patch"
// @Success      200 {object} service.Group
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /groups/{id} [patch]
func (h *Handler) UpdateGroup(c fiber.Ctx) error {
	var req service.UpdateGroupRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	group, err := h.groups.Update(c.Params("id"), req)
	if err != nil {
		return groupError(c, err)
	}
	return c.JSON(group)
}

// DeleteGroup stops any active run and deletes a group.
// @Summary      Delete group
// @Tags         Groups
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Success      200 {object} map[string]bool
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /groups/{id} [delete]
func (h *Handler) DeleteGroup(c fiber.Ctx) error {
	if err := h.groups.Delete(c.Params("id"), h.auth.UserID()); err != nil {
		return groupError(c, err)
	}
	return c.JSON(fiber.Map{"deleted": true})
}

// ClearGroupMessages clears the group's message history and resets member cursors.
// @Summary      Clear group messages
// @Tags         Groups
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Success      200 {object} map[string]bool
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /groups/{id}/messages [delete]
func (h *Handler) ClearGroupMessages(c fiber.Ctx) error {
	if err := h.groups.ClearMessages(c.Params("id"), h.auth.UserID()); err != nil {
		return groupError(c, err)
	}
	return c.JSON(fiber.Map{"cleared": true})
}

// SendGroupMessage posts a user message to a group and triggers multi-agent orchestration.
// @Summary      Send group message
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Param        request body service.SendGroupMessageRequest true "Message payload"
// @Success      202 {object} map[string]interface{}
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /groups/{id}/messages [post]
func (h *Handler) SendGroupMessage(c fiber.Ctx) error {
	var req service.SendGroupMessageRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	msg, runState, err := h.groups.SendMessage(c.Params("id"), h.auth.UserID(), req)
	if err != nil {
		return groupError(c, err)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"message":   msg,
		"run_state": runState,
	})
}

// StopGroupRun cancels any active autonomous discussion and stops the running agent turn.
// @Summary      Stop group run
// @Tags         Groups
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /groups/{id}/stop [post]
func (h *Handler) StopGroupRun(c fiber.Ctx) error {
	stopped, runState, err := h.groups.Stop(c.Params("id"), h.auth.UserID())
	if err != nil {
		return groupError(c, err)
	}
	return c.JSON(fiber.Map{
		"stopped":   stopped,
		"run_state": runState,
	})
}

// ContinueGroupRun resumes a group run that paused at max_turns.
// @Summary      Continue group run
// @Tags         Groups
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Param        request body service.ContinueGroupRequest false "Extra turns"
// @Success      202 {object} map[string]interface{}
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /groups/{id}/continue [post]
func (h *Handler) ContinueGroupRun(c fiber.Ctx) error {
	var req service.ContinueGroupRequest
	if len(strings.TrimSpace(string(c.Body()))) > 0 {
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}
	}
	runState, err := h.groups.Continue(c.Params("id"), h.auth.UserID(), req.ExtraTurns)
	if err != nil {
		return groupError(c, err)
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"continued": true,
		"run_state": runState,
	})
}

// StreamGroup streams live multi-agent orchestration events over Server-Sent Events.
// @Summary      Stream group events
// @Tags         Groups
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Success      200 {string} string "SSE stream"
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /groups/{id}/stream [get]
func (h *Handler) StreamGroup(c fiber.Ctx) error {
	groupID := c.Params("id")
	events, initial, unsubscribe, err := h.groups.Subscribe(groupID)
	if err != nil {
		return groupError(c, err)
	}

	ctx := c.Context()
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()

		writeEvent := func(name string, data any) error {
			payload, err := json.Marshal(data)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, payload); err != nil {
				return err
			}
			return w.Flush()
		}

		if err := writeEvent("status", map[string]any{
			"group_id":  groupID,
			"run_state": initial,
		}); err != nil {
			return
		}

		ticker := time.NewTicker(groupSSEKeepaliveInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if err := writeEvent(ev.Name, ev.Data); err != nil {
					return
				}
			case <-ticker.C:
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

// ListGroupAttachments returns paginated uploaded attachments for a group.
// @Summary      List group attachments
// @Description  Returns uploaded images and files shared in a group, newest first.
// @Tags         Groups
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Param        limit query int false "Page size (default 50, max 200)"
// @Param        offset query int false "Offset from newest attachment"
// @Param        kind query string false "Comma-separated kind filter (image, video, audio, pdf, file)"
// @Success      200 {object} service.GroupAttachmentsPage
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /groups/{id}/attachments [get]
func (h *Handler) ListGroupAttachments(c fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	kind := c.Query("kind")
	page, err := h.groups.ListAttachments(c.Params("id"), kind, limit, offset)
	if err != nil {
		return groupError(c, err)
	}
	return c.JSON(page)
}

// DownloadGroupUpload serves one uploaded file or image belonging to a group message.
// @Summary      Get group upload
// @Description  Serves an uploaded image or document for a group message. Pass download=1 to force a file download.
// @Tags         Groups
// @Security     BearerAuth
// @Param        id path string true "Group ID"
// @Param        msgId path string true "Message ID"
// @Param        file path string true "Stored file name"
// @Param        download query bool false "Force attachment download"
// @Success      200 {file} binary
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /groups/{id}/uploads/{msgId}/{file} [get]
func (h *Handler) DownloadGroupUpload(c fiber.Ctx) error {
	absPath, fileName, mimeType, err := h.groups.AttachmentFile(c.Params("id"), c.Params("msgId"), c.Params("file"))
	if err != nil {
		return groupError(c, err)
	}
	if mimeType != "" {
		c.Set("Content-Type", mimeType)
	}
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Cache-Control", "private, max-age=3600")
	if c.Query("download") == "1" || c.Query("download") == "true" {
		c.Attachment(fileName)
	}
	return c.SendFile(absPath)
}

func groupError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidGroupID), errors.Is(err, service.ErrGroupValidation):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrGroupNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "group not found"})
	case errors.Is(err, service.ErrGroupNotPaused):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
