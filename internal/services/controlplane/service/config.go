package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	myaaw "myaaw"
	"myaaw/internal/agentdir"
	"myaaw/internal/config"
)

var (
	ErrConfigNotFound   = errors.New("agent config not found")
	ErrProviderNotFound = errors.New("provider not found")
	ErrChannelNotFound  = errors.New("channel not found")
	ErrInvalidConfig    = errors.New("invalid config")
)

type ProviderInput struct {
	Name         string
	Type         string
	BaseURL      string
	APIKey       string
	DefaultModel string
}

type ProviderPatch struct {
	Type         *string
	BaseURL      *string
	APIKey       *string
	DefaultModel *string
}

type ChannelInput struct {
	Active         *bool
	Token          string
	Mode           string
	NgrokActive    *bool
	NgrokAuthToken string
}

type SecretState struct {
	Configured bool   `json:"configured"`
	Masked     string `json:"masked,omitempty"`
}

type ProviderView struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	BaseURL      string      `json:"base_url,omitempty"`
	DefaultModel string      `json:"default_model,omitempty"`
	APIKey       SecretState `json:"api_key"`
}

type ProvidersView struct {
	Default   string         `json:"default_provider"`
	Providers []ProviderView `json:"providers"`
}

type ChannelView struct {
	Name           string      `json:"name"`
	Active         bool        `json:"active"`
	Mode           string      `json:"mode,omitempty"`
	Token          SecretState `json:"token"`
	NgrokActive    bool        `json:"ngrok_active,omitempty"`
	NgrokAuthToken SecretState `json:"ngrok_auth_token,omitempty"`
}

type ChannelsView struct {
	Channels []ChannelView `json:"channels"`
}

type ServiceSecretView struct {
	Provider string      `json:"provider,omitempty"`
	APIKey   SecretState `json:"api_key"`
}

type ConfigView struct {
	DefaultProvider string                 `json:"default_provider"`
	Providers       []ProviderView         `json:"providers"`
	Channels        []ChannelView          `json:"channels"`
	Bot             config.GlobalBotConfig `json:"bot"`
	Heartbeat       config.HeartbeatConfig `json:"heartbeat"`
	Cron            config.CronConfig      `json:"cron"`
	Transcriber     ServiceSecretView      `json:"transcriber"`
	Synthesizer     ServiceSecretView      `json:"synthesizer"`
}

// ConfigService reads per-agent configuration with secrets masked.
type ConfigService struct{}

func NewConfigService() *ConfigService {
	return &ConfigService{}
}

func (s *ConfigService) Providers(agentName string) (ProvidersView, error) {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ProvidersView{}, err
	}
	return ProvidersView{
		Default:   cfg.DefaultProvider,
		Providers: providerViews(cfg),
	}, nil
}

func (s *ConfigService) Channels(agentName string) (ChannelsView, error) {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ChannelsView{}, err
	}
	return ChannelsView{Channels: channelViews(cfg)}, nil
}

func (s *ConfigService) Config(agentName string) (ConfigView, error) {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ConfigView{}, err
	}
	return configView(cfg), nil
}

// configView builds the read shape of a config with every secret masked.
func configView(cfg *config.Config) ConfigView {
	return ConfigView{
		DefaultProvider: cfg.DefaultProvider,
		Providers:       providerViews(cfg),
		Channels:        channelViews(cfg),
		Bot:             cfg.Bot,
		Heartbeat:       cfg.Heartbeat,
		Cron:            cfg.Cron,
		Transcriber: ServiceSecretView{
			Provider: cfg.Transcriber.Provider,
			APIKey:   secretState(cfg.Transcriber.APIKey),
		},
		Synthesizer: ServiceSecretView{
			Provider: cfg.Synthesizer.Provider,
			APIKey:   secretState(cfg.Synthesizer.APIKey),
		},
	}
}

func loadAgentConfig(agentName string) (*config.Config, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}
	path, err := agentdir.ConfigPath(agentName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrConfigNotFound
		}
		return nil, err
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func providerViews(cfg *config.Config) []ProviderView {
	names := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)

	views := make([]ProviderView, 0, len(names))
	for _, name := range names {
		provider := cfg.Providers[name]
		views = append(views, ProviderView{
			Name:         name,
			Type:         provider.Type,
			BaseURL:      provider.BaseURL,
			DefaultModel: provider.DefaultModel,
			APIKey:       secretState(provider.APIKey),
		})
	}
	return views
}

