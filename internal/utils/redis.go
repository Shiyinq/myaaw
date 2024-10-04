package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"teo/internal/services/bot/model"
	"time"

	"github.com/redis/go-redis/v9"
)

func SerializeUser(user *model.User) (string, error) {
	data, err := json.Marshal(user)
	if err != nil {
		return "", fmt.Errorf("error serializing user: %w", err)
	}
	return string(data), nil
}

func DeserializeUser(data string, user *model.User) error {
	err := json.Unmarshal([]byte(data), user)
	if err != nil {
		return fmt.Errorf("error deserializing user: %w", err)
	}
	return nil
}

func SaveUserToRedis(rd *redis.Client, user *model.User) error {
	userData, err := SerializeUser(user)
	if err != nil {
		return err
	}

	cacheKey := string(user.UserId)
	expiration := 24 * time.Hour
	err = rd.Set(context.Background(), cacheKey, userData, expiration).Err()
	if err != nil {
		return fmt.Errorf("error saving user to Redis: %w", err)
	}
	fmt.Println("saved user data to redis")
	return nil
}

func GetUserFromRedis(rd *redis.Client, userId int) (*model.User, error) {
	cacheKey := string(userId)
	cachedData, err := rd.Get(context.Background(), cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting user from Redis: %w", err)
	}

	var user model.User
	err = DeserializeUser(cachedData, &user)
	if err != nil {
		return nil, err
	}

	fmt.Println("user data from redis")
	return &user, nil
}

func DeleteDataFromRedis(rd *redis.Client, cacheKey string) error {
	err := rd.Del(context.Background(), cacheKey).Err()
	if err != nil {
		return fmt.Errorf("error deleting data from Redis: %w", err)
	}

	return nil
}
