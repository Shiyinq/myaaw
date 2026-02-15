package main

import (
	"fmt"
	"log"
	"myaaw/internal/heartbeat"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/joho/godotenv"
	"github.com/rabbitmq/amqp091-go"
	"github.com/spf13/cobra"
)

var consumerCmd = &cobra.Command{
	Use:   "consumer",
	Short: "Start the message consumer",
	Long:  "Start the RabbitMQ message consumer that forwards messages to the webhook endpoint.",
	Run: func(cmd *cobra.Command, args []string) {
		err := godotenv.Load()
		log.Println("Load .env file")
		if err != nil {
			log.Println("Error loading .env file, using environment variables")
		}

		heartbeatService := heartbeat.NewHeartbeatService()

		rabbitMQURL := os.Getenv("RABBITMQ_URL")
		conn, ch, err := consumerConnectRabbitMQ(rabbitMQURL)
		if err != nil {
			log.Fatalf("Error: %s", err)
		}
		defer conn.Close()
		defer ch.Close()

		queueName := os.Getenv("QUEUE_NAME")
		q, err := ch.QueueDeclare(
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

		if heartbeatService != nil {
			go heartbeatService.Start()
		}

		err = consumeMessages(ch, q.Name)
		if err != nil {
			log.Fatalf("Error in consumer: %s", err)
		}
	},
}

func consumerConnectRabbitMQ(rabbitMQURL string) (*amqp091.Connection, *amqp091.Channel, error) {
	conn, err := amqp091.Dial(rabbitMQURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}
	log.Println("Connected to RabbitMQ!")

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open channel: %w", err)
	}
	return conn, ch, nil
}

func sendToWebhookBot(jsonBody []byte) error {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(jsonBody).
		Post(fmt.Sprintf("%s/webhook/bot", os.Getenv("MYAAW_BASE_URL")))

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
