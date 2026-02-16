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

	"encoding/json"

	"github.com/joho/godotenv"
	"github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var PORT string
var HOST string
var AllowedOrigins string
var NgrokActive string
var NgrokAuthToken string
var DB *mongo.Database
var QueueName string
var MQ *amqp091.Channel
var BotType string
var TelegramBotToken string
var DiscordBotToken string
var RedisClient *redis.Client
var LLMProviderBaseURL string
var LLMProviderName string
var LLMProviderAPIKey string
var StreamResponse bool
var TTSProviderName string
var TTSProviderAPIKey string
var WatermarkModel bool
var OwnerIDs []string
var Heartbeat HeartbeatConfig

type Config struct {
	Heartbeat HeartbeatConfig `json:"heartbeat"`
	Bot       GlobalBotConfig `json:"bot,omitempty"`
	Channels  ChannelsConfig  `json:"channels"`
}

type GlobalBotConfig struct {
	OwnerIDs  []string `json:"owner_ids"`
	Type      string   `json:"type"`
	Watermark bool     `json:"watermark"`
}

type ChannelsConfig struct {
	Telegram *ChannelConfig `json:"telegram,omitempty"`
	Discord  *ChannelConfig `json:"discord,omitempty"`
}

type ChannelConfig struct {
	Active bool   `json:"active"`
	Token  string `json:"token"`
}

type HeartbeatConfig struct {
	Active  bool   `json:"active"`
	Every   string `json:"every"`
	To      string `json:"to"`
	Channel string `json:"channel"`
}

func loadJSONConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(homeDir, ".myaaw", "config.json")
	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Config file doesn't exist, that's fine
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config.json: %w", err)
	}

	return &config, nil
}

func envPath() string {
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Join(filepath.Dir(b), "../..")
	envPath := filepath.Join(basePath, ".env")
	return envPath
}

func LoadBaseConfig() {
	path := envPath()
	err := godotenv.Load(path)
	log.Println("Load .env file", path)
	if err != nil {
		log.Println("Error loading .env file, using environment variables")
	}

	PORT = ":" + os.Getenv("PORT")
	HOST = os.Getenv("HOST")
	AllowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	NgrokActive = os.Getenv("NGROK_ACTIVE")
	NgrokAuthToken = os.Getenv("NGROK_AUTHTOKEN")
	QueueName = os.Getenv("QUEUE_NAME")
	LLMProviderBaseURL = os.Getenv("LLM_PROVIDER_BASE_URL")
	LLMProviderName = os.Getenv("LLM_PROVIDER_NAME")
	LLMProviderAPIKey = os.Getenv("LLM_PROVIDER_API_KEY")

	TTSProviderName = os.Getenv("TTS_PROVIDER_NAME")
	if TTSProviderName == "" {
		TTSProviderName = "groq" // Default value if not set
		log.Println("TTS_PROVIDER_NAME not set, using default:", TTSProviderName)
	}
	TTSProviderAPIKey = os.Getenv("TTS_PROVIDER_API_KEY")
	// No default for API key for security reasons

	streamVal := os.Getenv("STREAM_RESPONSE")
	if streamVal != "" {
		StreamResponse, err = strconv.ParseBool(streamVal)
		if err != nil {
			log.Printf("Warning: Invalid value for STREAM_RESPONSE '%s', defaulting to false", streamVal)
			StreamResponse = false
		}
	} else {
		StreamResponse = false
	}

	if AllowedOrigins == "" {
		AllowedOrigins = "*"
	}

	jsonConfig, err := loadJSONConfig()
	if err != nil {
		log.Printf("Warning: Failed to load JSON config: %v", err)
	}
	if jsonConfig != nil {
		if len(jsonConfig.Bot.OwnerIDs) > 0 {
			OwnerIDs = jsonConfig.Bot.OwnerIDs
		}
		if jsonConfig.Bot.Type != "" {
			BotType = jsonConfig.Bot.Type
		}
		WatermarkModel = jsonConfig.Bot.Watermark

		if jsonConfig.Channels.Telegram != nil {
			if jsonConfig.Channels.Telegram.Active {
				TelegramBotToken = jsonConfig.Channels.Telegram.Token
			} else {
				TelegramBotToken = ""
			}
		}

		if jsonConfig.Channels.Discord != nil {
			if jsonConfig.Channels.Discord.Active {
				DiscordBotToken = jsonConfig.Channels.Discord.Token
			} else {
				DiscordBotToken = ""
			}
		}

		Heartbeat = jsonConfig.Heartbeat
	}
}

func ConnectDatabases() {
	maxRetries := 10
	retryDelay := 3 * time.Second

	mongoURI := os.Getenv("MONGODB_URI")
	dbName := os.Getenv("DB_NAME")
	redisURL := os.Getenv("REDIS_URL")

	ConnectMongoDB(mongoURI, dbName, maxRetries, retryDelay)
	ConnectRedis(redisURL, maxRetries, retryDelay)
}

func ConnectQueue() {
	maxRetries := 10
	retryDelay := 3 * time.Second

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	ConnectRabbitMQ(rabbitMQURL, maxRetries, retryDelay)
}

func LoadConfig() {
	LoadBaseConfig()
	ConnectDatabases()
	ConnectQueue()
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
