package handler

import (
	"errors"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListSkills returns every skill directory with its effective state.
// @Summary      List skills
// @Description  Reads the skills directory fresh from disk. Enabled state follows skills.json, then the SKILL.md frontmatter, then defaults to enabled.
// @Tags         Skills
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} SkillsResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/skills [get]
func (h *Handler) ListSkills(c fiber.Ctx) error {
	list, err := h.skills.List(c.Params("name"))
	if err != nil {
		return skillError(c, err)
	}
	return c.JSON(fiber.Map{"skills": list})
}

// InstallSkill copies an embedded built-in skill into the agent.
// @Summary      Install a built-in skill
// @Description  Copies the embedded template into the agent's skills directory and records it as enabled in skills.json. An existing folder is never overwritten, so the call is idempotent.
// @Tags         Skills
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        skill path string true "Skill directory name"
// @Success      200 {object} service.SkillDetail
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/skills/{skill}/install [post]
func (h *Handler) InstallSkill(c fiber.Ctx) error {
	detail, err := h.skills.Install(c.Params("name"), c.Params("skill"))
	if err != nil {
		return skillError(c, err)
	}
	return c.JSON(detail)
}

// GetSkill returns one skill's metadata, SKILL.md content, and file listing.
// @Summary      Get skill detail
// @Description  Includes the SKILL.md content and every file in the skill directory (scripts, images, nested folders). A stale skills.json key for a deleted folder is removed automatically.
// @Tags         Skills
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        skill path string true "Skill directory name"
// @Success      200 {object} service.SkillDetail
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      413 {object} common.ErrorResponse
// @Router       /agents/{name}/skills/{skill} [get]
func (h *Handler) GetSkill(c fiber.Ctx) error {
	detail, err := h.skills.Get(c.Params("name"), c.Params("skill"))
	if err != nil {
		return skillError(c, err)
	}
	return c.JSON(detail)
}

// GetSkillFile returns one file inside a skill, base64 for binary files.
// @Summary      Read a skill file
// @Description  Text files return as plain text; binary files (images, compiled scripts) return base64 with binary:true. Paths are relative to the skill directory and cannot escape it.
// @Tags         Skills
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        skill path string true "Skill directory name"
// @Param        path query string true "File path relative to the skill" example(scripts/weather.py)
// @Success      200 {object} service.SkillFileContent
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      413 {object} common.ErrorResponse
// @Router       /agents/{name}/skills/{skill}/file [get]
func (h *Handler) GetSkillFile(c fiber.Ctx) error {
	content, err := h.skills.ReadFile(c.Params("name"), c.Params("skill"), c.Query("path"))
	if err != nil {
		return skillError(c, err)
	}
	return c.JSON(content)
}

// RemoveSkillConfig removes the skills.json override for a skill.
// @Summary      Reset a skill to its default state
// @Description  Removes the skill's skills.json key so the enabled state falls back to the SKILL.md frontmatter or the default. The skill folder is untouched; a missing key is not an error.
// @Tags         Skills
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        skill path string true "Skill directory name"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/skills/{skill}/config [delete]
func (h *Handler) RemoveSkillConfig(c fiber.Ctx) error {
	if err := h.skills.RemoveConfig(c.Params("name"), c.Params("skill")); err != nil {
		return skillError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// ToggleSkill enables or disables a skill via skills.json.
// @Summary      Enable or disable a skill
// @Description  Writes the skill's flag to skills.json. The agent picks up the change on the next conversation without a restart.
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        skill path string true "Skill directory name"
// @Param        request body SkillToggleRequest true "Enabled flag"
// @Success      200 {object} SkillToggleResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/skills/{skill} [patch]
func (h *Handler) ToggleSkill(c fiber.Ctx) error {
	var req SkillToggleRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}
	if req.Enabled == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "enabled is required"})
	}

	name := c.Params("skill")
	if err := h.skills.SetEnabled(c.Params("name"), name, *req.Enabled); err != nil {
		return skillError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true, "name": name, "enabled": *req.Enabled})
}

// DeleteSkill removes a skill directory and its skills.json entry.
// @Summary      Delete a skill
// @Description  Removes the skill folder recursively. Changes made through the terminal are reflected automatically.
// @Tags         Skills
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        skill path string true "Skill directory name"
// @Success      200 {object} OKResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/skills/{skill} [delete]
func (h *Handler) DeleteSkill(c fiber.Ctx) error {
	if err := h.skills.Delete(c.Params("name"), c.Params("skill")); err != nil {
		return skillError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func skillError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrSkillNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "skill not found"})
	case errors.Is(err, service.ErrPathNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "file not found"})
	case errors.Is(err, service.ErrPathTooLarge):
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidPath), errors.Is(err, service.ErrInvalidSkillName):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