func channelViews(cfg *config.Config) []ChannelView {
	views := make([]ChannelView, 0, 2)
	if telegram := cfg.Channels.Telegram; telegram != nil {
		views = append(views, ChannelView{
			Name:           "telegram",
			Active:         telegram.Active,
			Mode:           telegram.Mode,
			Token:          secretState(telegram.Token),
			NgrokActive:    telegram.NgrokActive,
			NgrokAuthToken: secretState(telegram.NgrokAuthToken),
		})
	}
	if discord := cfg.Channels.Discord; discord != nil {
		views = append(views, ChannelView{
			Name:   "discord",
			Active: discord.Active,
			Token:  secretState(discord.Token),
		})
	}
	return views
}

func secretState(value string) SecretState {
	if value == "" {
		return SecretState{Configured: false}
	}
	return SecretState{Configured: true, Masked: "********"}
}

func (s *ConfigService) CreateProvider(agentName string, in ProviderInput) (ProviderView, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || strings.TrimSpace(in.Type) == "" {
		return ProviderView{}, ErrInvalidConfig
	}

	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ProviderView{}, err
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}
	if _, exists := cfg.Providers[name]; exists {
		return ProviderView{}, ErrInvalidConfig
	}

	providerConfig := config.ProviderConfig{
		Type:         strings.TrimSpace(in.Type),
		BaseURL:      strings.TrimSpace(in.BaseURL),
		APIKey:       strings.TrimSpace(in.APIKey),
		DefaultModel: strings.TrimSpace(in.DefaultModel),
	}
	cfg.Providers[name] = providerConfig
	if cfg.DefaultProvider == "" {
		cfg.DefaultProvider = name
	}
	if err := saveAgentConfig(agentName, cfg); err != nil {
		return ProviderView{}, err
	}

	return ProviderView{
		Name:         name,
		Type:         providerConfig.Type,
		BaseURL:      providerConfig.BaseURL,
		DefaultModel: providerConfig.DefaultModel,
		APIKey:       secretState(providerConfig.APIKey),
	}, nil
}

func (s *ConfigService) UpdateProvider(agentName, id string, patch ProviderPatch) (ProviderView, error) {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ProviderView{}, err
	}
	providerConfig, exists := cfg.Providers[id]
	if !exists {
		return ProviderView{}, ErrProviderNotFound
	}

	if patch.Type != nil {
		providerConfig.Type = strings.TrimSpace(*patch.Type)
	}
	if patch.BaseURL != nil {
		providerConfig.BaseURL = strings.TrimSpace(*patch.BaseURL)
	}
	if patch.APIKey != nil && *patch.APIKey != "" {
		providerConfig.APIKey = strings.TrimSpace(*patch.APIKey)
	}
	if patch.DefaultModel != nil {
		providerConfig.DefaultModel = strings.TrimSpace(*patch.DefaultModel)
	}

	cfg.Providers[id] = providerConfig
	if err := saveAgentConfig(agentName, cfg); err != nil {
		return ProviderView{}, err
	}

	return ProviderView{
		Name:         id,
		Type:         providerConfig.Type,
		BaseURL:      providerConfig.BaseURL,
		DefaultModel: providerConfig.DefaultModel,
		APIKey:       secretState(providerConfig.APIKey),
	}, nil
}

func (s *ConfigService) DeleteProvider(agentName, id string) error {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return err
	}
	if _, exists := cfg.Providers[id]; !exists {
		return ErrProviderNotFound
	}
	delete(cfg.Providers, id)

	if cfg.DefaultProvider == id {
		names := make([]string, 0, len(cfg.Providers))
		for name := range cfg.Providers {
			names = append(names, name)
		}
		sort.Strings(names)
		cfg.DefaultProvider = ""
		if len(names) > 0 {
			cfg.DefaultProvider = names[0]
		}
	}

	return saveAgentConfig(agentName, cfg)
}

func (s *ConfigService) SetDefaultProvider(agentName, id string) error {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return err
	}
	if _, exists := cfg.Providers[id]; !exists {
		return ErrProviderNotFound
	}
	cfg.DefaultProvider = id
	return saveAgentConfig(agentName, cfg)
}

