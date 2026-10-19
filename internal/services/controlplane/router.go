package controlplane

import (
	"os"
	"strings"

	_ "myaaw/docs/controlplane"
	"myaaw/internal/agentdir"
	"myaaw/internal/services/controlplane/handler"
	cpMiddleware "myaaw/internal/services/controlplane/middleware"
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/contrib/v3/swaggo"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

// NewApp builds the control-plane Fiber app. Public routes are registered
// first, then the bearer-token middleware, then the secured routes.
func NewApp() (*fiber.App, error) {
	if err := agentdir.EnsureLayout(); err != nil {
		return nil, err
	}

	auth, err := service.NewAuthService()
	if err != nil {
		return nil, err
	}
	agents := service.NewAgentService()
	proxy := service.NewProxyService(agents)
	files := service.NewFileService()
	configSvc := service.NewConfigService()
	conversations := service.NewConversationService()
	logs := service.NewLogsService()
	serverSvc := service.NewServerService(agents)
	cronSvc := service.NewCronService(agents)
	skillsSvc := service.NewSkillService()
	toolsSvc := service.NewToolsService()
	h := handler.New(auth, agents, proxy, files, configSvc, conversations, logs, serverSvc, cronSvc, skillsSvc, toolsSvc)

	app := fiber.New(fiber.Config{AppName: "myaaw api"})
	app.Use(cors.New(cors.Config{
		AllowOrigins: allowedOrigins(),
		AllowMethods: []string{"GET", "POST", "HEAD", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
	}))

	app.Get("/health", h.Health)
	app.Get("/docs", func(c fiber.Ctx) error {
		return c.Redirect().To("/docs/index.html")
	})
	app.Get("/docs/*", swaggo.New(swaggo.Config{
		InstanceName: "controlplane",
		Title:        "Myaaw Control Plane API",
	}))
	app.Post("/auth/login", h.Login)

	app.Use(cpMiddleware.RequireAuth(auth))

	app.Get("/auth/me", h.Me)
	app.Post("/auth/logout", h.Logout)
	app.Post("/auth/change-password", h.ChangePassword)
	app.Put("/auth/username", h.ChangeUsername)

	app.Get("/agents", h.ListAgents)
	app.Post("/agents", h.CreateAgent)
	app.Get("/agents/:name", h.GetAgent)
	app.Delete("/agents/:name", h.DeleteAgent)
	app.Post("/agents/:name/default", h.SetDefaultAgent)
	app.Post("/agents/:name/chat", h.Chat)
	app.Post("/agents/:name/chat/stream", h.ChatStream)

	app.Get("/agents/:name/fs", h.ListFiles)
	app.Get("/agents/:name/fs/read", h.ReadFile)
	app.Get("/agents/:name/fs/search", h.SearchFiles)
	app.Get("/agents/:name/context", h.ListContextFiles)
	app.Get("/agents/:name/context/file", h.ReadContextFile)
	app.Put("/agents/:name/context/file", h.WriteContextFile)

	app.Get("/agents/:name/skills", h.ListSkills)
	app.Get("/agents/:name/skills/:skill", h.GetSkill)
	app.Patch("/agents/:name/skills/:skill", h.ToggleSkill)
	app.Delete("/agents/:name/skills/:skill", h.DeleteSkill)

	app.Get("/agents/:name/tools", h.ListTools)
	app.Post("/agents/:name/tools/custom", h.CreateCustomTool)
	app.Patch("/agents/:name/tools/custom/:tool", h.PatchCustomTool)
	app.Delete("/agents/:name/tools/custom/:tool", h.DeleteCustomTool)
	app.Post("/agents/:name/tools/mcp", h.CreateMCPServer)
	app.Patch("/agents/:name/tools/mcp/:tool", h.PatchMCPServer)
	app.Delete("/agents/:name/tools/mcp/:tool", h.DeleteMCPServer)
	app.Patch("/agents/:name/tools/builtin/:tool", h.ToggleBuiltinTool)
	app.Post("/agents/:name/fs/write", h.WriteFile)
	app.Post("/agents/:name/fs/mkdir", h.Mkdir)
	app.Post("/agents/:name/fs/rename", h.RenameFile)
	app.Delete("/agents/:name/fs", h.DeleteFile)

	app.Get("/agents/:name/providers", h.ListProviders)
	app.Post("/agents/:name/providers", h.CreateProvider)
	app.Put("/agents/:name/providers/:id", h.UpdateProvider)
	app.Delete("/agents/:name/providers/:id", h.DeleteProvider)
	app.Post("/agents/:name/providers/default", h.SetDefaultProvider)
	app.Get("/agents/:name/channels", h.ListChannels)
	app.Put("/agents/:name/channels/:channel", h.UpsertChannel)
	app.Delete("/agents/:name/channels/:channel", h.DeleteChannel)
	app.Get("/agents/:name/config", h.GetConfig)
	app.Get("/agents/:name/conversations", h.ListConversations)
	app.Get("/agents/:name/conversations/:id", h.GetConversation)
	app.Patch("/agents/:name/conversations/:id", h.RenameConversation)
	app.Delete("/agents/:name/conversations/:id", h.DeleteConversation)
	app.Get("/agents/:name/logs", h.ListLogs)
	app.Get("/agents/:name/logs/stream", h.StreamLogs)
	app.Get("/agents/:name/server/status", h.GetServerStatus)
	app.Post("/agents/:name/server/start", h.StartServer)
	app.Post("/agents/:name/server/stop", h.StopServer)
	app.Post("/agents/:name/server/restart", h.RestartServer)
	app.Get("/agents/:name/cron", h.ListCron)
	app.Post("/agents/:name/cron", h.AddCron)
	app.Delete("/agents/:name/cron/:id", h.RemoveCron)
	app.Post("/agents/:name/cron/:id/run", h.RunCron)
	app.Get("/agents/:name/cron/:id/history", h.CronHistory)

	return app, nil
}

func Run(addr string) error {
	app, err := NewApp()
	if err != nil {
		return err
	}
	return app.Listen(addr)
}

func allowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
	if raw == "" || raw == "*" {
		return []string{"*"}
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	if len(origins) == 0 {
		return []string{"*"}
	}
	return origins
}
