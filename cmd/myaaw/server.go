package main

import (
	"context"
	"fmt"
	"log"
	routes "myaaw/internal"
	"myaaw/internal/channel/discord"
	"myaaw/internal/channel/telegram"
	"myaaw/internal/config"
	"myaaw/internal/daemon"
	"myaaw/internal/middleware"
	"myaaw/internal/services/queue/repository"
	"myaaw/internal/services/queue/service"
	"time"

	_ "myaaw/docs/swagger"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage the web server",
	Long:  "Manage the Fiber web server (run, start, stop, status).",
}

var serverRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run server in foreground",
	Run: func(cmd *cobra.Command, args []string) {
		startServer(cmd.Context())
	},
}

var serverStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start server in background (Daemon)",
	Run: func(cmd *cobra.Command, args []string) {
		dm, err := daemon.NewManager("myaaw-server")
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		runArgs := []string{"server", "run"}
		if err := dm.Start(runArgs); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	},
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background server",
	Run: func(cmd *cobra.Command, args []string) {
		dm, err := daemon.NewManager("myaaw-server")
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		if err := dm.Stop(); err != nil {
			log.Fatalf("Failed to stop server: %v", err)
		}
	},
}

var serverRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the server daemon",
	Run: func(cmd *cobra.Command, args []string) {
		dm, err := daemon.NewManager("myaaw-server")
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		fmt.Println("🔄 Stopping server...")
		if err := dm.Stop(); err != nil {
			fmt.Printf("⚠️  Stop warning: %v\n", err)
		}

		time.Sleep(1 * time.Second)

		fmt.Println("🚀 Starting server...")
		runArgs := []string{"server", "run"}
		if err := dm.Start(runArgs); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	},
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check server status",
	Run: func(cmd *cobra.Command, args []string) {
		dm, err := daemon.NewManager("myaaw-server")
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		pid, running, err := dm.Status()
		if err != nil {
			log.Fatalf("Error checking status: %v", err)
		}

		if running {
			fmt.Printf("✅ Server is running (PID: %d)\n", pid)
		} else {
			fmt.Println("❌ Server is stopped")
		}
	},
}

func init() {
	serverCmd.AddCommand(serverRunCmd)
	serverCmd.AddCommand(serverStartCmd)
	serverCmd.AddCommand(serverStopCmd)
	serverCmd.AddCommand(serverRestartCmd)
	serverCmd.AddCommand(serverStatusCmd)
}

func startServer(ctx context.Context) {
	config.LoadBaseConfig()
	config.ConnectDatabases()
	config.ConnectQueue()

	app := fiber.New(fiber.Config{
		EnablePrintRoutes: false,
	})

	app.Use(middleware.SetupCORS())
	app.Use(middleware.NewLogger())

	app.Get("/", middleware.HelloWorldHandler)
	app.Get("/docs/*", swagger.HandlerDefault)
	routes.SetupRoutes(app)

	app.Use(middleware.NotFoundHandler)

	queueRepo := repository.NewQueueRepository(config.MQ)
	queueServ := service.NewQueueService(queueRepo)

	if config.TelegramBotToken != "" {
		if config.TelegramMode == "polling" {
			go telegram.StartLongPolling(ctx, queueServ)
		} else {
			middleware.SetTelegramWebhook()
		}
	}

	if config.DiscordBotToken != "" {
		adapter, err := discord.NewDiscordAdapter(config.DiscordBotToken)
		if err != nil {
			log.Printf("Failed to create Discord adapter: %v", err)
		} else {
			go func() {
				if err := adapter.StartListener(queueRepo); err != nil {
					log.Printf("Discord listener error: %v", err)
				}
			}()
		}
	}

	log.Fatal(app.Listen(config.PORT))
}
