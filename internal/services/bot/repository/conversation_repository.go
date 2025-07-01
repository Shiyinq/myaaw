package repository

import (
	"context"
	"teo/internal/provider"
	"time"

	"teo/internal/pkg"
	"teo/internal/services/bot/model"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type ConversationRepository interface {
	GetConversationByUserId(userId int) ([]*model.Conversation, error)
	CreateConversation(userId int, title string) (*model.Conversation, error)
	UpdateConversationById(id primitive.ObjectID, messages []provider.Message, title string) error
	GetActiveConversationByUserId(userId int) (*model.Conversation, error)
}

type ConversationRepositoryImpl struct {
	conversations *mongo.Collection
	rd            *redis.Client
}

func NewConversationRepository(db *mongo.Database, rd *redis.Client) ConversationRepository {
	return &ConversationRepositoryImpl{conversations: db.Collection("conversations"), rd: rd}
}

func (r *ConversationRepositoryImpl) GetConversationByUserId(userId int) ([]*model.Conversation, error) {
	if cached, err := pkg.GetConversationsFromRedis(r.rd, userId); err == nil && cached != nil {
		return cached, nil
	}

	filter := bson.M{"userId": userId}
	cur, err := r.conversations.Find(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(context.Background())

	var conversations []*model.Conversation
	for cur.Next(context.Background()) {
		var conv model.Conversation
		if err := cur.Decode(&conv); err != nil {
			return nil, err
		}
		conversations = append(conversations, &conv)
	}

	_ = pkg.SaveConversationsToRedis(r.rd, userId, conversations)
	return conversations, nil
}

func (r *ConversationRepositoryImpl) CreateConversation(userId int, title string) (*model.Conversation, error) {
	if title == "" {
		title = "New Chat"
	}
	filter := bson.M{"userId": userId, "active": true}
	update := bson.M{"$set": bson.M{"active": false}}
	_, _ = r.conversations.UpdateMany(context.Background(), filter, update)

	conversation := &model.Conversation{
		UserId:    userId,
		Title:     title,
		Messages:  []provider.Message{},
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	res, err := r.conversations.InsertOne(context.Background(), conversation)
	if err != nil {
		return nil, err
	}
	conversation.Id = res.InsertedID.(primitive.ObjectID)
	_ = pkg.SaveConversationToRedis(r.rd, conversation)
	return conversation, nil
}

func (r *ConversationRepositoryImpl) UpdateConversationById(id primitive.ObjectID, messages []provider.Message, title string) error {
	update := bson.M{
		"messages":   messages,
		"updated_at": time.Now(),
	}
	if title != "" {
		update["title"] = title
	}
	filter := bson.M{"_id": id}
	_, err := r.conversations.UpdateOne(context.Background(), filter, bson.M{"$set": update})
	if err != nil {
		return err
	}
	var conv model.Conversation
	err = r.conversations.FindOne(context.Background(), filter).Decode(&conv)
	if err == nil {
		_ = pkg.SaveConversationToRedis(r.rd, &conv)
	}
	return err
}

func (r *ConversationRepositoryImpl) GetActiveConversationByUserId(userId int) (*model.Conversation, error) {
	filter := bson.M{"userId": userId, "active": true}
	var conv model.Conversation
	err := r.conversations.FindOne(context.Background(), filter).Decode(&conv)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	cached, err := pkg.GetConversationFromRedis(r.rd, userId, conv.Id.Hex())
	if err == nil && cached != nil {
		return cached, nil
	}
	_ = pkg.SaveConversationToRedis(r.rd, &conv)
	conv.CreatedAt = time.Time{}
	conv.UpdatedAt = time.Time{}
	return &conv, nil
}
