package service

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

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
	}, nil
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
	return os.WriteFile(path, data, 0o644)
}
