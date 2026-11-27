package handler

import (
	"errors"
	"strings"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

type WorkspaceMkdirRequest struct {
	Parent string `json:"parent"`
	Name   string `json:"name"`
}

// GetWorkspaceRoots returns the API host identity and shortcut directories
// that can be browsed when attaching a working directory to a chat.
// @Summary      List host workspace roots
// @Tags         Workspace
// @Produce      json
// @Security     BearerAuth
// @Param        agent query string false "Optional agent name for agent workspace shortcut"
// @Success      200 {object} service.WorkspaceRootsResponse
// @Router       /workspace/roots [get]
func (h *Handler) GetWorkspaceRoots(c fiber.Ctx) error {
	resp, err := h.workspace.Roots(strings.TrimSpace(c.Query("agent")))
	if err != nil {
		return workspaceError(c, err)
	}
	return c.JSON(resp)
}

// BrowseWorkspace lists safe subdirectories on the API host for the folder picker.
// @Summary      Browse host directories
// @Tags         Workspace
// @Produce      json
// @Security     BearerAuth
// @Param        agent query string false "Optional agent name"
// @Param        path query string false "Absolute directory path on the API host"
// @Param        hidden query bool false "Include dotfiles/hidden folders"
// @Success      200 {object} service.WorkspaceBrowseResponse
// @Router       /workspace/browse [get]
func (h *Handler) BrowseWorkspace(c fiber.Ctx) error {
	showHidden := queryBool(c.Query("hidden"))
	resp, err := h.workspace.Browse(
		strings.TrimSpace(c.Query("agent")),
		strings.TrimSpace(c.Query("path")),
		showHidden,
	)
	if err != nil {
		return workspaceError(c, err)
	}
	return c.JSON(resp)
}

// CreateWorkspaceFolder creates an empty directory inside an allowed parent on the API host.
// @Summary      Create empty workspace directory
// @Tags         Workspace
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body WorkspaceMkdirRequest true "Parent path and new folder name"
// @Success      201 {object} map[string]string
// @Router       /agents/{name}/workspace/mkdir [post]
func (h *Handler) CreateWorkspaceFolder(c fiber.Ctx) error {
	var req WorkspaceMkdirRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	created, err := h.workspace.Mkdir(c.Params("name"), req.Parent, req.Name)
	if err != nil {
		return workspaceError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"path": created})
}

func workspaceError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrHostBrowseDisabled):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrWorkspaceInvalid):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
