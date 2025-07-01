package repository

import (
	"context"
	"teo/internal/provider"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Conversation struct {
	Id        primitive.ObjectID `bson:"_id,omitempty"`
	UserId    int                `bson:"userId"`
	Title     string             `bson:"title"`
	Messages  []provider.Message `bson:"messages"`
	Active    bool               `bson:"active"`
	CreatedAt time.Time          `bson:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"`
}

type ConversationRepository interface {
	GetConversationByUserId(userId int) ([]*Conversation, error)
	CreateConversation(userId int) (*Conversation, error)
	UpdateConversationById(id primitive.ObjectID, messages []provider.Message) error
	GetActiveConversationByUserId(userId int) (*Conversation, error)
}

type ConversationRepositoryImpl struct {
	conversations *mongo.Collection
}

func NewConversationRepository(db *mongo.Database) ConversationRepository {
	return &ConversationRepositoryImpl{conversations: db.Collection("conversations")}
}

func (r *ConversationRepositoryImpl) GetConversationByUserId(userId int) ([]*Conversation, error) {
	filter := bson.M{"userId": userId}
	cur, err := r.conversations.Find(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(context.Background())

	var conversations []*Conversation
	for cur.Next(context.Background()) {
		var conv Conversation
		if err := cur.Decode(&conv); err != nil {
			return nil, err
		}
		conversations = append(conversations, &conv)
	}
	return conversations, nil
}

func (r *ConversationRepositoryImpl) CreateConversation(userId int) (*Conversation, error) {
	filter := bson.M{"userId": userId, "active": true}
	update := bson.M{"$set": bson.M{"active": false}}
	_, _ = r.conversations.UpdateMany(context.Background(), filter, update)

	conversation := &Conversation{
		UserId:    userId,
		Title:     "",
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
	return conversation, nil
}

func (r *ConversationRepositoryImpl) UpdateConversationById(id primitive.ObjectID, messages []provider.Message) error {
	update := bson.M{
		"messages":   messages,
		"updated_at": time.Now(),
	}
	filter := bson.M{"_id": id}
	_, err := r.conversations.UpdateOne(context.Background(), filter, bson.M{"$set": update})
	return err
}

func (r *ConversationRepositoryImpl) GetActiveConversationByUserId(userId int) (*Conversation, error) {
	filter := bson.M{"userId": userId, "active": true}
	var conv Conversation
	err := r.conversations.FindOne(context.Background(), filter).Decode(&conv)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	conv.CreatedAt = time.Time{}
	conv.UpdatedAt = time.Time{}
	return &conv, nil
}