func (s *ConfigService) UpsertChannel(agentName, channel string, in ChannelInput) (ChannelView, error) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ChannelView{}, err
	}

	switch channel {
	case "telegram":
		existing := cfg.Channels.Telegram
		if existing == nil {
			existing = &config.ChannelConfig{Active: true}
		}
		if in.Active != nil {
			existing.Active = *in.Active
		}
		if in.Token != "" {
			existing.Token = strings.TrimSpace(in.Token)
		}
		if in.Mode != "" {
			existing.Mode = strings.TrimSpace(in.Mode)
		}
		if in.NgrokActive != nil {
			existing.NgrokActive = *in.NgrokActive
		}
		if in.NgrokAuthToken != "" {
			existing.NgrokAuthToken = strings.TrimSpace(in.NgrokAuthToken)
		}
		cfg.Channels.Telegram = existing
	case "discord":
		existing := cfg.Channels.Discord
		if existing == nil {
			existing = &config.ChannelConfig{Active: true}
		}
		if in.Active != nil {
			existing.Active = *in.Active
		}
		if in.Token != "" {
			existing.Token = strings.TrimSpace(in.Token)
		}
		cfg.Channels.Discord = existing
	default:
		return ChannelView{}, ErrInvalidConfig
	}

	if err := saveAgentConfig(agentName, cfg); err != nil {
		return ChannelView{}, err
	}
	for _, view := range channelViews(cfg) {
		if view.Name == channel {
			return view, nil
		}
	}
	return ChannelView{}, ErrChannelNotFound
}

func (s *ConfigService) DisableChannel(agentName, channel string) error {
	channel = strings.ToLower(strings.TrimSpace(channel))
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return err
	}

	switch channel {
	case "telegram":
		if cfg.Channels.Telegram == nil {
			return ErrChannelNotFound
		}
		cfg.Channels.Telegram.Active = false
	case "discord":
		if cfg.Channels.Discord == nil {
			return ErrChannelNotFound
		}
		cfg.Channels.Discord.Active = false
	default:
		return ErrChannelNotFound
	}

	return saveAgentConfig(agentName, cfg)
}

func saveAgentConfig(agentName string, cfg *config.Config) error {
	path, err := agentdir.ConfigPath(agentName)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

// writeFileAtomic writes data through a temp file in the same directory, so a
// crash mid-write cannot leave a truncated config.json behind.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpPath)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(path)
		if retryErr := os.Rename(tmpPath, path); retryErr != nil {
			os.Remove(tmpPath)
			return retryErr
		}
	}
	return nil
}

// ConfigPatch is a partial update for config.json. Every field is optional:
// absent fields are left untouched, present fields replace the current value.
// Keys are never removed; a field can only be set to a new value (including an
// empty one).
type ConfigPatch struct {
	Bot         *BotPatch       `json:"bot,omitempty"`
	Heartbeat   *HeartbeatPatch `json:"heartbeat,omitempty"`
	Cron        *CronPatch      `json:"cron,omitempty"`
	Transcriber *SecretPatch    `json:"transcriber,omitempty"`
	Synthesizer *SecretPatch    `json:"synthesizer,omitempty"`
}

type BotPatch struct {
	Type                 *string   `json:"type,omitempty"`
	OwnerIDs             *[]string `json:"owner_ids,omitempty"`
	Watermark            *bool     `json:"watermark,omitempty"`
	Stream               *bool     `json:"stream,omitempty"`
	MaxIterations        *int      `json:"max_iterations,omitempty"`
	WarningIterations    *int      `json:"warning_iterations,omitempty"`
	ContextMaxTokens     *int      `json:"context_max_tokens,omitempty"`
	MaxToolOutputBytes   *int      `json:"max_tool_output_bytes,omitempty"`
	RepeatToolCallLimit  *int      `json:"repeat_tool_call_limit,omitempty"`
	ToolTimeoutSeconds   *int      `json:"tool_timeout_seconds,omitempty"`
	RunTimeoutSeconds    *int      `json:"run_timeout_seconds,omitempty"`
	StreamTimeoutSeconds *int      `json:"stream_timeout_seconds,omitempty"`
	RequestRetries       *int      `json:"request_retries,omitempty"`
	BlockedTools         *[]string `json:"blocked_tools,omitempty"`
}

type HeartbeatPatch struct {
	Active  *bool   `json:"active,omitempty"`
	Channel *string `json:"channel,omitempty"`
	To      *string `json:"to,omitempty"`
	Every   *string `json:"every,omitempty"`
}

type CronPatch struct {
	Active *bool `json:"active,omitempty"`
}

type SecretPatch struct {
	Provider *string `json:"provider,omitempty"`
	APIKey   *string `json:"api_key,omitempty"`
}

