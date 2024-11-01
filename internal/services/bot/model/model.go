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
	Model     string             `json:"model" bson:"model"`
	Messages  []provider.Message `json:"messages" bson:"messages"`
	Role      string             `json:"role" bson:"role"`
	CreatedAt time.Time          `json:"created_at" bson:"createdAt"`
	UpdatedAt time.Time          `json:"updated_at" bson:"updatedAt"`
}
