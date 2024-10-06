package repository

import (
	"context"
	"fmt"
	"teo/internal/common"
	"teo/internal/pkg"
	"teo/internal/services/bot/model"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type UserRepository interface {
	CreateUser(user *model.User) (*model.User, error)
	GetUserById(userId int) (*model.User, error)
	updateUserField(userId int, fields bson.M) error
	UpdateMessages(userId int, messges *[]model.Message) error
	UpdateSystem(userId int, system string) error
	UpdateModel(userId int, model string) error
}

type UserRepositoryImpl struct {
	users *mongo.Collection
	rd    *redis.Client
}

func NewBotRepository(db *mongo.Database, rd *redis.Client) UserRepository {
	return &UserRepositoryImpl{users: db.Collection("users"), rd: rd}
}

func (r *UserRepositoryImpl) GetUserById(userId int) (*model.User, error) {
	cachedUser, err := pkg.GetUserFromRedis(r.rd, userId)
	if err != nil {
		return nil, err
	}

	if cachedUser != nil {
		return cachedUser, nil
	}

	var user model.User
	err = r.users.FindOne(context.Background(), bson.M{"userId": userId}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	err = pkg.SaveUserToRedis(r.rd, &user)
	if err != nil {
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

func (r *UserRepositoryImpl) updateUserField(userId int, fields bson.M) error {
	query := bson.M{"userId": userId}
	update := bson.M{"$set": fields}
	_, err := r.users.UpdateOne(context.Background(), query, update)
	return err
}

func (r *UserRepositoryImpl) updateUserAndCache(userId int, fields bson.M) error {
	timeNow := time.Now()
	fields["updatedAt"] = timeNow

	if err := r.updateUserField(userId, fields); err != nil {
		return err
	}

	user, err := r.GetUserById(userId)
	if err != nil {
		return err
	}

	for key, value := range fields {
		switch key {
		case "messages":
			if messages, ok := value.(*[]model.Message); ok {
				user.Messages = *messages
			} else {
				return fmt.Errorf("expected *[]model.Message, got %T", value)
			}
		case "system":
			user.System = value.(string)
		case "model":
			user.Model = value.(string)
		}
	}
	user.UpdatedAt = timeNow

	return pkg.SaveUserToRedis(r.rd, user)
}

func (r *UserRepositoryImpl) UpdateMessages(userId int, messages *[]model.Message) error {
	fields := bson.M{"messages": messages}
	return r.updateUserAndCache(userId, fields)
}

func (r *UserRepositoryImpl) UpdateSystem(userId int, system string) error {
	fields := bson.M{"system": system}
	return r.updateUserAndCache(userId, fields)
}

func (r *UserRepositoryImpl) UpdateModel(userId int, model string) error {
	fields := bson.M{"model": model}
	return r.updateUserAndCache(userId, fields)
}
