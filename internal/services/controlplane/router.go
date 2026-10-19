package controlplane

import (
	_ "myaaw/docs/controlplane"
	"myaaw/internal/agentdir"
	"myaaw/internal/services/controlplane/handler"
	cpMiddleware "myaaw/internal/services/controlplane/middleware"
	"myaaw/internal/services/controlplane/service"

	"github.com/gofiber/contrib/v3/swaggo"
	"github.com/gofiber/fiber/v3"
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
	h := handler.New(auth, agents, proxy, files, configSvc, conversations, logs)

	app := fiber.New(fiber.Config{AppName: "myaaw api"})

	app.Get("/health", h.Health)
	app.Get("/docs", func(c fiber.Ctx) error {
		return c.Redirect().To("/docs/")
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
	app.Get("/agents/:name", h.GetAgent)
	app.Post("/agents/:name/chat", h.Chat)
	app.Post("/agents/:name/chat/stream", h.ChatStream)

	app.Get("/agents/:name/fs", h.ListFiles)
	app.Get("/agents/:name/fs/read", h.ReadFile)
	app.Get("/agents/:name/fs/search", h.SearchFiles)
	app.Post("/agents/:name/fs/write", h.WriteFile)
	app.Post("/agents/:name/fs/mkdir", h.Mkdir)
	app.Post("/agents/:name/fs/rename", h.RenameFile)
	app.Delete("/agents/:name/fs", h.DeleteFile)

	app.Get("/agents/:name/providers", h.ListProviders)
	app.Get("/agents/:name/channels", h.ListChannels)
	app.Get("/agents/:name/config", h.GetConfig)
	app.Get("/agents/:name/conversations", h.ListConversations)
	app.Get("/agents/:name/conversations/:id", h.GetConversation)
	app.Get("/agents/:name/logs", h.ListLogs)
	app.Get("/agents/:name/logs/stream", h.StreamLogs)

	return app, nil
}

func Run(addr string) error {
	app, err := NewApp()
	if err != nil {
		return err
	}
	return app.Listen(addr)
}
