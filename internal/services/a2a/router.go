package a2a

import (
	"context"
	"log"

	"myaaw/internal/config"

	"github.com/gofiber/fiber/v3"
)

// Router mounts the A2A discovery and JSON-RPC endpoints on the agent server.
func Router(router fiber.Router, runner Runner) {
	if config.DB == nil {
		log.Printf("a2a: database is not available, skipping routes")
		return
	}
	service := NewService(NewTaskStore(config.DB), runner)
	if err := service.EnsureSchema(); err != nil {
		log.Printf("a2a: failed to prepare schema: %v", err)
		return
	}
	if resumed, err := service.ResumePending(context.Background()); err != nil {
		log.Printf("a2a: failed to resume pending tasks: %v", err)
	} else if resumed > 0 {
		log.Printf("a2a: resumed %d pending task(s)", resumed)
	}
	handler := NewHandler(service)

	router.Get("/.well-known/agent.json", handler.Card)
	router.Get("/.well-known/agent-card.json", handler.Card)
	router.Post("/a2a", handler.RPC)
}
