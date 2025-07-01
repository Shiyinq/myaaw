package pkg

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

	cacheKey := fmt.Sprintf("user_%d", user.UserId)
	expiration := 24 * time.Hour
	err = rd.Set(context.Background(), cacheKey, userData, expiration).Err()
	if err != nil {
		return fmt.Errorf("error saving user to Redis: %w", err)
	}

	return nil
}

func GetUserFromRedis(rd *redis.Client, userId int) (*model.User, error) {
	cacheKey := fmt.Sprintf("user_%d", userId)
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

	return &user, nil
}

func SaveModelNamesToRedis(rd *redis.Client, provider string, models interface{}) error {
	tagsData, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("error serializing model names: %w", err)
	}

	cacheKey := provider + "_model_names"
	expiration := 24 * time.Hour
	err = rd.Set(context.Background(), cacheKey, tagsData, expiration).Err()
	if err != nil {
		return fmt.Errorf("error saving model names to Redis: %w", err)
	}
	fmt.Println("save model names data to redis")
	return nil
}

func GetModelNamesFromRedis(rd *redis.Client, provider string) ([]string, error) {
	cacheKey := provider + "_model_names"
	cachedData, err := rd.Get(context.Background(), cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting model names from Redis: %w", err)
	}

	var models []string
	err = json.Unmarshal([]byte(cachedData), &models)
	if err != nil {
		return nil, fmt.Errorf("error deserializing model names: %w", err)
	}

	fmt.Println("model names data from redis")
	return models, nil
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

func SerializeConversation(conv *model.Conversation) (string, error) {
	data, err := json.Marshal(conv)
	if err != nil {
		return "", fmt.Errorf("error serializing conversation: %w", err)
	}
	return string(data), nil
}

func DeserializeConversation(data string, conv *model.Conversation) error {
	err := json.Unmarshal([]byte(data), conv)
	if err != nil {
		return fmt.Errorf("error deserializing conversation: %w", err)
	}
	return nil
}

func SaveConversationToRedis(rd *redis.Client, conv *model.Conversation) error {
	convData, err := SerializeConversation(conv)
	if err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("conversation_%d_%s", conv.UserId, conv.Id.Hex())
	expiration := 24 * time.Hour
	err = rd.Set(context.Background(), cacheKey, convData, expiration).Err()
	if err != nil {
		return fmt.Errorf("error saving conversation to Redis: %w", err)
	}
	return nil
}

func GetConversationFromRedis(rd *redis.Client, userId int, convId string) (*model.Conversation, error) {
	cacheKey := fmt.Sprintf("conversation_%d_%s", userId, convId)
	cachedData, err := rd.Get(context.Background(), cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting conversation from Redis: %w", err)
	}

	var conv model.Conversation
	err = DeserializeConversation(cachedData, &conv)
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

func SaveConversationsToRedis(rd *redis.Client, userId int, conversations []*model.Conversation) error {
	data, err := json.Marshal(conversations)
	if err != nil {
		return fmt.Errorf("error serializing conversations: %w", err)
	}
	cacheKey := fmt.Sprintf("conversations_%d", userId)
	expiration := 2 * time.Minute
	if err := rd.Set(context.Background(), cacheKey, data, expiration).Err(); err != nil {
		return fmt.Errorf("error saving conversations to Redis: %w", err)
	}
	return nil
}

func GetConversationsFromRedis(rd *redis.Client, userId int) ([]*model.Conversation, error) {
	cacheKey := fmt.Sprintf("conversations_%d", userId)
	cachedData, err := rd.Get(context.Background(), cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting conversations from Redis: %w", err)
	}
	var conversations []*model.Conversation
	if err := json.Unmarshal([]byte(cachedData), &conversations); err != nil {
		return nil, fmt.Errorf("error deserializing conversations: %w", err)
	}
	return conversations, nil
}
