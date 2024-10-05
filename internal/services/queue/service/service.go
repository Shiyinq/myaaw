package service

import (
	"teo/internal/services/queue/model"
	"teo/internal/services/queue/repository"
)

type QueueService interface {
	ProcessAndPublishMessage(msg *model.TelegramIncommingChat) error
}

type QueueServiceImpl struct {
	queueRepo repository.QueueRepository
}

func NewQueueService(queueRepo repository.QueueRepository) QueueService {
	return &QueueServiceImpl{queueRepo: queueRepo}
}

func (r *QueueServiceImpl) ProcessAndPublishMessage(msg *model.TelegramIncommingChat) error {
	return r.queueRepo.PublishMessage(msg)
}
