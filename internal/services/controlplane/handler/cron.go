package handler

import (
	"errors"
	"strconv"

	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ListCron returns the agent's scheduled jobs.
// @Summary      List cron jobs
// @Tags         Cron
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Success      200 {object} CronJobsResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/cron [get]
func (h *Handler) ListCron(c fiber.Ctx) error {
	jobs, err := h.cron.List(c.Params("name"))
	if err != nil {
		return cronError(c, err)
	}
	return c.JSON(fiber.Map{"jobs": jobs})
}

// AddCron creates a scheduled job.
// @Summary      Create cron job
// @Tags         Cron
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        request body service.CronJobInput true "Job definition"
// @Success      201 {object} CronJobResponse
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/cron [post]
func (h *Handler) AddCron(c fiber.Ctx) error {
	var req service.CronJobInput
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON request body"})
	}

	job, err := h.cron.Add(c.Params("name"), req)
	if err != nil {
		return cronError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(job)
}

// RemoveCron deletes a scheduled job.
// @Summary      Delete cron job
// @Tags         Cron
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Job ID or prefix"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/cron/{id} [delete]
func (h *Handler) RemoveCron(c fiber.Ctx) error {
	if err := h.cron.Remove(c.Params("name"), c.Params("id")); err != nil {
		return cronError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// RunCron triggers a job immediately through the agent server.
// @Summary      Run cron job
// @Tags         Cron
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Job ID or prefix"
// @Success      200 {object} OKResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Failure      502 {object} common.ErrorResponse
// @Router       /agents/{name}/cron/{id}/run [post]
func (h *Handler) RunCron(c fiber.Ctx) error {
	if err := h.cron.Run(c.Params("name"), c.Params("id")); err != nil {
		return cronError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

// CronHistory returns recent executions of a job.
// @Summary      Cron job history
// @Tags         Cron
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Job ID or prefix"
// @Param        limit query int false "Maximum entries (default 20)"
// @Success      200 {object} CronHistoryResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/cron/{id}/history [get]
func (h *Handler) CronHistory(c fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit"))
	history, err := h.cron.History(c.Params("name"), c.Params("id"), limit)
	if err != nil {
		return cronError(c, err)
	}
	return c.JSON(fiber.Map{"history": history})
}

func cronError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrCronJobNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrAgentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "agent not found"})
	case errors.Is(err, service.ErrInvalidConfig):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrAgentUnavailable):
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "agent server is not running"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}
