package repository

import (
	"log"
	"myaaw/internal/config"

	"github.com/rabbitmq/amqp091-go"
)

type QueueRepository interface {
	PublishMessage(body []byte) error
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

func (r *QueueRepositoryImpl) PublishMessage(body []byte) error {
	err := r.Channel.Publish(
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

	return nil
}