// ConfigResetRequest names the sections and bot fields to restore to their
// template defaults. It is used by PATCH /config/reset.
type ConfigResetRequest struct {
	Bot         []string `json:"bot,omitempty"`       // bot field names; an empty list resets the whole bot section
	Heartbeat   *bool    `json:"heartbeat,omitempty"` // true resets the whole section
	Cron        *bool    `json:"cron,omitempty"`
	Transcriber *bool    `json:"transcriber,omitempty"`
	Synthesizer *bool    `json:"synthesizer,omitempty"`
}

// Patch applies a partial update and returns the resulting config view.
func (s *ConfigService) Patch(agentName string, patch ConfigPatch) (ConfigView, error) {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ConfigView{}, err
	}
	if err := applyConfigPatch(cfg, patch); err != nil {
		return ConfigView{}, err
	}
	if err := saveAgentConfig(agentName, cfg); err != nil {
		return ConfigView{}, err
	}
	return configView(cfg), nil
}

// ResetFields restores only the requested sections and bot fields.
func (s *ConfigService) ResetFields(agentName string, req ConfigResetRequest) (ConfigView, error) {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ConfigView{}, err
	}
	tmpl, err := templateConfig()
	if err != nil {
		return ConfigView{}, err
	}

	if req.Bot != nil {
		resetBotFields(cfg, tmpl, req.Bot)
	}
	if req.Heartbeat != nil && *req.Heartbeat {
		cfg.Heartbeat = tmpl.Heartbeat
	}
	if req.Cron != nil && *req.Cron {
		cfg.Cron = tmpl.Cron
	}
	if req.Transcriber != nil && *req.Transcriber {
		cfg.Transcriber = tmpl.Transcriber
	}
	if req.Synthesizer != nil && *req.Synthesizer {
		cfg.Synthesizer = tmpl.Synthesizer
	}

	if err := saveAgentConfig(agentName, cfg); err != nil {
		return ConfigView{}, err
	}
	return configView(cfg), nil
}

// ResetConfig restores the whole config.json to the embedded template.
func (s *ConfigService) ResetConfig(agentName string) (ConfigView, error) {
	tmpl, err := templateConfig()
	if err != nil {
		return ConfigView{}, err
	}
	if err := saveAgentConfig(agentName, tmpl); err != nil {
		return ConfigView{}, err
	}
	return configView(tmpl), nil
}

func applyConfigPatch(cfg *config.Config, patch ConfigPatch) error {
	if patch.Bot != nil {
		if err := applyBotPatch(&cfg.Bot, *patch.Bot); err != nil {
			return err
		}
	}
	if patch.Heartbeat != nil {
		if err := applyHeartbeatPatch(&cfg.Heartbeat, *patch.Heartbeat); err != nil {
			return err
		}
	}
	if patch.Cron != nil && patch.Cron.Active != nil {
		cfg.Cron.Active = *patch.Cron.Active
	}
	if patch.Transcriber != nil {
		applySecretPatch(&cfg.Transcriber.Provider, &cfg.Transcriber.APIKey, *patch.Transcriber)
	}
	if patch.Synthesizer != nil {
		applySecretPatch(&cfg.Synthesizer.Provider, &cfg.Synthesizer.APIKey, *patch.Synthesizer)
	}
	return nil
}

func applyBotPatch(bot *config.GlobalBotConfig, patch BotPatch) error {
	if patch.Type != nil {
		t := strings.ToLower(strings.TrimSpace(*patch.Type))
		if t != "private" && t != "public" {
			return ErrInvalidConfig
		}
		bot.Type = t
	}
	if patch.OwnerIDs != nil {
		bot.OwnerIDs = append([]string{}, (*patch.OwnerIDs)...)
	}
	if patch.Watermark != nil {
		bot.Watermark = *patch.Watermark
	}
	if patch.Stream != nil {
		bot.Stream = cpBoolPtr(*patch.Stream)
	}
	if patch.MaxIterations != nil {
		if *patch.MaxIterations < 1 {
			return ErrInvalidConfig
		}
		bot.MaxIterations = cpIntPtr(*patch.MaxIterations)
	}
	if patch.WarningIterations != nil {
		bot.WarningIterations = cpIntPtr(*patch.WarningIterations)
	}
	if patch.ContextMaxTokens != nil {
		bot.ContextMaxTokens = cpIntPtr(*patch.ContextMaxTokens)
	}
	if patch.MaxToolOutputBytes != nil {
		bot.MaxToolOutputBytes = cpIntPtr(*patch.MaxToolOutputBytes)
	}
	if patch.RepeatToolCallLimit != nil {
		bot.RepeatToolCallLimit = cpIntPtr(*patch.RepeatToolCallLimit)
	}
	if patch.ToolTimeoutSeconds != nil {
		bot.ToolTimeoutSeconds = cpIntPtr(*patch.ToolTimeoutSeconds)
	}
	if patch.RunTimeoutSeconds != nil {
		bot.RunTimeoutSeconds = cpIntPtr(*patch.RunTimeoutSeconds)
	}
	if patch.StreamTimeoutSeconds != nil {
		bot.StreamTimeoutSeconds = cpIntPtr(*patch.StreamTimeoutSeconds)
	}
	if patch.RequestRetries != nil {
		bot.RequestRetries = cpIntPtr(*patch.RequestRetries)
	}
	if patch.BlockedTools != nil {
		bot.BlockedTools = append([]string{}, (*patch.BlockedTools)...)
	}
	if bot.MaxIterations != nil && bot.WarningIterations != nil && *bot.WarningIterations >= *bot.MaxIterations {
		return ErrInvalidConfig
	}
	return nil
}

