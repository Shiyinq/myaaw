package config

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"myaaw/internal/agentdir"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
)

var PORT string
var HOST string
var MYAAWBaseURL string
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
var A2AToken string
var A2ATokens []string
var A2ACallers []string
var A2ASigningKeyPath string
var A2ATrustedKeys string
var A2AProviderURL string
var A2AProviderOrganization string
var A2ADocumentationURL string
var A2AIconURL string
var A2AExtendedCardPath string
var A2AAllowPrivateWebhooks bool
var A2ARateLimit float64
var A2ARateBurst int
var A2AGRPCPort string
var A2AGRPCAddr string
var A2AGRPCTLSCert string
var A2AGRPCTLSKey string

// A2AMaxRequestBytes is the server-wide HTTP request body limit. It must be
// large enough to carry the maximum inline file payload (base64 inflates raw
// bytes by roughly 4/3).
var A2AMaxRequestBytes int = 16 << 20

// A2AContactTimeoutSeconds bounds a contact tool call. It must exceed the
// remote agent's run timeout, otherwise the caller gives up while the task is
// still running and a retry creates a duplicate task.
var A2AContactTimeoutSeconds int = 960
var TranscriberProviderName string
var TranscriberAPIKey string
var SynthesizerProviderName string
var SynthesizerAPIKey string
var WatermarkModel bool
var OwnerIDs []string
var Heartbeat HeartbeatConfig
var TelegramMode string // "webhook" or "polling"
var Verbose bool
var CurrentProviderID string
var AgentMaxIterations int = 100
var AgentWarningIterations int = 90
var AgentContextMaxTokens int = 64000
var AgentMaxToolOutputBytes int = 32 * 1024
var AgentRepeatToolCallLimit int = 3
var AgentToolTimeoutSeconds int = 300
var AgentBlockedTools []string
var LLMRequestRetries int = 1
var LLMStreamTimeoutSeconds int = 180
var AgentRunTimeoutSeconds int = 900

// QueueWorkers bounds how many inbound channel messages are processed at the
// same time. Messages from the same user stay serialized.
var QueueWorkers int = 4

// Observability defaults. Recording is on by default; OBS_ENABLED=false (or
// observability.enabled in config.json) turns it off entirely.
var (
	ObsEnabled         = true
	ObsRetentionDays   = 30
	ObsMaxContentChars = 8192
	ObsStoreContent    = true
	ObsModelPrices     map[string]ModelPriceConfig
)

// MaxQueueWorkers caps the configured concurrency so a typo cannot spawn an
// unbounded number of concurrent LLM runs.
const MaxQueueWorkers = 16

type Config struct {
	DefaultProvider string                    `json:"default_provider,omitempty"`
	Providers       map[string]ProviderConfig `json:"providers,omitempty"`
	Heartbeat       HeartbeatConfig           `json:"heartbeat"`
	Bot             GlobalBotConfig           `json:"bot,omitempty"`
	Channels        ChannelsConfig            `json:"channels"`
	Cron            CronConfig                `json:"cron"`
	Transcriber     TranscriberConfig         `json:"transcriber,omitempty"`
	Synthesizer     SynthesizerConfig         `json:"synthesizer,omitempty"`
	Observability   *ObservabilityConfig      `json:"observability,omitempty"`
}

// ObservabilityConfig controls local trace recording and model pricing.
type ObservabilityConfig struct {
	Enabled         *bool                       `json:"enabled,omitempty"`
	RetentionDays   int                         `json:"retention_days,omitempty"`
	MaxContentChars int                         `json:"max_content_chars,omitempty"`
	StoreContent    *bool                       `json:"store_content,omitempty"`
	ModelPrices     map[string]ModelPriceConfig `json:"model_prices,omitempty"`
}

// ModelPriceConfig is a model price in USD per one million tokens.
//
// Input is the cache-miss price and CacheHit the discounted price for prompt
// tokens served from the provider cache. OffPeak replaces the triplet outside
// every PeakWindows entry; only models that declare both get peak pricing.
type ModelPriceConfig struct {
	Input       float64           `json:"input"`
	CacheHit    float64           `json:"cache_hit,omitempty"`
	Output      float64           `json:"output"`
	OffPeak     *ModelPriceConfig `json:"off_peak,omitempty"`
	PeakWindows []string          `json:"peak_windows,omitempty"`
}

