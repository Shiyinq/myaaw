package handler

import (
	"errors"
	"strconv"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListFiles browses an agent directory.
// @Summary      Browse agent files
// @Description  Lists the virtual roots (/home, /skills, /jobs) or the entries of one directory.
// @Tags         Files
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name" default(default)
// @Param        path query string false "Virtual path" default(/)
// @Success      200 {object} FileListResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs [get]
func (h *Handler) ListFiles(c fiber.Ctx) error {
	path := c.Query("path")
	entries, err := h.files.List(c.Params("name"), path)
	if err != nil {
		return fileError(c, err)
	}
	if path == "" {
		path = "/"
	}
	return c.JSON(fiber.Map{"path": path, "entries": entries})
}

// ReadFile returns a text file.
// @Summary      Read an agent file
// @Tags         Files
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        path query string true "Virtual path" default(/home/AGENTS.md)
// @Success      200 {object} service.FileContent
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      413 {object} common.ErrorResponse
// @Failure      415 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/read [get]
func (h *Handler) ReadFile(c fiber.Ctx) error {
	content, err := h.files.Read(c.Params("name"), c.Query("path"))
	if err != nil {
		return fileError(c, err)
	}
	return c.JSON(content)
}

// WriteFile writes a text file.
// @Summary      Write an agent file
// @Tags         Files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body WriteFileRequest true "Path and content"
// @Success      200 {object} service.FileContent
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      413 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/write [post]
func (h *Handler) WriteFile(c fiber.Ctx) error {
	var req WriteFileRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	content, err := h.files.Write(c.Params("name"), req.Path, req.Content)
	if err != nil {
		return fileError(c, err)
	}
	return c.JSON(content)
}

// Mkdir creates a directory.
// @Summary      Create an agent directory
// @Tags         Files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body MkdirRequest true "Directory path"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/mkdir [post]
func (h *Handler) Mkdir(c fiber.Ctx) error {
	var req MkdirRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if err := h.files.Mkdir(c.Params("name"), req.Path); err != nil {
		return fileError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// RenameFile moves a file or directory.
// @Summary      Rename or move an agent path
// @Tags         Files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body RenameRequest true "Source and destination"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/rename [post]
func (h *Handler) RenameFile(c fiber.Ctx) error {
	var req RenameRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if err := h.files.Rename(c.Params("name"), req.From, req.To); err != nil {
		return fileError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// DeleteFile removes a file or directory.
// @Summary      Delete an agent path
// @Tags         Files
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        path query string true "Virtual path"
// @Param        recursive query bool false "Delete directories recursively"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs [delete]
func (h *Handler) DeleteFile(c fiber.Ctx) error {
	recursive := false
	if raw := c.Query("recursive"); raw != "" {
		recursive, _ = strconv.ParseBool(raw)
	}
	if err := h.files.Delete(c.Params("name"), c.Query("path"), recursive); err != nil {
		return fileError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// SearchFiles searches text under an agent root.
// @Summary      Search agent files
// @Tags         Files
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        q query string true "Search text"
// @Param        path query string false "Virtual path" default(/home)
// @Success      200 {object} SearchResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/fs/search [get]
func (h *Handler) SearchFiles(c fiber.Ctx) error {
	matches, err := h.files.Search(c.Params("name"), c.Query("path"), c.Query("q"))
	if err != nil {
		return fileError(c, err)
	}
	return c.JSON(fiber.Map{"matches": matches})
}

func fileError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrPathNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "path not found"})
	case errors.Is(err, service.ErrPathTooLarge):
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrBinaryFile):
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidPath), errors.Is(err, service.ErrRootProtected):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
