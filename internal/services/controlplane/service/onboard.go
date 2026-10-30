package service

import (
	"fmt"
	"strings"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/config"
	"myaaw/internal/onboard"
	"myaaw/internal/provider"
)

// OnboardProviderInput configures the LLM provider in one step.
type OnboardProviderInput struct {
	Name         string `json:"name,omitempty" example:"deepseek"`
	Type         string `json:"type" example:"openai"`
	BaseURL      string `json:"base_url,omitempty" example:"https://api.deepseek.com"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultModel string `json:"model,omitempty" example:"deepseek-chat"`
}

// OnboardChannelsInput configures Telegram and/or Discord.
type OnboardChannelsInput struct {
	TelegramToken      string `json:"telegram_token,omitempty"`
	TelegramMode       string `json:"telegram_mode,omitempty" example:"polling"`
	TelegramWebhookURL string `json:"telegram_webhook_url,omitempty"`
	DiscordToken       string `json:"discord_token,omitempty"`
	DisableTelegram    bool   `json:"disable_telegram,omitempty"`
	DisableDiscord     bool   `json:"disable_discord,omitempty"`
}

// OnboardHeartbeatInput configures the periodic self-check.
type OnboardHeartbeatInput struct {
	Active  bool   `json:"active"`
	Channel string `json:"channel,omitempty" example:"telegram"`
	To      string `json:"to,omitempty" example:"123456789"`
	Every   string `json:"every,omitempty" example:"30m"`
}

// OnboardBotInput configures access and owner IDs.
type OnboardBotInput struct {
	Type     string   `json:"type,omitempty" example:"private"`
	OwnerIDs []string `json:"owner_ids,omitempty"`
}

// OnboardRequest is the one-shot create/reconfigure payload. Omitting a section
// keeps its current value; on a new agent the omitted parts fall back to the
// template defaults.
type OnboardRequest struct {
	Provider     *OnboardProviderInput  `json:"provider,omitempty"`
	Skills       []string               `json:"skills,omitempty"`
	DisableTools *[]string              `json:"disable_tools,omitempty"`
	Channels     *OnboardChannelsInput  `json:"channels,omitempty"`
	Heartbeat    *OnboardHeartbeatInput `json:"heartbeat,omitempty"`
	Bot          *OnboardBotInput       `json:"bot,omitempty"`
}

// OnboardResult reports what was written.
type OnboardResult struct {
	Created  bool       `json:"created"`
	Config   ConfigView `json:"config"`
	Warnings []string   `json:"warnings,omitempty"`
}

// Onboard creates an agent when the name is free and reconfigures it otherwise,
// using the same core as `myaaw onboard` / `agent create`.
func (s *AgentService) Onboard(name string, req OnboardRequest) (OnboardResult, error) {
	if err := agentdir.ValidateName(name); err != nil {
		return OnboardResult{}, fmt.Errorf("%w: %v", ErrInvalidAgentName, err)
	}
	existed := agentdir.AgentInitialized(name)

	input, err := buildOnboardInput(name, req)
	if err != nil {
		return OnboardResult{}, err
	}

	// setActive is false: the API always targets an explicit agent and must not
	// change the control-plane process's active agent.
	if _, err := onboard.Initialize(name, false); err != nil {
		return OnboardResult{}, err
	}
	if _, warnings, err := onboard.Apply(name, input, 0); err != nil {
		return OnboardResult{}, err
	} else {
		view, viewErr := NewConfigService().Config(name)
		if viewErr != nil {
			return OnboardResult{}, viewErr
		}
		return OnboardResult{Created: !existed, Config: view, Warnings: warnings}, nil
	}
}

func buildOnboardInput(name string, req OnboardRequest) (onboard.Input, error) {
	var in onboard.Input
	existing := readConfigIfExists(name)

	if req.Provider != nil {
		p := req.Provider
		providerType := strings.ToLower(strings.TrimSpace(p.Type))
		if providerType != "openai" && providerType != "gemini" {
			return in, fmt.Errorf("%w: provider.type must be openai or gemini", ErrInvalidConfig)
		}
		providerName := strings.TrimSpace(p.Name)
		if providerName == "" {
			providerName = providerType
		}

		var saved config.ProviderConfig
		if existing != nil {
			saved = existing.Providers[providerName]
		}

		baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
		if baseURL == "" {
			baseURL = saved.BaseURL
		}
		if baseURL == "" {
			baseURL = provider.DefaultBaseURL(providerType)
		}
		if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
			return in, fmt.Errorf("%w: provider base_url must start with http:// or https://", ErrInvalidConfig)
		}

		apiKey := strings.TrimSpace(p.APIKey)
		if apiKey == "" {
			apiKey = saved.APIKey
		}
		if onboard.IsTemplateValue(apiKey) {
			apiKey = ""
		}
		if apiKey == "" {
			return in, fmt.Errorf("%w: provider api_key is required", ErrInvalidConfig)
		}

		model := strings.TrimSpace(p.DefaultModel)
		if model == "" {
			model = saved.DefaultModel
		}
		if model == "" {
			model = provider.DefaultModel(providerType)
		}
		if model == "" {
			return in, fmt.Errorf("%w: provider model is required", ErrInvalidConfig)
		}

		in.ProviderName = providerName
		in.ProviderType = providerType
		in.ProviderBaseURL = baseURL
		in.ProviderAPIKey = apiKey
		in.ProviderModel = model
	}

	if req.Channels != nil {
		c := req.Channels
		if token := strings.TrimSpace(c.TelegramToken); token != "" {
			if err := onboard.ValidateTelegramToken(token); err != nil {
				return in, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
			}
			mode := strings.ToLower(strings.TrimSpace(c.TelegramMode))
			if mode == "" {
				mode = "polling"
			}
			if mode != "polling" && mode != "webhook" {
				return in, fmt.Errorf("%w: telegram_mode must be polling or webhook", ErrInvalidConfig)
			}
			webhookURL := strings.TrimRight(strings.TrimSpace(c.TelegramWebhookURL), "/")
			if mode == "webhook" {
				if err := onboard.ValidateWebhookURL(webhookURL); err != nil {
					return in, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
				}
			}
			in.TelegramToken = token
			in.TelegramMode = mode
			in.TelegramWebhookURL = webhookURL
		}
		in.DiscordToken = strings.TrimSpace(c.DiscordToken)
		in.DisableTelegram = c.DisableTelegram
		in.DisableDiscord = c.DisableDiscord
	}

	if req.Heartbeat != nil {
		h := req.Heartbeat
		in.HeartbeatSet = true
		in.Heartbeat = h.Active
		if h.Active {
			channel := strings.ToLower(strings.TrimSpace(h.Channel))
			if channel != "telegram" && channel != "discord" {
				return in, fmt.Errorf("%w: heartbeat.channel must be telegram or discord", ErrInvalidConfig)
			}
			to := strings.TrimSpace(h.To)
			if err := onboard.ValidateChatID(to); err != nil {
				return in, fmt.Errorf("%w: heartbeat.to: %v", ErrInvalidConfig, err)
			}
			every := strings.TrimSpace(h.Every)
			if every == "" {
				every = "30m"
			}
			if d, err := time.ParseDuration(every); err != nil || d <= 0 {
				return in, fmt.Errorf("%w: heartbeat.every must be a positive duration", ErrInvalidConfig)
			}
			in.HeartbeatChannel = channel
			in.HeartbeatTo = to
			in.HeartbeatEvery = every
		}
	}

	if req.Bot != nil {
		if botType := strings.ToLower(strings.TrimSpace(req.Bot.Type)); botType != "" {
			if botType != "private" && botType != "public" {
				return in, fmt.Errorf("%w: bot.type must be private or public", ErrInvalidConfig)
			}
			in.BotType = botType
		}
		if req.Bot.OwnerIDs != nil {
			in.OwnersSet = true
			in.OwnerIDs = append([]string{}, req.Bot.OwnerIDs...)
		}
	}

	if len(req.Skills) > 0 {
		skills, err := onboard.ParseOnboardSelection(strings.Join(req.Skills, ","), onboard.BuiltinSkillNames(), "skills")
		if err != nil {
			return in, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
		in.Skills = skills
	}

	if req.DisableTools != nil {
		disabled, err := onboard.ParseOnboardSelection(strings.Join(*req.DisableTools, ","), onboard.BuiltinToolNames(), "disable_tools")
		if err != nil {
			return in, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
		in.DisabledTools = disabled
		in.UpdateTools = true
	}

	return in, nil
}

// readConfigIfExists reads config.json without requiring the agent to be
// initialized, so creating a brand-new agent can still reuse defaults.
func readConfigIfExists(name string) *config.Config {
	path, err := agentdir.ConfigPath(name)
	if err != nil {
		return nil
	}
	cfg, err := onboard.LoadConfig(path)
	if err != nil {
		return nil
	}
	return cfg
}
