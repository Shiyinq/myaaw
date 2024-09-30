package repository

import (
	"context"
	"teo/internal/common"
	"teo/internal/services/bot/model"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type UserRepository interface {
	CreateUser(user *model.User) (*model.User, error)
	GetUserById(userId int) (*model.User, error)
	UpdateMessages(userID int, messges *[]model.Message) error
	UpdateSystem(userID int, system string) error
}

type UserRepositoryImpl struct {
	users *mongo.Collection
}

func NewBotRepository(db *mongo.Database) UserRepository {
	return &UserRepositoryImpl{users: db.Collection("users")}
}

func (r *UserRepositoryImpl) GetUserById(userId int) (*model.User, error) {
	var user model.User
	err := r.users.FindOne(context.Background(), bson.M{"userId": userId}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}

func (r *UserRepositoryImpl) CreateUser(user *model.User) (*model.User, error) {
	user.System = common.RoleSystemDefault()
	user.Model = common.ModelDefault()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	newuser, err := r.users.InsertOne(context.Background(), user)
	if err != nil {
		return nil, err
	}

	user.Id = newuser.InsertedID.(primitive.ObjectID)
	currentUser, err := r.GetUserById(user.UserId)

	if err != nil {
		return nil, err
	}

	return currentUser, nil
}

func (r *UserRepositoryImpl) UpdateMessages(userId int, messages *[]model.Message) error {
	var query = bson.M{"userId": userId}
	var update = bson.M{"$set": bson.M{"messages": messages, "updatedAt": time.Now()}}
	_, err := r.users.UpdateOne(context.Background(), query, update)
	if err != nil {
		return err
	}

	return nil
}

func (r *UserRepositoryImpl) UpdateSystem(userId int, system string) error {
	var query = bson.M{"userId": userId}
	var update = bson.M{"$set": bson.M{"system": system, "updatedAt": time.Now()}}
	_, err := r.users.UpdateOne(context.Background(), query, update)
	if err != nil {
		return err
	}

	return nil
}
