package service

import (
	"teo/internal/pkg"
	"teo/internal/services/queue/repository"
)

type QueueService interface {
	ProcessAndPublishMessage(msg *pkg.TelegramIncommingChat) error
}

type QueueServiceImpl struct {
	queueRepo repository.QueueRepository
}

func NewQueueService(queueRepo repository.QueueRepository) QueueService {
	return &QueueServiceImpl{queueRepo: queueRepo}
}

func (r *QueueServiceImpl) ProcessAndPublishMessage(msg *pkg.TelegramIncommingChat) error {
	return r.queueRepo.PublishMessage(msg)
}
