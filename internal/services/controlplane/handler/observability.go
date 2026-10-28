package handler

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"myaaw/internal/obs"
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

// ObservabilitySummary returns run totals, token usage, cost, latency
// percentiles, and per-turn/per-model/per-tool breakdowns.
// @Summary      Observability summary
// @Tags         Observability
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        days query int false "Window in days (default 7)"
// @Success      200 {object} obs.Summary
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/observability/summary [get]
func (h *Handler) ObservabilitySummary(c fiber.Ctx) error {
	days := 7
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "days must be a positive integer"})
		}
		days = parsed
	}

	summary, err := h.observability.Summary(c.Params("name"), days)
	if err != nil {
		return observabilityError(c, err)
	}
	return c.JSON(summary)
}

// ObservabilityRuns lists recorded runs.
// @Summary      List runs
// @Tags         Observability
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        limit query int false "Page size (default 50)"
// @Param        offset query int false "Page offset"
// @Param        status query string false "Filter by status"
// @Param        channel query string false "Filter by channel"
// @Param        conversation query string false "Filter by conversation id (one session)"
// @Param        from query int false "Start of the window (unix ms)"
// @Param        to query int false "End of the window (unix ms)"
// @Success      200 {object} service.RunList
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/observability/runs [get]
func (h *Handler) ObservabilityRuns(c fiber.Ctx) error {
	filter := obs.RunFilter{
		Status:         c.Query("status"),
		Channel:        c.Query("channel"),
		ConversationID: c.Query("conversation"),
	}

	var err error
	if filter.Limit, err = queryInt(c, "limit", 50); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if filter.Offset, err = queryInt(c, "offset", 0); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if filter.From, err = queryInt64(c, "from", 0); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if filter.To, err = queryInt64(c, "to", 0); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	runs, total, err := h.observability.Runs(c.Params("name"), filter)
	if err != nil {
		return observabilityError(c, err)
	}
	return c.JSON(service.RunList{Runs: runs, Total: total, Limit: filter.Limit, Offset: filter.Offset})
}

// ObservabilitySessions lists sessions, grouped by conversation.
// @Summary      List sessions
// @Tags         Observability
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        limit query int false "How many sessions (default 50)"
// @Success      200 {object} service.SessionList
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/observability/sessions [get]
func (h *Handler) ObservabilitySessions(c fiber.Ctx) error {
	limit, err := queryInt(c, "limit", 50)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	sessions, err := h.observability.Sessions(c.Params("name"), limit)
	if err != nil {
		return observabilityError(c, err)
	}
	return c.JSON(service.SessionList{Sessions: sessions})
}

// ObservabilitySession returns one session with every turn, oldest first.
// @Summary      Get session
// @Tags         Observability
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Conversation id"
// @Param        spans query bool false "Include each turn's spans (default true)"
// @Success      200 {object} service.SessionDetail
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/observability/sessions/{id} [get]
func (h *Handler) ObservabilitySession(c fiber.Ctx) error {
	includeSpans := true
	switch strings.ToLower(c.Query("spans")) {
	case "false", "0", "no":
		includeSpans = false
	}

	detail, err := h.observability.Session(c.Params("name"), c.Params("id"), includeSpans)
	if err != nil {
		return observabilityError(c, err)
	}
	return c.JSON(detail)
}

// ObservabilityRun returns one run with its spans and nested tree.
// @Summary      Get run trace
// @Tags         Observability
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        id path string true "Run id"
// @Success      200 {object} service.RunDetail
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/observability/runs/{id} [get]
func (h *Handler) ObservabilityRun(c fiber.Ctx) error {
	detail, err := h.observability.Run(c.Params("name"), c.Params("id"))
	if err != nil {
		return observabilityError(c, err)
	}
	return c.JSON(detail)
}

// DeleteObservabilityRuns removes runs older than the given time. Without
// `before` it removes everything.
// @Summary      Delete runs
// @Tags         Observability
// @Produce      json
// @Security     BearerAuth
// @Param        name path string true "Agent name"
// @Param        before query int false "Delete runs started before this unix ms"
// @Success      200 {object} map[string]int64
// @Failure      400 {object} common.ErrorResponse
// @Failure      401 {object} common.ErrorResponse
// @Failure      404 {object} common.ErrorResponse
// @Router       /agents/{name}/observability/runs [delete]
func (h *Handler) DeleteObservabilityRuns(c fiber.Ctx) error {
	before := time.Now()
	if c.Query("before") != "" {
		millis, err := queryInt64(c, "before", 0)
		if err != nil || millis <= 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "before must be a unix millisecond timestamp"})
		}
		before = time.UnixMilli(millis)
	}

	removed, err := h.observability.DeleteBefore(c.Params("name"), before)
	if err != nil {
		return observabilityError(c, err)
	}
	return c.JSON(fiber.Map{"removed": removed})
}

func observabilityError(c fiber.Ctx, err error) error {
	if errors.Is(err, service.ErrObservabilityNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return configWriteError(c, err)
}

func queryInt(c fiber.Ctx, name string, fallback int) (int, error) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	return value, nil
}

func queryInt64(c fiber.Ctx, name string, fallback int64) (int64, error) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	return value, nil
}
