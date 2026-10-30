package handler

import (
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/fiber/v3"
)

type Handler struct {
	auth          *service.AuthService
	agents        *service.AgentService
	proxy         *service.ProxyService
	files         *service.FileService
	config        *service.ConfigService
	conversations *service.ConversationService
	logs          *service.LogsService
	server        *service.ServerService
	cron          *service.CronService
	skills        *service.SkillService
	tools         *service.ToolsService
	contacts      *service.ContactService
	observability *service.ObservabilityService
	approvals     *service.ApprovalsService
	terminals     *service.TerminalService
}

func New(
	auth *service.AuthService,
	agents *service.AgentService,
	proxy *service.ProxyService,
	files *service.FileService,
	configSvc *service.ConfigService,
	conversations *service.ConversationService,
	logs *service.LogsService,
	server *service.ServerService,
	cronSvc *service.CronService,
	skillsSvc *service.SkillService,
	toolsSvc *service.ToolsService,
	contactsSvc *service.ContactService,
	observabilitySvc *service.ObservabilityService,
	approvalsSvc *service.ApprovalsService,
	terminals *service.TerminalService,
) *Handler {
	return &Handler{
		auth:          auth,
		agents:        agents,
		proxy:         proxy,
		files:         files,
		config:        configSvc,
		conversations: conversations,
		logs:          logs,
		server:        server,
		cron:          cronSvc,
		skills:        skillsSvc,
		tools:         toolsSvc,
		contacts:      contactsSvc,
		observability: observabilitySvc,
		approvals:     approvalsSvc,
		terminals:     terminals,
	}
}

// Health reports whether the control plane is accepting requests.
// @Summary      Health check
// @Tags         System
// @Produce      json
// @Success      200 {object} HealthResponse
// @Router       /health [get]
func (h *Handler) Health(c fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}
