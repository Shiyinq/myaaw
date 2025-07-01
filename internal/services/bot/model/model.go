package model

import (
	"teo/internal/provider"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	Id        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	UserId    int                `json:"user_id" bson:"userId"`
	Name      string             `json:"name" bson:"name"`
	System    string             `json:"system" bson:"system"`
	Provider  string             `json:"provider" bson:"provider"`
	Model     string             `json:"model" bson:"model"`
	Messages  []provider.Message `json:"messages" bson:"messages"`
	Role      string             `json:"role" bson:"role"`
	CreatedAt time.Time          `json:"created_at" bson:"createdAt"`
	UpdatedAt time.Time          `json:"updated_at" bson:"updatedAt"`
}

type Conversation struct {
	Id        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	UserId    int                `json:"userId" bson:"userId"`
	Title     string             `json:"title" bson:"title"`
	Messages  []provider.Message `json:"messages" bson:"messages"`
	Active    bool               `json:"active" bson:"active"`
	CreatedAt time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time          `json:"updated_at" bson:"updated_at"`
}
