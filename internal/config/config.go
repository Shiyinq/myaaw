package config

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

var PORT string
var HOST string
var AllowedOrigins string
var NgrokActive string
var NgrokAuthToken string
var DB *sql.DB
var BotType string
var TelegramBotToken string
var DiscordBotToken string
var LLMProviderBaseURL string
var LLMProviderName string
var LLMProviderAPIKey string
var LLMDefaultModel string
var StreamResponse bool
var TranscriberProviderName string
var TranscriberAPIKey string
var WatermarkModel bool
var OwnerIDs []string
var Heartbeat HeartbeatConfig
var TelegramMode string // "webhook" or "polling"
var Verbose bool
var CurrentProviderID string

type Config struct {
	DefaultProvider string                    `json:"default_provider,omitempty"`
	Providers       map[string]ProviderConfig `json:"providers,omitempty"`
	Heartbeat       HeartbeatConfig           `json:"heartbeat"`
	Bot             GlobalBotConfig           `json:"bot,omitempty"`
	Channels        ChannelsConfig            `json:"channels"`
	Cron            CronConfig                `json:"cron"`
}

type ProviderConfig struct {
	Type         string `json:"type"`
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
}

type CronConfig struct {
	Active bool `json:"active"`
}

type GlobalBotConfig struct {
	OwnerIDs  []string `json:"owner_ids"`
	Type      string   `json:"type"`
	Watermark bool     `json:"watermark"`
	Stream    *bool    `json:"stream,omitempty"`
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

func GetConfigPath() string {
	// Try current directory
	if _, err := os.Stat("config.json"); err == nil {
		return "config.json"
	}
	
	homeDir, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(homeDir, ".myaaw", "config.json")
	}
	return "config.json"
}

func SaveConfig(config *Config) error {
	path := GetConfigPath()
	
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func LoadJSONConfigOnly() (*Config, error) {
	return loadJSONConfig()
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
	LLMProviderBaseURL = os.Getenv("LLM_PROVIDER_BASE_URL")
	LLMProviderName = os.Getenv("LLM_PROVIDER_NAME")
	LLMProviderAPIKey = os.Getenv("LLM_PROVIDER_API_KEY")
	LLMDefaultModel = ""

	TranscriberProviderName = os.Getenv("TRANSCRIBER_PROVIDER_NAME")
	if TranscriberProviderName == "" {
		if LLMProviderName == "gemini" {
			TranscriberProviderName = "gemini"
		} else {
			TranscriberProviderName = "groq"
		}
	}

	TranscriberAPIKey = os.Getenv("TRANSCRIBER_API_KEY")
	if TranscriberAPIKey == "" && TranscriberProviderName == "gemini" && LLMProviderName == "gemini" {
		TranscriberAPIKey = LLMProviderAPIKey
	}

	streamVal := strings.ToLower(os.Getenv("STREAM_RESPONSE"))
	if streamVal == "false" || streamVal == "0" {
		StreamResponse = false
	} else {
		StreamResponse = true // Default to true
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
		if jsonConfig.Bot.Stream != nil {
			StreamResponse = *jsonConfig.Bot.Stream
		}

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
		
		// Priority: config.json Providers > .env
		if jsonConfig.DefaultProvider != "" && jsonConfig.Providers != nil {
			CurrentProviderID = jsonConfig.DefaultProvider
			if provider, exists := jsonConfig.Providers[jsonConfig.DefaultProvider]; exists {
				LLMProviderName = provider.Type
				LLMProviderAPIKey = provider.APIKey
				LLMProviderBaseURL = provider.BaseURL
				LLMDefaultModel = provider.DefaultModel
			}
		}
	}
}

func ConnectDatabases() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			dbPath = filepath.Join(homeDir, ".myaaw", "myaaw.db")
		} else {
			dbPath = "myaaw.db"
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create db directory: %v", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal("Failed to open SQLite database:", err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping SQLite database:", err)
	}

	DB = db
	if Verbose {
		log.Println("Connected to SQLite database at:", dbPath)
	}

	// Initialize Schema
	initSchema(db)
}

func initSchema(db *sql.DB) {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			user_id INTEGER UNIQUE,
			name TEXT,
			provider TEXT,
			model TEXT,
			role TEXT,
			created_at DATETIME,
			updated_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			user_id INTEGER,
			title TEXT,
			messages TEXT,
			active BOOLEAN,
			created_at DATETIME,
			updated_at DATETIME
		);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			log.Fatalf("Failed to execute schema query: %v", err)
		}
	}
}

func LoadConfig() {
	LoadBaseConfig()
	ConnectDatabases()
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
