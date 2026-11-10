package main

import (
	"context"
	"fmt"
	"log"
	routes "myaaw/internal"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel/discord"
	"myaaw/internal/channel/telegram"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/daemon"
	"myaaw/internal/middleware"
	"myaaw/internal/services/queue/repository"
	"myaaw/internal/services/queue/service"
	"os"
	"time"

	_ "myaaw/docs/swagger"

	"github.com/gofiber/contrib/v3/swaggo"
	"github.com/gofiber/fiber/v3"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage the agent server",
	Long:  "Manage the per-agent Fiber server (run, start, stop, status).",
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
	Short: "Start the bot as a persistent background service",
	Run: func(cmd *cobra.Command, args []string) {
		targets, err := serverAgentTargets()
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		fmt.Println(theme.RenderPrimary("Starting agent servers"))
		failed := 0
		for _, target := range targets {
			if err := startServiceTarget(target); err != nil {
				failed++
				printTargetError(target, "start", err)
			}
		}
		finishService(failed, "agents", "started")
	},
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop and unregister the background service",
	Run: func(cmd *cobra.Command, args []string) {
		targets, err := serverAgentTargets()
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		fmt.Println(theme.RenderSecondary("Stopping agent servers"))
		failed := 0
		for _, target := range reverseTargets(targets) {
			if err := stopServiceTarget(target); err != nil {
				failed++
				printTargetError(target, "stop", err)
			}
		}
		finishService(failed, "agents", "stopped")
	},
}

var serverRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the background service",
	Run: func(cmd *cobra.Command, args []string) {
		targets, err := serverAgentTargets()
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		fmt.Println(theme.RenderPrimary("Restarting agent servers"))
		failed := 0
		for _, target := range reverseTargets(targets) {
			if err := stopServiceTarget(target); err != nil {
				failed++
				printTargetError(target, "stop", err)
			}
		}
		time.Sleep(1 * time.Second)
		for _, target := range targets {
			if err := startServiceTarget(target); err != nil {
				failed++
				printTargetError(target, "start", err)
			}
		}
		finishService(failed, "agents", "restarted")
	},
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check server status",
	Run: func(cmd *cobra.Command, args []string) {
		targets, err := serverAgentTargets()
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		fmt.Println(theme.RenderPrimary("Agent server status"))
		width := 0
		for _, target := range targets {
			if len(target.name) > width {
				width = len(target.name)
			}
		}
		failed := 0
		for _, target := range targets {
			label := fmt.Sprintf("%q", target.name)
			pid, running, err := agentDaemonStatus(target.name)
			if err != nil {
				failed++
				fmt.Printf("  agent %-*s  error: %v\n", width+2, label, err)
				continue
			}
			if running {
				fmt.Printf("  agent %-*s  running (PID: %d, Port: %s)\n", width+2, label, pid, agentPort(target.name))
			} else {
				fmt.Printf("  agent %-*s  stopped\n", width+2, label)
			}
		}
		if failed > 0 {
			os.Exit(1)
		}
	},
}

func init() {
	serverCmd.AddCommand(serverRunCmd)
	serverCmd.AddCommand(serverStartCmd)
	serverCmd.AddCommand(serverStopCmd)
	serverCmd.AddCommand(serverRestartCmd)
	serverCmd.AddCommand(serverStatusCmd)

	for _, cmd := range []*cobra.Command{serverStartCmd, serverStopCmd, serverRestartCmd, serverStatusCmd} {
		cmd.Flags().BoolVar(&serviceAllAgents, "all-agent", false, "operate on every registered agent")
	}
}

// serverAgentTargets lists the agents a `server` command operates on: every
// registered agent with --all-agent, otherwise the selected (or default) one.
func serverAgentTargets() ([]serviceTarget, error) {
	if serviceAllAgents {
		names, err := agentdir.ListAgents()
		if err != nil {
			return nil, err
		}
		targets := make([]serviceTarget, 0, len(names))
		for _, name := range names {
			targets = append(targets, serviceTarget{kind: targetAgent, name: name})
		}
		return targets, nil
	}
	return []serviceTarget{{kind: targetAgent, name: agentdir.Active()}}, nil
}

// agentRunArgs is the command line the supervisor runs for one agent.
func agentRunArgs(agentName string) []string {
	return []string{"--agent", agentName, "server", "run"}
}

// stopAgent stops and unregisters one agent's server daemon.
func stopAgent(agentName string) error {
	dm, err := daemon.NewAgentManager(agentName)
	if err != nil {
		return err
	}
	return dm.Stop()
}

func agentDaemonStatus(agentName string) (int, bool, error) {
	dm, err := daemon.NewAgentManager(agentName)
	if err != nil {
		return 0, false, err
	}
	return dm.Status()
}

func startServer(ctx context.Context) {
	config.LoadBaseConfig()
	config.ConnectDatabases()

	// A2A carries inline file parts (base64) in the request body, so the
	// default 4 MiB Fiber limit would reject valid messages before the A2A
	// handler ever sees them.
	app := fiber.New(fiber.Config{BodyLimit: config.A2AMaxRequestBytes})

	app.Use(middleware.SetupCORS())
	app.Use(middleware.NewLogger())

	app.Get("/", middleware.HelloWorldHandler)
	app.Get("/docs/*", swaggo.New())
	routes.SetupRoutes(app)

	app.Use(middleware.NotFoundHandler)

	queueRepo := repository.NewQueueRepository()
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
				if err := adapter.StartListener(ctx, queueRepo); err != nil {
					log.Printf("Discord listener error: %v", err)
				}
			}()
		}
	}

	go startBackgroundWorkers()

	log.Fatal(app.Listen(config.PORT))
}