func applyHeartbeatPatch(hb *config.HeartbeatConfig, patch HeartbeatPatch) error {
	if patch.Active != nil {
		hb.Active = *patch.Active
	}
	if patch.Channel != nil {
		channel := strings.ToLower(strings.TrimSpace(*patch.Channel))
		if channel != "" && channel != "telegram" && channel != "discord" {
			return ErrInvalidConfig
		}
		hb.Channel = channel
	}
	if patch.To != nil {
		hb.To = strings.TrimSpace(*patch.To)
	}
	if patch.Every != nil {
		every := strings.TrimSpace(*patch.Every)
		if d, err := time.ParseDuration(every); err != nil || d <= 0 {
			return ErrInvalidConfig
		}
		hb.Every = every
	}
	return nil
}

func applySecretPatch(provider, apiKey *string, patch SecretPatch) {
	if patch.Provider != nil {
		*provider = strings.TrimSpace(*patch.Provider)
	}
	if patch.APIKey != nil {
		*apiKey = strings.TrimSpace(*patch.APIKey)
	}
}

// resetBotFields copies the named bot fields from the template. An empty list
// resets the whole bot section.
func resetBotFields(cfg, tmpl *config.Config, fields []string) {
	if len(fields) == 0 {
		cfg.Bot = tmpl.Bot
		return
	}
	for _, field := range fields {
		switch field {
		case "type":
			cfg.Bot.Type = tmpl.Bot.Type
		case "owner_ids":
			cfg.Bot.OwnerIDs = append([]string{}, tmpl.Bot.OwnerIDs...)
		case "watermark":
			cfg.Bot.Watermark = tmpl.Bot.Watermark
		case "stream":
			cfg.Bot.Stream = tmpl.Bot.Stream
		case "max_iterations":
			cfg.Bot.MaxIterations = tmpl.Bot.MaxIterations
		case "warning_iterations":
			cfg.Bot.WarningIterations = tmpl.Bot.WarningIterations
		case "context_max_tokens":
			cfg.Bot.ContextMaxTokens = tmpl.Bot.ContextMaxTokens
		case "max_tool_output_bytes":
			cfg.Bot.MaxToolOutputBytes = tmpl.Bot.MaxToolOutputBytes
		case "repeat_tool_call_limit":
			cfg.Bot.RepeatToolCallLimit = tmpl.Bot.RepeatToolCallLimit
		case "tool_timeout_seconds":
			cfg.Bot.ToolTimeoutSeconds = tmpl.Bot.ToolTimeoutSeconds
		case "run_timeout_seconds":
			cfg.Bot.RunTimeoutSeconds = tmpl.Bot.RunTimeoutSeconds
		case "stream_timeout_seconds":
			cfg.Bot.StreamTimeoutSeconds = tmpl.Bot.StreamTimeoutSeconds
		case "request_retries":
			cfg.Bot.RequestRetries = tmpl.Bot.RequestRetries
		case "blocked_tools":
			cfg.Bot.BlockedTools = append([]string{}, tmpl.Bot.BlockedTools...)
		}
	}
}

// templateConfig reads the embedded config template, which doubles as the
// single source of truth for default values.
func templateConfig() (*config.Config, error) {
	data, err := myaaw.DefaultMyaawDir.ReadFile(".myaaw/config/config.json")
	if err != nil {
		return nil, err
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func cpBoolPtr(v bool) *bool { return &v }
func cpIntPtr(v int) *int    { return &v }
