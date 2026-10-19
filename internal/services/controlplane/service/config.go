package service

import (
	"encoding/json"
	"errors"
	"os"
	"sort"

	"myaaw/internal/agentdir"
	"myaaw/internal/config"
)

var ErrConfigNotFound = errors.New("agent config not found")

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
