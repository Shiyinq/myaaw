package repository

import (
	"encoding/json"
	"log"
	"teo/internal/config"
	"teo/internal/services/queue/model"

	"github.com/rabbitmq/amqp091-go"
)

type QueueRepository interface {
	PublishMessage(msg *model.TelegramIncommingChat) error
}

type QueueRepositoryImpl struct {
	Channel *amqp091.Channel
	Queue   amqp091.Queue
}

func NewQueueRepository(ch *amqp091.Channel) QueueRepository {
	q, err := ch.QueueDeclare(
		config.QueueName, // Queue name
		false,            // Durable
		false,            // Delete when unused
		false,            // Exclusive
		false,            // No-wait
		nil,              // Arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %s", err)
	}

	return &QueueRepositoryImpl{
		Channel: ch,
		Queue:   q,
	}
}

func (r *QueueRepositoryImpl) PublishMessage(msg *model.TelegramIncommingChat) error {
	body, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal message to JSON: %s", err)
		return err
	}

	err = r.Channel.Publish(
		"",           // Exchange
		r.Queue.Name, // Routing key (queue name)
		false,        // Mandatory
		false,        // Immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)

	if err != nil {
		log.Printf("Failed to publish message: %s", err)
		return err
	}

	return err
}
