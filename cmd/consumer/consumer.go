package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/joho/godotenv"
	"github.com/rabbitmq/amqp091-go"
)

func connectRabbitMQ(rabbitMQURL string) (*amqp091.Connection, *amqp091.Channel, error) {
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
	client.SetTimeout(90 * time.Second)

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(jsonBody).
		Post(fmt.Sprintf("%s/webhook/bot", os.Getenv("TEO_BASE_URL")))

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("received error response from webhook: %s", resp.Status())
	}

	log.Println("Message forwarded to webhook bot successfully")
	return nil
}

func consumeMessages(ch *amqp091.Channel, queueName string) error {
	msgs, err := ch.Consume(
		queueName,
		"",    // Consumer
		true,  // Auto-ack
		false, // Exclusive
		false, // No-local
		false, // No-wait
		nil,   // Args
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

func main() {
	err := godotenv.Load()
	log.Println("Load .env file")
	if err != nil {
		log.Println("Error loading .env file, using environment variables")
	}

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	conn, ch, err := connectRabbitMQ(rabbitMQURL)
	if err != nil {
		log.Fatalf("Error: %s", err)
	}
	defer conn.Close()
	defer ch.Close()

	queueName := os.Getenv("QUEUE_NAME")
	q, err := ch.QueueDeclare(
		queueName, // Queue name
		false,     // Durable
		false,     // Delete when unused
		false,     // Exclusive
		false,     // No-wait
		nil,       // Args
	)
	if err != nil {
		log.Fatalf("Failed to declare queue: %s", err)
	}

	err = consumeMessages(ch, q.Name)
	if err != nil {
		log.Fatalf("Error in consumer: %s", err)
	}
}
