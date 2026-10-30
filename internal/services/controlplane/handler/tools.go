package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListTools returns built-in, custom, and MCP tools from tools.json.
// @Summary      List tools
// @Description  Built-in tools are the canonical set. Custom tools and MCP servers come only from tools.json; files inside the tools directory are storage, not active tools.
// @Tags         Tools
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} service.ToolsView
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/tools [get]
func (h *Handler) ListTools(c fiber.Ctx) error {
	view, err := h.tools.List(c.Params("name"))
	if err != nil {
		return toolError(c, err)
	}
	return c.JSON(view)
}

// ToggleBuiltinTool enables or disables a built-in tool.
// @Summary      Enable or disable a built-in tool
// @Description  Built-in tools can only be toggled, never deleted. Accepted names: bash, filesystem, cron, execute_python (alias python), provider, subagent.
// @Tags         Tools
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        tool path string true "Built-in tool name"
// @Param        request body BuiltinToggleRequest true "Enabled flag"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/tools/builtin/{tool} [patch]
func (h *Handler) ToggleBuiltinTool(c fiber.Ctx) error {
	var req BuiltinToggleRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if req.Enabled == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "enabled is required"})
	}
	if err := h.tools.ToggleBuiltin(c.Params("name"), c.Params("tool"), *req.Enabled); err != nil {
		return toolError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// CreateCustomTool adds a custom_tools entry.
// @Summary      Create a custom tool
// @Description  Registers a command as a custom tool. The command may live anywhere; storing it under the tools folder does not activate it until it is added here.
// @Tags         Tools
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body CreateCustomToolRequest true "Tool definition"
// @Success      200 {object} service.CustomToolView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /agents/{name}/tools/custom [post]
func (h *Handler) CreateCustomTool(c fiber.Ctx) error {
	var req CreateCustomToolRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	view, err := h.tools.CreateCustom(c.Params("name"), service.CustomToolInput{
		Name:    req.Name,
		Command: req.Command,
		Args:    req.Args,
		Schema:  req.Schema,
		Enabled: req.Enabled,
	})
	if err != nil {
		return toolError(c, err)
	}
	return c.JSON(view)
}

// PatchCustomTool updates a custom_tools entry.
// @Summary      Update a custom tool
// @Description  Updates enabled, command, args, or schema. Only provided fields change; schema null removes an inline schema.
// @Tags         Tools
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        tool path string true "Custom tool name"
// @Param        request body PatchCustomToolRequest true "Fields to update"
// @Success      200 {object} service.CustomToolView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/tools/custom/{tool} [patch]
func (h *Handler) PatchCustomTool(c fiber.Ctx) error {
	var req PatchCustomToolRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if req.Enabled == nil && req.Command == nil && req.Args == nil && req.Schema == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nothing to update"})
	}
	view, err := h.tools.PatchCustom(c.Params("name"), c.Params("tool"), service.CustomToolPatch{
		Enabled: req.Enabled,
		Command: req.Command,
		Args:    req.Args,
		Schema:  req.Schema,
	})
	if err != nil {
		return toolError(c, err)
	}
	return c.JSON(view)
}

// DeleteCustomTool removes a custom_tools entry.
// @Summary      Delete a custom tool
// @Description  Removes the tools.json entry only; files in the tools directory are never touched.
// @Tags         Tools
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        tool path string true "Custom tool name"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/tools/custom/{tool} [delete]
func (h *Handler) DeleteCustomTool(c fiber.Ctx) error {
	if err := h.tools.DeleteCustom(c.Params("name"), c.Params("tool")); err != nil {
		return toolError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// CreateMCPServer adds an mcp_servers entry.
// @Summary      Create an MCP server
// @Description  Registers an stdio MCP server. Env values are accepted on input and masked on output.
// @Tags         Tools
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body CreateMCPRequest true "Server definition"
// @Success      200 {object} service.MCPServerView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      409 {object} common.ErrorResponse
// @Router       /agents/{name}/tools/mcp [post]
func (h *Handler) CreateMCPServer(c fiber.Ctx) error {
	var req CreateMCPRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	view, err := h.tools.CreateMCP(c.Params("name"), service.MCPServerInput{
		Name:    req.Name,
		Command: req.Command,
		Args:    req.Args,
		Env:     req.Env,
		Enabled: req.Enabled,
	})
	if err != nil {
		return toolError(c, err)
	}
	return c.JSON(view)
}

// PatchMCPServer updates an mcp_servers entry.
// @Summary      Update an MCP server
// @Description  Updates enabled, command, args, or env. A provided env map replaces the previous one entirely.
// @Tags         Tools
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        tool path string true "MCP server name"
// @Param        request body PatchMCPRequest true "Fields to update"
// @Success      200 {object} service.MCPServerView
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/tools/mcp/{tool} [patch]
func (h *Handler) PatchMCPServer(c fiber.Ctx) error {
	var req PatchMCPRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if req.Enabled == nil && req.Command == nil && req.Args == nil && req.Env == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "nothing to update"})
	}
	view, err := h.tools.PatchMCP(c.Params("name"), c.Params("tool"), service.MCPServerPatch{
		Enabled: req.Enabled,
		Command: req.Command,
		Args:    req.Args,
		Env:     req.Env,
	})
	if err != nil {
		return toolError(c, err)
	}
	return c.JSON(view)
}

// DeleteMCPServer removes an mcp_servers entry.
// @Summary      Delete an MCP server
// @Description  Removes the tools.json entry only; a running server process stops on the next agent server reload or restart.
// @Tags         Tools
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        tool path string true "MCP server name"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/tools/mcp/{tool} [delete]
func (h *Handler) DeleteMCPServer(c fiber.Ctx) error {
	if err := h.tools.DeleteMCP(c.Params("name"), c.Params("tool")); err != nil {
		return toolError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func toolError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrToolNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "tool not found"})
	case errors.Is(err, service.ErrToolNameTaken):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrUnknownBuiltin),
		errors.Is(err, service.ErrInvalidToolName),
		errors.Is(err, service.ErrInvalidToolConfig):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
