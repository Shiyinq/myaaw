package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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

	cacheKey := strconv.Itoa(user.UserId)
	expiration := 24 * time.Hour
	err = rd.Set(context.Background(), cacheKey, userData, expiration).Err()
	if err != nil {
		return fmt.Errorf("error saving user to Redis: %w", err)
	}
	fmt.Println("save user data to redis")
	return nil
}

func GetUserFromRedis(rd *redis.Client, userId int) (*model.User, error) {
	cacheKey := strconv.Itoa(userId)
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

func SaveOllamaTagsToRedis(rd *redis.Client, tags *model.OllamaTagsResponse) error {
	tagsData, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("error serializing ollama tags: %w", err)
	}

	cacheKey := "ollama_tags"
	expiration := 24 * time.Hour
	err = rd.Set(context.Background(), cacheKey, tagsData, expiration).Err()
	if err != nil {
		return fmt.Errorf("error saving ollama tags to Redis: %w", err)
	}
	fmt.Println("save ollama tags data to redis")
	return nil
}

func GetOllamaTagsFromRedis(rd *redis.Client) (*model.OllamaTagsResponse, error) {
	cacheKey := "ollama_tags"
	cachedData, err := rd.Get(context.Background(), cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting ollama tags from Redis: %w", err)
	}

	var tags model.OllamaTagsResponse
	err = json.Unmarshal([]byte(cachedData), &tags)
	if err != nil {
		return nil, fmt.Errorf("error deserializing ollama tags: %w", err)
	}

	fmt.Println("ollama tags data from redis")
	return &tags, nil
}

func SetChattingStatus(rd *redis.Client, userId int) error {
	cacheKey := strconv.Itoa(userId) + "_chatting"
	expiration := 2 * time.Minute
	err := rd.Set(context.Background(), cacheKey, true, expiration).Err()
	if err != nil {
		return fmt.Errorf("error setting chatting status in Redis: %w", err)
	}
	fmt.Println("save chatting status to redis")
	return nil
}

func IsUserChatting(rd *redis.Client, userId int) (bool, error) {
	cacheKey := strconv.Itoa(userId) + "_chatting"
	_, err := rd.Get(context.Background(), cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, fmt.Errorf("error retrieving chatting status from Redis: %w", err)
	}
	fmt.Println("user is temporarily blocked from chatting")
	return true, nil
}

func DeleteDataFromRedis(rd *redis.Client, cacheKey string) error {
	err := rd.Del(context.Background(), cacheKey).Err()
	if err != nil {
		return fmt.Errorf("error deleting data from Redis: %w", err)
	}

	return nil
}
