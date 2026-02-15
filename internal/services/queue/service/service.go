package service

import (
	"encoding/json"
	"myaaw/internal/channel"
	"myaaw/internal/services/queue/repository"
)

type QueueService interface {
	ProcessAndPublishMessage(envelope *channel.QueueEnvelope) error
}

type QueueServiceImpl struct {
	queueRepo repository.QueueRepository
}

func NewQueueService(queueRepo repository.QueueRepository) QueueService {
	return &QueueServiceImpl{queueRepo: queueRepo}
}

func (r *QueueServiceImpl) ProcessAndPublishMessage(envelope *channel.QueueEnvelope) error {
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return r.queueRepo.PublishMessage(body)
}
