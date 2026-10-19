package handler

import (
	"github.com/gofiber/fiber/v3"
)

// ListContextFiles returns the agent's core context files.
// @Summary      List context files
// @Description  Returns the allowed core Markdown files and every .md file under memory/. Other .md files are never listed.
// @Tags         Context
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} ContextFilesResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/context [get]
func (h *Handler) ListContextFiles(c fiber.Ctx) error {
	files, err := h.files.ListContext(c.Params("name"))
	if err != nil {
		return fileError(c, err)
	}
	return c.JSON(fiber.Map{"files": files})
}

// ReadContextFile returns one core context file.
// @Summary      Read a context file
// @Description  Reads an allowed core Markdown file or any .md file under memory/.
// @Tags         Context
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        path query string true "Home-relative path" example(AGENTS.md)
// @Success      200 {object} service.ContextContent
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      413 {object} common.ErrorResponse
// @Failure      415 {object} common.ErrorResponse
// @Router       /agents/{name}/context/file [get]
func (h *Handler) ReadContextFile(c fiber.Ctx) error {
	content, err := h.files.ReadContext(c.Params("name"), c.Query("path"))
	if err != nil {
		return fileError(c, err)
	}
	return c.JSON(content)
}

// WriteContextFile creates or updates a context file.
// @Summary      Update a context file
// @Description  Writes an allowed core Markdown file or any .md file under memory/. Parent directories under memory/ are created automatically.
// @Tags         Context
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body WriteContextRequest true "Path and content"
// @Success      200 {object} service.ContextContent
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      413 {object} common.ErrorResponse
// @Router       /agents/{name}/context/file [put]
func (h *Handler) WriteContextFile(c fiber.Ctx) error {
	var req WriteContextRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	content, err := h.files.WriteContext(c.Params("name"), req.Path, req.Content)
	if err != nil {
		return fileError(c, err)
	}
	return c.JSON(content)
}
