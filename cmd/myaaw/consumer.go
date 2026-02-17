package main

import (
	"fmt"
	"log"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/daemon"
	"myaaw/internal/heartbeat"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rabbitmq/amqp091-go"
	"github.com/spf13/cobra"
)

var consumerCmd = &cobra.Command{
	Use:   "consumer",
	Short: "Manage the message consumer",
	Long:  "Manage the RabbitMQ message consumer (run, start, stop, status).",
}

var consumerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run consumer in foreground",
	Run: func(cmd *cobra.Command, args []string) {
		startConsumer()
	},
}

var consumerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start consumer in background (Daemon)",
	Run: func(cmd *cobra.Command, args []string) {
		dm, err := daemon.NewManager("myaaw-consumer")
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		runArgs := []string{"consumer", "run"}
		if err := dm.Start(runArgs); err != nil {
			log.Fatalf("Failed to start consumer: %v", err)
		}
	},
}

var consumerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background consumer",
	Run: func(cmd *cobra.Command, args []string) {
		dm, err := daemon.NewManager("myaaw-consumer")
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		if err := dm.Stop(); err != nil {
			log.Fatalf("Failed to stop consumer: %v", err)
		}
	},
}

var consumerRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the consumer daemon",
	Run: func(cmd *cobra.Command, args []string) {
		dm, err := daemon.NewManager("myaaw-consumer")
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		fmt.Println(theme.RenderSecondary("🔄 Stopping consumer..."))
		if err := dm.Stop(); err != nil {
			fmt.Printf("%s: %v\n", theme.RenderError("⚠️  Stop warning"), err)
		}

		time.Sleep(1 * time.Second)

		fmt.Println(theme.RenderPrimary("🚀 Starting consumer..."))
		runArgs := []string{"consumer", "run"}
		if err := dm.Start(runArgs); err != nil {
			log.Fatalf("Failed to start consumer: %v", err)
		}
	},
}

var consumerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check consumer status",
	Run: func(cmd *cobra.Command, args []string) {
		dm, err := daemon.NewManager("myaaw-consumer")
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		pid, running, err := dm.Status()
		if err != nil {
			log.Fatalf("Error checking status: %v", err)
		}

		if running {
			fmt.Printf("%s (PID: %d)\n", theme.RenderSuccess("✅ Consumer is running"), pid)
		} else {
			fmt.Println(theme.RenderError("❌ Consumer is stopped"))
		}
	},
}

func init() {
	consumerCmd.AddCommand(consumerRunCmd)
	consumerCmd.AddCommand(consumerStartCmd)
	consumerCmd.AddCommand(consumerStopCmd)
	consumerCmd.AddCommand(consumerRestartCmd)
	consumerCmd.AddCommand(consumerStatusCmd)
}

func startConsumer() {
	config.LoadConfig()

	if config.Heartbeat.Active {
		hb := heartbeat.NewHeartbeatService()
		go hb.Start()
	}

	ch := config.MQ
	queueName := config.QueueName

	_, err := ch.QueueDeclare(
		queueName,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to declare queue: %s", err)
	}

	err = consumeMessages(ch, queueName)
	if err != nil {
		log.Fatalf("Error in consumer: %s", err)
	}
}

func sendToWebhookBot(jsonBody []byte) error {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	baseURL := os.Getenv("MYAAW_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(jsonBody).
		Post(fmt.Sprintf("%s/webhook/bot", baseURL))

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("received error response from webhook: %s %v", resp.Status(), resp.String())
	}

	log.Println("Message forwarded to webhook bot successfully")
	return nil
}

func consumeMessages(ch *amqp091.Channel, queueName string) error {
	msgs, err := ch.Consume(
		queueName,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	log.Println("Waiting for messages. To exit press CTRL+C")

	for msg := range msgs {
		log.Printf("Received message: %s", msg.Body)

		err := sendToWebhookBot(msg.Body)
		if err != nil {
			log.Printf("Failed to send message to webhook: %s", err)
		}
	}

	return nil
}
