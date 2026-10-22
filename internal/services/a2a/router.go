package a2a

import (
	"context"
	"log"

	"myaaw/internal/config"

	"github.com/gofiber/fiber/v3"
)

// SetupService builds and initializes the shared A2A service, or returns nil
// when the database is unavailable. The returned service is shared across the
// JSON-RPC, REST, and gRPC bindings so the agent loop is never run
// concurrently by two service instances.
func SetupService(runner Runner) *Service {
	if config.DB == nil {
		log.Printf("a2a: database is not available, skipping routes")
		return nil
	}
	service := NewService(NewTaskStore(config.DB), runner)
	if err := service.EnsureSchema(); err != nil {
		log.Printf("a2a: failed to prepare schema: %v", err)
		return nil
	}
	if resumed, err := service.ResumePending(context.Background()); err != nil {
		log.Printf("a2a: failed to resume pending tasks: %v", err)
	} else if resumed > 0 {
		log.Printf("a2a: resumed %d pending task(s)", resumed)
	}
	return service
}

// MountRouter mounts the JSON-RPC and REST endpoints plus the Agent Card on
// the given router, reusing an already-initialized service.
func MountRouter(router fiber.Router, service *Service) {
	handler := NewHandler(service)
	router.Get("/.well-known/agent-card.json", handler.Card)
	router.Post("/a2a", handler.RPC)
	mountRest(router, handler)
}

// Router is a convenience wrapper that sets up the service and mounts the
// HTTP routes.
func Router(router fiber.Router, runner Runner) {
	if service := SetupService(runner); service != nil {
		MountRouter(router, service)
	}
}