// EffectiveObservability resolves observability settings from .env and
// config.json, applying defaults for anything unset.
func EffectiveObservability() ObservabilityConfig {
	enabled := ObsEnabled
	storeContent := ObsStoreContent
	return ObservabilityConfig{
		Enabled:         &enabled,
		RetentionDays:   ObsRetentionDays,
		MaxContentChars: ObsMaxContentChars,
		StoreContent:    &storeContent,
		ModelPrices:     ObsModelPrices,
	}
}

type ProviderConfig struct {
	Type         string `json:"type"`
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
}

type TranscriberConfig struct {
	Provider string `json:"provider,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
}

type SynthesizerConfig struct {
	Provider string `json:"provider,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
}

type CronConfig struct {
	Active bool `json:"active"`
}

type GlobalBotConfig struct {
	OwnerIDs             []string `json:"owner_ids"`
	Type                 string   `json:"type"`
	Watermark            bool     `json:"watermark"`
	Stream               *bool    `json:"stream,omitempty"`
	MaxIterations        *int     `json:"max_iterations,omitempty"`
	WarningIterations    *int     `json:"warning_iterations,omitempty"`
	ContextMaxTokens     *int     `json:"context_max_tokens,omitempty"`
	MaxToolOutputBytes   *int     `json:"max_tool_output_bytes,omitempty"`
	RepeatToolCallLimit  *int     `json:"repeat_tool_call_limit,omitempty"`
	ToolTimeoutSeconds   *int     `json:"tool_timeout_seconds,omitempty"`
	RunTimeoutSeconds    *int     `json:"run_timeout_seconds,omitempty"`
	StreamTimeoutSeconds *int     `json:"stream_timeout_seconds,omitempty"`
	RequestRetries       *int     `json:"request_retries,omitempty"`
	QueueWorkers         *int     `json:"queue_workers,omitempty"`
	BlockedTools         []string `json:"blocked_tools,omitempty"`
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
	// 1. Check the active agent configuration.
	if configPath, err := agentdir.ActiveConfigPath(); err == nil {
		if _, err := os.Stat(configPath); err == nil {
			if Verbose {
				log.Println("Reading config from agent directory:", configPath)
			}
			return parseJSONFile(configPath)
		}
	}

	// 2. Check current directory for development overrides.
	if _, err := os.Stat("config.json"); err == nil {
		if Verbose {
			log.Println("Reading config from current directory: config.json")
		}
		return parseJSONFile("config.json")
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
	if path, err := agentdir.ActiveConfigPath(); err == nil {
		return path
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
	// 1. Check the active agent environment.
	if path, err := agentdir.ActiveEnvPath(); err == nil {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// 2. Check current directory for development overrides.
	if abs, err := filepath.Abs(".env"); err == nil {
		if _, err := os.Stat(abs); err == nil {
			return abs
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
			log.Println("ℹ️  No .env file found in the agent directory, current directory, or dev fallback. Using system environment variables.")
		}
	}

	portValue := strings.TrimSpace(os.Getenv("PORT"))
	if portValue == "" {
		portValue = strconv.Itoa(agentdir.SuggestedPort(agentdir.Active()))
	}
	PORT = ":" + portValue
	HOST = os.Getenv("HOST")
	MYAAWBaseURL = os.Getenv("MYAAW_BASE_URL")
	A2AToken = strings.TrimSpace(os.Getenv("A2A_TOKEN"))
	A2ATokens = splitCSV(os.Getenv("A2A_TOKENS"))
	A2ACallers = splitCSV(os.Getenv("A2A_CALLERS"))
	A2ASigningKeyPath = strings.TrimSpace(os.Getenv("A2A_SIGNING_KEY"))
	A2ATrustedKeys = strings.TrimSpace(os.Getenv("A2A_TRUSTED_KEYS"))
	A2AProviderURL = strings.TrimSpace(os.Getenv("A2A_PROVIDER_URL"))
	A2AProviderOrganization = strings.TrimSpace(os.Getenv("A2A_PROVIDER_ORGANIZATION"))
	A2ADocumentationURL = strings.TrimSpace(os.Getenv("A2A_DOCUMENTATION_URL"))
	A2AIconURL = strings.TrimSpace(os.Getenv("A2A_ICON_URL"))
	A2AExtendedCardPath = strings.TrimSpace(os.Getenv("A2A_EXTENDED_CARD"))
	A2AAllowPrivateWebhooks = envBool("A2A_ALLOW_PRIVATE_WEBHOOKS", false)
	A2ARateLimit = envFloat("A2A_RATE_LIMIT", 0)
	A2ARateBurst = envInt("A2A_RATE_BURST", 0)
	A2AGRPCPort = strings.TrimSpace(os.Getenv("A2A_GRPC_PORT"))
	A2AGRPCAddr = strings.TrimSpace(os.Getenv("A2A_GRPC_ADDR"))
	A2AGRPCTLSCert = strings.TrimSpace(os.Getenv("A2A_GRPC_TLS_CERT"))
	A2AGRPCTLSKey = strings.TrimSpace(os.Getenv("A2A_GRPC_TLS_KEY"))
	A2AMaxRequestBytes = envInt("A2A_MAX_REQUEST_BYTES", A2AMaxRequestBytes)
	A2AContactTimeoutSeconds = envInt("A2A_CONTACT_TIMEOUT_SECONDS", A2AContactTimeoutSeconds)
	AllowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	LLMProviderBaseURL = os.Getenv("LLM_PROVIDER_BASE_URL")
	LLMProviderName = os.Getenv("LLM_PROVIDER_NAME")
	LLMProviderAPIKey = os.Getenv("LLM_PROVIDER_API_KEY")
	LLMDefaultModel = ""

	TranscriberProviderName = os.Getenv("TRANSCRIBER_PROVIDER_NAME")
	TranscriberAPIKey = os.Getenv("TRANSCRIBER_API_KEY")

	SynthesizerProviderName = os.Getenv("SYNTHESIZER_PROVIDER_NAME")
	SynthesizerAPIKey = os.Getenv("SYNTHESIZER_API_KEY")

	AgentContextMaxTokens = envInt("AGENT_CONTEXT_MAX_TOKENS", AgentContextMaxTokens)
	AgentMaxToolOutputBytes = envInt("AGENT_MAX_TOOL_OUTPUT_BYTES", AgentMaxToolOutputBytes)
	AgentMaxIterations = envInt("AGENT_MAX_ITERATIONS", AgentMaxIterations)
	AgentWarningIterations = envInt("AGENT_WARNING_ITERATIONS", AgentWarningIterations)
	AgentRepeatToolCallLimit = envInt("AGENT_REPEAT_TOOL_CALL_LIMIT", AgentRepeatToolCallLimit)
	AgentToolTimeoutSeconds = envInt("AGENT_TOOL_TIMEOUT_SECONDS", AgentToolTimeoutSeconds)
	AgentBlockedTools = splitCSV(os.Getenv("AGENT_BLOCKED_TOOLS"))
	LLMRequestRetries = envInt("LLM_REQUEST_RETRIES", LLMRequestRetries)
	QueueWorkers = envInt("QUEUE_WORKERS", QueueWorkers)

	ObsEnabled = envBool("OBS_ENABLED", ObsEnabled)
	ObsRetentionDays = envInt("OBS_RETENTION_DAYS", ObsRetentionDays)
	ObsMaxContentChars = envInt("OBS_MAX_CONTENT_CHARS", ObsMaxContentChars)
	ObsStoreContent = envBool("OBS_STORE_CONTENT", ObsStoreContent)
	LLMStreamTimeoutSeconds = envInt("LLM_STREAM_TIMEOUT_SECONDS", LLMStreamTimeoutSeconds)
	AgentRunTimeoutSeconds = envInt("AGENT_RUN_TIMEOUT_SECONDS", AgentRunTimeoutSeconds)

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
		if jsonConfig.Bot.MaxIterations != nil {
			AgentMaxIterations = *jsonConfig.Bot.MaxIterations
		}
		if jsonConfig.Bot.WarningIterations != nil {
			AgentWarningIterations = *jsonConfig.Bot.WarningIterations
		}
		if jsonConfig.Bot.ContextMaxTokens != nil {
			AgentContextMaxTokens = *jsonConfig.Bot.ContextMaxTokens
		}
		if jsonConfig.Bot.MaxToolOutputBytes != nil {
			AgentMaxToolOutputBytes = *jsonConfig.Bot.MaxToolOutputBytes
		}
		if jsonConfig.Bot.RepeatToolCallLimit != nil {
			AgentRepeatToolCallLimit = *jsonConfig.Bot.RepeatToolCallLimit
		}
		if jsonConfig.Bot.ToolTimeoutSeconds != nil {
			AgentToolTimeoutSeconds = *jsonConfig.Bot.ToolTimeoutSeconds
		}
		if jsonConfig.Bot.RunTimeoutSeconds != nil {
			AgentRunTimeoutSeconds = *jsonConfig.Bot.RunTimeoutSeconds
		}
		if jsonConfig.Bot.StreamTimeoutSeconds != nil {
			LLMStreamTimeoutSeconds = *jsonConfig.Bot.StreamTimeoutSeconds
		}
		if jsonConfig.Bot.RequestRetries != nil {
			LLMRequestRetries = *jsonConfig.Bot.RequestRetries
		}
		if jsonConfig.Bot.QueueWorkers != nil {
			QueueWorkers = *jsonConfig.Bot.QueueWorkers
		}
		if len(jsonConfig.Bot.BlockedTools) > 0 {
			AgentBlockedTools = jsonConfig.Bot.BlockedTools
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

		// Priority: config.json Transcriber > .env
		if jsonConfig.Transcriber.Provider != "" {
			TranscriberProviderName = jsonConfig.Transcriber.Provider
			if jsonConfig.Transcriber.APIKey != "" {
				TranscriberAPIKey = jsonConfig.Transcriber.APIKey
			}
		}

		// Priority: config.json Synthesizer > .env
		if jsonConfig.Synthesizer.Provider != "" {
			SynthesizerProviderName = jsonConfig.Synthesizer.Provider
			if jsonConfig.Synthesizer.APIKey != "" {
				SynthesizerAPIKey = jsonConfig.Synthesizer.APIKey
			}
		}

		// Priority: config.json observability > .env OBS_*
		if obs := jsonConfig.Observability; obs != nil {
			if obs.Enabled != nil {
				ObsEnabled = *obs.Enabled
			}
			if obs.RetentionDays > 0 {
				ObsRetentionDays = obs.RetentionDays
			}
			if obs.MaxContentChars > 0 {
				ObsMaxContentChars = obs.MaxContentChars
			}
			if obs.StoreContent != nil {
				ObsStoreContent = *obs.StoreContent
			}
			ObsModelPrices = obs.ModelPrices
		}
	}

	validateAgentConfig()
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func envBool(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	log.Printf("Warning: invalid %s=%q, using %t", name, value, fallback)
	return fallback
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("Warning: invalid %s=%q, using %d", name, value, fallback)
		return fallback
	}
	return parsed
}

func envFloat(name string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("Warning: invalid %s=%q, using %g", name, value, fallback)
		return fallback
	}
	return parsed
}

func validateAgentConfig() {
	if AgentMaxIterations < 1 {
		AgentMaxIterations = 100
	}
	if AgentWarningIterations < 1 || AgentWarningIterations >= AgentMaxIterations {
		AgentWarningIterations = AgentMaxIterations - 10
		if AgentWarningIterations < 1 {
			AgentWarningIterations = 1
		}
	}
	if AgentContextMaxTokens < 0 {
		AgentContextMaxTokens = 0
	}
	if AgentMaxToolOutputBytes <= 0 {
		AgentMaxToolOutputBytes = 32 * 1024
	}
	if AgentRepeatToolCallLimit < 0 {
		AgentRepeatToolCallLimit = 0
	}
	if AgentToolTimeoutSeconds < 0 {
		AgentToolTimeoutSeconds = 0
	}
	if LLMRequestRetries < 0 {
		LLMRequestRetries = 0
	}
	if QueueWorkers < 1 {
		QueueWorkers = 1
	}
	if QueueWorkers > MaxQueueWorkers {
		QueueWorkers = MaxQueueWorkers
	}
	if ObsRetentionDays < 0 {
		ObsRetentionDays = 0
	}
	if ObsMaxContentChars <= 0 {
		ObsMaxContentChars = 8192
	}
	if LLMStreamTimeoutSeconds <= 0 {
		LLMStreamTimeoutSeconds = 180
	}
	if AgentRunTimeoutSeconds <= 0 {
		AgentRunTimeoutSeconds = 900
	}
	if A2AMaxRequestBytes <= 0 {
		A2AMaxRequestBytes = 16 << 20
	}
	if A2AContactTimeoutSeconds <= 0 {
		A2AContactTimeoutSeconds = 960
	}
}

func ConnectDatabases() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		if path, err := agentdir.ActiveDatabasePath(); err == nil {
			dbPath = path
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
	configPath, err := agentdir.ActiveConfigPath()
	if err != nil {
		log.Printf("Failed to resolve agent config path: %v", err)
		return
	}

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
