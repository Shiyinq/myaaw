package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"encoding/json"

	"github.com/fsnotify/fsnotify"
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
var TranscriberProviderName string
var TranscriberAPIKey string
var WatermarkModel bool
var OwnerIDs []string
var Heartbeat HeartbeatConfig
var TelegramMode string // "webhook" or "polling"
var Verbose bool

type Config struct {
	Heartbeat HeartbeatConfig `json:"heartbeat"`
	Bot       GlobalBotConfig `json:"bot,omitempty"`
	Channels  ChannelsConfig  `json:"channels"`
	Cron      CronConfig      `json:"cron"`
}

type CronConfig struct {
	Active bool `json:"active"`
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
	Active         bool   `json:"active"`
	Token          string `json:"token"`
	Mode           string `json:"mode,omitempty"`             // "polling" or "webhook"
	NgrokActive    bool   `json:"ngrok_active,omitempty"`     // Only for webhook
	NgrokAuthToken string `json:"ngrok_auth_token,omitempty"` // Only for webhook
}

type HeartbeatConfig struct {
	Active  bool   `json:"active"`
	Every   string `json:"every"`
	To      string `json:"to"`
	Channel string `json:"channel"`
}

var CronActive bool

func loadJSONConfig() (*Config, error) {
	// 1. Check Current Directory
	if _, err := os.Stat("config.json"); err == nil {
		if Verbose {
			log.Println("Reading config from current directory: config.json")
		}
		return parseJSONFile("config.json")
	}

	// 2. Check Home Directory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(homeDir, ".myaaw", "config.json")
		if _, err := os.Stat(configPath); err == nil {
			if Verbose {
				log.Println("Reading config from home directory:", configPath)
			}
			return parseJSONFile(configPath)
		}
	}

	return nil, nil // No config file found
}

func parseJSONFile(path string) (*Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config Config
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return &config, nil
}

func envPath() string {
	// 1. Check Current Directory
	if abs, err := filepath.Abs(".env"); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}

	// 2. Check Home Directory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		path := filepath.Join(homeDir, ".myaaw", ".env")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// 3. Fallback to hardcoded dev path (only if it exists)
	_, b, _, _ := runtime.Caller(0)
	devPath := filepath.Join(filepath.Dir(b), "../../.env")
	if _, err := os.Stat(devPath); err == nil {
		return devPath
	}

	return ""
}

func LoadBaseConfig() {
	path := envPath()
	if path != "" {
		err := godotenv.Load(path)
		if err != nil {
			log.Printf("⚠️  Warning: Error loading .env file from %s: %v\n", path, err)
		} else {
			if Verbose {
				log.Printf("✅ Loaded environment from: %s\n", path)
			}
		}
	} else {
		if Verbose {
			log.Println("ℹ️  No .env file found in current directory, ~/.myaaw/, or dev fallback. Using system environment variables.")
		}
	}

	PORT = ":" + os.Getenv("PORT")
	HOST = os.Getenv("HOST")
	AllowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	QueueName = os.Getenv("QUEUE_NAME")
	LLMProviderBaseURL = os.Getenv("LLM_PROVIDER_BASE_URL")
	LLMProviderName = os.Getenv("LLM_PROVIDER_NAME")
	LLMProviderAPIKey = os.Getenv("LLM_PROVIDER_API_KEY")

	TranscriberProviderName = os.Getenv("TRANSCRIBER_PROVIDER_NAME")
	if TranscriberProviderName == "" {
		if LLMProviderName == "gemini" {
			TranscriberProviderName = "gemini"
		} else {
			TranscriberProviderName = "groq"
		}
		log.Println("TRANSCRIBER_PROVIDER_NAME not set, using default:", TranscriberProviderName)
	}

	TranscriberAPIKey = os.Getenv("TRANSCRIBER_API_KEY")
	if TranscriberAPIKey == "" && TranscriberProviderName == "gemini" && LLMProviderName == "gemini" {
		TranscriberAPIKey = LLMProviderAPIKey
		log.Println("TRANSCRIBER_API_KEY not set, reusing LLM_PROVIDER_API_KEY for Gemini")
	}
	// No default for API key for security reasons

	streamVal := os.Getenv("STREAM_RESPONSE")
	if streamVal != "" {
		boolVal, err := strconv.ParseBool(streamVal)
		if err != nil {
			log.Printf("Warning: Invalid value for STREAM_RESPONSE '%s', defaulting to false", streamVal)
			StreamResponse = false
		} else {
			StreamResponse = boolVal
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
			TelegramBotToken = jsonConfig.Channels.Telegram.Token
			TelegramMode = jsonConfig.Channels.Telegram.Mode
			if TelegramMode == "" {
				TelegramMode = "polling"
			} else {
				TelegramMode = strings.ToLower(TelegramMode)
			}

			if jsonConfig.Channels.Telegram.NgrokActive {
				NgrokActive = "true"
			} else {
				NgrokActive = "false"
			}
			NgrokAuthToken = jsonConfig.Channels.Telegram.NgrokAuthToken

			if !jsonConfig.Channels.Telegram.Active {
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
		CronActive = jsonConfig.Cron.Active
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

		// Only log on the first failure or every 5th failure to reduce noise, unless verbose is on
		if Verbose || i == 0 || (i+1)%5 == 0 {
			log.Printf("Attempt %d/%d failed: %v. Retrying in %v...", i+1, attempts, err, delay)
		}
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
		if Verbose {
			log.Println("Connected to MongoDB!")
		}
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

		if Verbose {
			log.Println("Connected to Redis!")
		}
		return nil
	})

	if err != nil {
		log.Fatal("Redis connection failed:", err)
	}
}

func ConnectRabbitMQ(rabbitMQURL string, maxRetries int, retryDelay time.Duration) {
	if rabbitMQURL == "" {
		log.Fatal("❌ Error: RABBITMQ_URL is not set. Please check your .env file.")
	}
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
		if Verbose {
			log.Println("Connected to RabbitMQ!")
		}
		return nil
	})

	if err != nil {
		log.Fatal("RabbitMQ connection failed:", err)
	}
}

// WatchConfig watches for changes in config.json and reloads the config
func WatchConfig(onChange func()) {
	// Identify config path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Failed to get home dir for watcher: %v", err)
		return
	}
	configPath := filepath.Join(homeDir, ".myaaw", "config.json")

	// Create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Failed to create config watcher: %v", err)
		return
	}

	// Watch the directory
	configDir := filepath.Dir(configPath)
	if err := watcher.Add(configDir); err != nil {
		log.Printf("Failed to watch config directory: %v", err)
		watcher.Close()
		return
	}

	log.Println("Global Config Watcher started...")

	// Debounce timer
	var debounceTimer *time.Timer
	debounceDuration := 500 * time.Millisecond

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) == filepath.Base(configPath) {
					if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Chmod) {
						// Stop existing timer if any
						if debounceTimer != nil {
							debounceTimer.Stop()
						}
						// Reset timer
						debounceTimer = time.AfterFunc(debounceDuration, func() {
							log.Println("Config file changed, reloading...")
							LoadBaseConfig() // Reloads global vars
							if onChange != nil {
								onChange()
							}
						})
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Config watcher error: %v", err)
			}
		}
	}()
}
