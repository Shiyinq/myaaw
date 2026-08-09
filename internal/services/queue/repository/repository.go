package repository

type QueueRepository interface {
	PublishMessage(body []byte) error
}

// In-memory queue
var WebhookQueue = make(chan []byte, 1000)

type QueueRepositoryImpl struct{}

func NewQueueRepository() QueueRepository {
	return &QueueRepositoryImpl{}
}

func (r *QueueRepositoryImpl) PublishMessage(body []byte) error {
	// Push to the in-memory queue
	WebhookQueue <- body
	return nil
}
