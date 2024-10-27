package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var PORT string
var AllowedOrigins string
var DB *mongo.Database
var QueueName string
var MQ *amqp091.Channel
var OwnerId string
var BotType string
var BotToken string
var RedisClient *redis.Client
var LLMProviderBaseURL string
var LLMProviderName string
var LLMProviderAPIKey string
var StreamResponse bool

func envPath() string {
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Join(filepath.Dir(b), "../..")
	envPath := filepath.Join(basePath, ".env")
	return envPath
}

func LoadConfig() {
	path := envPath()
	err := godotenv.Load(path)
	log.Println("Load .env file", path)
	if err != nil {
		log.Println("Error loading .env file, using environment variables")
	}

	PORT = ":" + os.Getenv("PORT")
	AllowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	mongoURI := os.Getenv("MONGODB_URI")
	dbName := os.Getenv("DB_NAME")
	redisURL := os.Getenv("REDIS_URL")
	QueueName = os.Getenv("QUEUE_NAME")
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	OwnerId = os.Getenv("OWNER_ID")
	BotType = os.Getenv("BOT_TYPE")
	BotToken = os.Getenv("BOT_TOKEN")
	LLMProviderBaseURL = os.Getenv("LLM_PROVIDER_BASE_URL")
	LLMProviderName = os.Getenv("LLM_PROVIDER_NAME")
	LLMProviderAPIKey = os.Getenv("LLM_PROVIDER_API_KEY")
	maxRetries := 10
	retryDelay := 3 * time.Second

	StreamResponse, err = strconv.ParseBool(os.Getenv("STREAM_RESPONSE"))
	if err != nil {
		log.Fatalf("Invalid value for STREAM_RESPONSE: %v", err)
	}

	if AllowedOrigins == "" {
		AllowedOrigins = "*"
	}

	ConnectMongoDB(mongoURI, dbName, maxRetries, retryDelay)
	ConnectRedis(redisURL, maxRetries, retryDelay)
	ConnectRabbitMQ(rabbitMQURL, maxRetries, retryDelay)
}

func retry(attempts int, delay time.Duration, fn func() error) error {
	for i := 0; i < attempts; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		log.Printf("Attempt %d failed: %v. Retrying in %v...\n", i+1, err, delay)
		time.Sleep(delay)
	}
	return fmt.Errorf("failed after %d attempts", attempts)
}

func ConnectMongoDB(mongoURI, dbName string, maxRetries int, retryDelay time.Duration) {
	err := retry(maxRetries, retryDelay, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		clientOptions := options.Client().ApplyURI(mongoURI)
		client, err := mongo.Connect(ctx, clientOptions)
		if err != nil {
			return err
		}

		err = client.Ping(ctx, nil)
		if err != nil {
			return err
		}

		DB = client.Database(dbName)
		log.Println("Connected to MongoDB!")
		return nil
	})

	if err != nil {
		log.Fatal("MongoDB connection failed:", err)
	}
}

func ConnectRedis(redisURL string, maxRetries int, retryDelay time.Duration) {
	err := retry(maxRetries, retryDelay, func() error {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			return err
		}

		RedisClient = redis.NewClient(opt)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err = RedisClient.Ping(ctx).Result()
		if err != nil {
			return err
		}

		log.Println("Connected to Redis!")
		return nil
	})

	if err != nil {
		log.Fatal("Redis connection failed:", err)
	}
}

func ConnectRabbitMQ(rabbitMQURL string, maxRetries int, retryDelay time.Duration) {
	err := retry(maxRetries, retryDelay, func() error {
		conn, err := amqp091.Dial(rabbitMQURL)
		if err != nil {
			return err
		}

		ch, err := conn.Channel()
		if err != nil {
			return err
		}

		MQ = ch
		log.Println("Connected to RabbitMQ!")
		return nil
	})

	if err != nil {
		log.Fatal("RabbitMQ connection failed:", err)
	}
}
