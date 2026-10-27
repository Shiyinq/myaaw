package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"myaaw"
	"myaaw/internal/agent"
	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup for first-time use",
	Long:  "Create or configure an agent: choose its name, provider, channels, and heartbeat, then write ~/.myaaw/<agent>.",
	Run:   runOnboard,
}

func init() {
	rootCmd.AddCommand(onboardCmd)
}

func runOnboard(cmd *cobra.Command, args []string) {
	agentName := strings.TrimSpace(agentFlag)
	if agentName != "" {
		if err := agentdir.ValidateName(agentName); err != nil {
			fmt.Printf("Invalid agent name: %v\n", err)
			os.Exit(1)
		}
	}
	if err := agentdir.EnsureLayout(); err != nil {
		fmt.Printf("Failed to prepare agent layout: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(initialOnboardModel(agentName), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

// Steps
const (
	stepWelcome = iota
	stepAgentChoice
	stepAgentName
	stepInitHome
	stepProviderType
	stepProviderName
	stepProviderBaseURL
	stepProviderKey
	stepProviderModel
	stepChannelChoice
	stepTelegramMode
	stepTelegramToken
	stepDiscordToken
	stepHeartbeatChannel
	stepHeartbeatID
	stepHeartbeatEvery
	stepOwner
	stepSummary
	stepInstallGlobal
	stepDone
)

const (
	newAgentOption     = "+ Create a new agent"
	manualModelOption  = "Enter model manually"
	defaultHeartbeatEV = "30m"
)

type onboardModel struct {
	step      int
	width     int
	height    int
	textInput textinput.Model
	err       error

	// State for choices
	cursor  int
	choices []string

	// Config Data
	homeDir    string
	agentName  string
	myaawHome  string
	envFile    string
	configFile string

	// Collected Data
	providerType    string // "openai", "gemini"
	providerName    string // custom name stored in config.json
	providerBaseURL string
	providerModel   string
	llmKey          string
	models          []string
	modelsLoading   bool
	modelsErr       error
	modelManual     bool

	channelChoice string // "telegram", "discord", "both", "skip"
	telegramMode  string // "polling", "webhook"
	telegramToken string
	discordToken  string

	heartbeat        bool
	heartbeatChannel string
	heartbeatTo      string
	heartbeatEvery   string

	ownerIDs   []string
	a2aWarning error

	// Flags
	globalPathDone bool

	// Command output
	cmdOutput  []string
	isRunning  bool
	outputChan chan tea.Msg
}

func initialOnboardModel(agentName string) onboardModel {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 48

	home, _ := os.UserHomeDir()
	m := onboardModel{
		step:       stepWelcome,
		textInput:  ti,
		homeDir:    home,
		choices:    []string{"Yes", "No"},
		outputChan: make(chan tea.Msg, 10),
	}
	if agentName != "" {
		m.setAgent(agentName)
	}
	return m
}

// setAgent pins the agent name and recomputes every derived path.
func (m *onboardModel) setAgent(name string) {
	m.agentName = name
	m.myaawHome = filepath.Join(m.homeDir, ".myaaw", name)
	m.envFile = filepath.Join(m.myaawHome, ".env")
	m.configFile = filepath.Join(m.myaawHome, "config", "config.json")
}

func (m onboardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m onboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case modelsFetchedMsg:
		return m.handleModels(msg), nil

	case globalPathFinishedMsg:
		m.isRunning = false
		if msg.err != nil {
			m.globalPathDone = false
			m.cmdOutput = []string{theme.ErrorStyle.Render("Install failed: " + msg.err.Error())}
			m.choices = []string{"Try again", "Skip"}
			m.cursor = 0
			return m, nil
		}
		m.globalPathDone = true
		m.cmdOutput = []string{theme.SuccessStyle.Render("Installed myaaw at " + msg.path)}
		if msg.pathHint != "" {
			m.cmdOutput = append(m.cmdOutput, theme.WarningStyle.Render("⚠  "+msg.pathHint))
		}
		return m, nil

	case tea.KeyMsg:
		if m.isRunning || m.modelsLoading {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			m_next, cmd_next := m.nextStep(msg)
			m = m_next.(onboardModel)
			return m, cmd_next
		case "up", "k":
			if !m.isTextInputStep() && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if !m.isTextInputStep() && m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		default:
			if m.isTextInputStep() {
				m.textInput, cmd = m.textInput.Update(msg)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, cmd
}

func (m onboardModel) View() string {
	timeline := m.renderTimeline()
	content := m.renderContent()
	if m.err != nil {
		content += "\n\n" + theme.ErrorStyle.Render("⚠  "+m.err.Error())
	}

	mainStyle := lipgloss.NewStyle().Margin(1, 1)
	return mainStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, timeline, content))
}

// --- Logic ---

func (m onboardModel) isTextInputStep() bool {
	switch m.step {
	case stepAgentName, stepProviderName, stepProviderBaseURL, stepProviderKey,
		stepTelegramToken, stepDiscordToken, stepHeartbeatID, stepHeartbeatEvery, stepOwner:
		return true
	case stepProviderModel:
		return m.modelManual && !m.modelsLoading
	default:
		return false
	}
}

func (m onboardModel) nextStep(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.err = nil

	switch m.step {
	case stepWelcome:
		switch {
		case m.agentName != "":
			m.step = stepInitHome
		case len(m.initializedAgents()) > 0:
			m.step = stepAgentChoice
			m.cursor = 0
			m.choices = append(m.initializedAgents(), newAgentOption)
		default:
			m.step = stepAgentName
			m.resetInput("myagent", false)
		}

	case stepAgentChoice:
		choice := m.choices[m.cursor]
		if choice == newAgentOption {
			m.step = stepAgentName
			m.resetInput("myagent", false)
			return m, nil
		}
		m.setAgent(choice)
		m.step = stepInitHome

	case stepAgentName:
		name := strings.TrimSpace(m.textInput.Value())
		if err := validateAgentNameInput(name); err != nil {
			m.err = err
			return m, nil
		}
		m.setAgent(name)
		m.step = stepInitHome

	case stepInitHome:
		if err := m.initAgent(); err != nil {
			m.err = err
			return m, nil
		}
		m.step = stepProviderType
		m.cursor = 0
		m.choices = []string{"OpenAI (or compatible)", "Gemini"}

	case stepProviderType:
		if m.cursor == 1 {
			m.providerType = "gemini"
		} else {
			m.providerType = "openai"
		}
		m.providerName = m.providerType
		m.step = stepProviderName
		m.resetInput(m.providerType, false)

	case stepProviderName:
		name := strings.TrimSpace(m.textInput.Value())
		if name == "" {
			name = m.providerType
		}
		if strings.ContainsAny(name, " \t") {
			m.err = fmt.Errorf("provider name cannot contain spaces")
			return m, nil
		}
		m.providerName = name
		m.providerBaseURL = provider.DefaultBaseURL(m.providerType)
		m.step = stepProviderBaseURL
		m.resetInput(m.providerBaseURL, false)

	case stepProviderBaseURL:
		value := strings.TrimRight(strings.TrimSpace(m.textInput.Value()), "/")
		if value == "" {
			value = provider.DefaultBaseURL(m.providerType)
		}
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			m.err = fmt.Errorf("base URL must start with http:// or https://")
			return m, nil
		}
		m.providerBaseURL = value
		m.step = stepProviderKey
		m.resetInput("API key", true)

	case stepProviderKey:
		key := strings.TrimSpace(m.textInput.Value())
		if key == "" {
			m.err = fmt.Errorf("API key is required")
			return m, nil
		}
		m.llmKey = key
		m.step = stepProviderModel
		m.modelsLoading = true
		m.modelManual = false
		m.modelsErr = nil
		m.models = nil
		m.choices = nil
		m.resetInput("", false)
		return m, fetchProviderModels(m.providerType, m.providerBaseURL, m.llmKey)

	case stepProviderModel:
		if m.modelsLoading {
			return m, nil
		}
		if m.modelManual {
			model := strings.TrimSpace(m.textInput.Value())
			if model == "" {
				m.err = fmt.Errorf("model name is required")
				return m, nil
			}
			m.providerModel = model
		} else {
			if len(m.choices) == 0 {
				return m, nil
			}
			choice := m.choices[m.cursor]
			if choice == manualModelOption {
				m.modelManual = true
				m.choices = nil
				m.resetInput("e.g. gpt-4o", false)
				return m, nil
			}
			m.providerModel = choice
		}
		m.step = stepChannelChoice
		m.cursor = 0
		m.choices = []string{"Telegram", "Discord", "Both", "Skip"}

	case stepChannelChoice:
		m.channelChoice = strings.ToLower(m.choices[m.cursor])
		switch m.channelChoice {
		case "skip":
			m.setupHeartbeatChannel()
		case "discord":
			m.step = stepDiscordToken
			m.resetInput("Discord Bot Token", true)
		default: // Telegram or Both
			m.step = stepTelegramMode
			m.cursor = 0
			m.choices = []string{"Polling (Recommended)", "Webhook"}
		}

	case stepTelegramMode:
		if m.cursor == 0 {
			m.telegramMode = "polling"
		} else {
			m.telegramMode = "webhook"
		}
		m.step = stepTelegramToken
		m.resetInput("Telegram Bot Token", true)

	case stepTelegramToken:
		token := strings.TrimSpace(m.textInput.Value())
		if err := validateTelegramToken(token); err != nil {
			m.err = err
			return m, nil
		}
		m.telegramToken = token
		if m.channelChoice == "both" {
			m.step = stepDiscordToken
			m.resetInput("Discord Bot Token", true)
			return m, nil
		}
		m.setupHeartbeatChannel()

	case stepDiscordToken:
		token := strings.TrimSpace(m.textInput.Value())
		if token == "" {
			m.err = fmt.Errorf("Discord bot token is required")
			return m, nil
		}
		m.discordToken = token
		m.setupHeartbeatChannel()

	case stepHeartbeatChannel:
		choice := strings.ToLower(m.choices[m.cursor])
		if choice == "skip" {
			m.heartbeat = false
			m.step = stepOwner
			m.resetInput("123456789, 987654321 (optional, comma separated)", false)
			return m, nil
		}
		m.heartbeat = true
		m.heartbeatChannel = choice
		m.step = stepHeartbeatID
		m.resetInput(heartbeatPlaceholder(choice), false)

	case stepHeartbeatID:
		to := strings.TrimSpace(m.textInput.Value())
		if err := validateChatID(to); err != nil {
			m.err = err
			return m, nil
		}
		m.heartbeatTo = to
		m.step = stepHeartbeatEvery
		if m.heartbeatEvery == "" {
			m.heartbeatEvery = defaultHeartbeatEV
		}
		m.resetInput(defaultHeartbeatEV, false)
		m.textInput.SetValue(m.heartbeatEvery)

	case stepHeartbeatEvery:
		every := strings.TrimSpace(m.textInput.Value())
		d, err := time.ParseDuration(every)
		if err != nil || d <= 0 {
			m.err = fmt.Errorf("interval must be a positive duration such as 30m or 1h")
			return m, nil
		}
		m.heartbeatEvery = every
		m.step = stepOwner
		m.resetInput("123456789, 987654321 (optional, comma separated)", false)

	case stepOwner:
		owners, err := parseOwnerIDs(m.textInput.Value())
		if err != nil {
			m.err = err
			return m, nil
		}
		m.ownerIDs = owners
		m.step = stepSummary
		m.cursor = 0
		m.choices = []string{"Save configuration", "Cancel"}

	case stepSummary:
		if m.cursor == 1 {
			return m, tea.Quit
		}
		if err := m.persist(); err != nil {
			m.err = err
			return m, nil
		}
		m.step = stepInstallGlobal
		m.cursor = 0
		m.choices = []string{"Yes, install globally", "No"}

	case stepInstallGlobal:
		if m.isRunning {
			return m, nil
		}
		if m.globalPathDone {
			m.step = stepDone
			return m, nil
		}
		if len(m.cmdOutput) > 0 && m.choices != nil && m.choices[0] == "Try again" {
			// A previous install attempt failed; retry or continue without it.
			if m.cursor == 1 {
				m.step = stepDone
				return m, nil
			}
		}
		if m.cursor == 0 {
			m.isRunning = true
			m.cmdOutput = []string{"Installing binary..."}
			return m, m.runGlobalPathCmd()
		}
		m.step = stepDone

	case stepDone:
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "enter" {
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

// initAgent creates the agent layout, registers it, assigns a port, and seeds
// A2A contacts so onboarding ends in the same state as `myaaw agent create`.
func (m *onboardModel) initAgent() error {
	if err := setupMyaawHome(m.myaawHome); err != nil {
		return fmt.Errorf("create agent layout: %w", err)
	}
	if err := agentdir.RewriteLegacyPaths(m.myaawHome, m.agentName); err != nil {
		return fmt.Errorf("rewrite legacy paths: %w", err)
	}
	if err := agentdir.RegisterAgent(m.agentName); err != nil {
		return fmt.Errorf("register agent: %w", err)
	}
	if err := agentdir.SetActive(m.agentName); err != nil {
		return err
	}
	if err := ensureDefaultAgent(m.agentName); err != nil {
		return fmt.Errorf("set default agent: %w", err)
	}
	if err := prepareAgentEnv(m.agentName); err != nil {
		return err
	}
	// Seeding rotates the A2A token, so only do it for a brand-new agent.
	envPath, err := agentdir.EnvPath(m.agentName)
	if err != nil {
		return err
	}
	if readEnvValue(envPath, "A2A_TOKEN") == "" {
		if err := agentdir.SeedA2AContacts(m.agentName); err != nil {
			m.a2aWarning = err
		}
	}
	if _, err := agent.EnsureWorkspace(); err != nil {
		return fmt.Errorf("prepare workspace: %w", err)
	}
	return nil
}

// persist writes .env (already prepared) and merges the wizard answers into
// config.json. Fields the user skipped are left untouched, and placeholder
// values from older templates are removed first.
func (m *onboardModel) persist() error {
	if err := prepareAgentEnv(m.agentName); err != nil {
		return err
	}

	cfg, err := loadConfigFromFile(m.configFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config.json: %w", err)
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	sanitizeOnboardConfig(cfg)

	merged := mergeOnboardConfig(cfg, m.input())
	if err := saveConfigToFile(m.configFile, merged); err != nil {
		return fmt.Errorf("write config.json: %w", err)
	}
	return nil
}

func (m onboardModel) input() onboardInput {
	return onboardInput{
		ProviderName:     m.providerName,
		ProviderType:     m.providerType,
		ProviderBaseURL:  m.providerBaseURL,
		ProviderAPIKey:   m.llmKey,
		ProviderModel:    m.providerModel,
		TelegramToken:    m.telegramToken,
		TelegramMode:     m.telegramMode,
		DiscordToken:     m.discordToken,
		Heartbeat:        m.heartbeat,
		HeartbeatChannel: m.heartbeatChannel,
		HeartbeatTo:      m.heartbeatTo,
		HeartbeatEvery:   m.heartbeatEvery,
		OwnerIDs:         m.ownerIDs,
	}
}

func (m *onboardModel) setupHeartbeatChannel() {
	m.step = stepHeartbeatChannel
	m.cursor = 0
	m.choices = nil
	if m.telegramToken != "" {
		m.choices = append(m.choices, "Telegram")
	}
	if m.discordToken != "" {
		m.choices = append(m.choices, "Discord")
	}
	m.choices = append(m.choices, "Skip")
}

func (m *onboardModel) resetInput(placeholder string, password bool) {
	m.textInput.Reset()
	m.textInput.Placeholder = placeholder
	m.textInput.EchoMode = textinput.EchoNormal
	if password {
		m.textInput.EchoMode = textinput.EchoPassword
	}
	m.textInput.Focus()
}

func (m onboardModel) initializedAgents() []string {
	names, err := agentdir.ListAgents()
	if err != nil {
		return nil
	}
	return names
}

func (m onboardModel) runGlobalPathCmd() tea.Cmd {
	go func() {
		if runtime.GOOS == "windows" {
			m.outputChan <- globalPathFinishedMsg{err: fmt.Errorf("automatic install is not supported on Windows; add myaaw to PATH manually")}
			return
		}
		path, err := installBinary()
		if err != nil {
			m.outputChan <- globalPathFinishedMsg{err: err}
			return
		}
		hint := ""
		binDir := filepath.Dir(path)
		if !pathContains(binDir) {
			hint = fmt.Sprintf("%s is not in PATH; add it to your shell profile, then restart the terminal", binDir)
		}
		m.outputChan <- globalPathFinishedMsg{path: path, pathHint: hint}
	}()
	return m.listenForOutput()
}

func (m onboardModel) listenForOutput() tea.Cmd {
	return func() tea.Msg {
		return <-m.outputChan
	}
}

type cmdOutputMsg string

type globalPathFinishedMsg struct {
	path     string
	pathHint string
	err      error
}

type modelsFetchedMsg struct {
	models []string
	err    error
}

// fetchProviderModels asks the provider for its model list so the user can pick
// a real model instead of relying on a hardcoded default.
func fetchProviderModels(providerType, baseURL, apiKey string) tea.Cmd {
	return func() tea.Msg {
		previous := config.LLMProviderBaseURL
		config.LLMProviderBaseURL = baseURL
		defer func() { config.LLMProviderBaseURL = previous }()

		llm, err := provider.CreateLLMProvider(providerType, apiKey)
		if err != nil {
			return modelsFetchedMsg{err: err}
		}
		models, err := llm.Models()
		return modelsFetchedMsg{models: models, err: err}
	}
}

func (m onboardModel) handleModels(msg modelsFetchedMsg) onboardModel {
	if m.step != stepProviderModel {
		return m
	}
	m.modelsLoading = false
	m.modelsErr = msg.err
	m.models = msg.models

	if msg.err != nil || len(msg.models) == 0 {
		m.modelManual = true
		m.choices = nil
		m.resetInput("e.g. gpt-4o", false)
		return m
	}
	m.modelManual = false
	m.choices = append(append([]string{}, msg.models...), manualModelOption)
	m.cursor = 0
	return m
}

// --- Config merge ---

type onboardInput struct {
	ProviderName     string
	ProviderType     string
	ProviderBaseURL  string
	ProviderAPIKey   string
	ProviderModel    string
	TelegramToken    string
	TelegramMode     string
	DiscordToken     string
	Heartbeat        bool
	HeartbeatChannel string
	HeartbeatTo      string
	HeartbeatEvery   string
	OwnerIDs         []string
}

// mergeOnboardConfig applies wizard answers to an existing config. Channels and
// heartbeat are only touched when the user actually configured them, so
// re-running onboarding never silently drops working settings.
func mergeOnboardConfig(base *config.Config, in onboardInput) *config.Config {
	cfg := &config.Config{}
	if base != nil {
		*cfg = *base
	}

	// Copy the mutable parts so merging never mutates the caller's config.
	providers := make(map[string]config.ProviderConfig, len(cfg.Providers))
	for name, p := range cfg.Providers {
		providers[name] = p
	}
	cfg.Providers = providers
	if cfg.Channels.Telegram != nil {
		tg := *cfg.Channels.Telegram
		cfg.Channels.Telegram = &tg
	}
	if cfg.Channels.Discord != nil {
		dc := *cfg.Channels.Discord
		cfg.Channels.Discord = &dc
	}
	if in.ProviderName != "" {
		cfg.Providers[in.ProviderName] = config.ProviderConfig{
			Type:         in.ProviderType,
			BaseURL:      in.ProviderBaseURL,
			APIKey:       in.ProviderAPIKey,
			DefaultModel: in.ProviderModel,
		}
		cfg.DefaultProvider = in.ProviderName
	}

	if in.TelegramToken != "" {
		tg := cfg.Channels.Telegram
		if tg == nil {
			tg = &config.ChannelConfig{}
		}
		tg.Active = true
		tg.Token = in.TelegramToken
		tg.Mode = in.TelegramMode
		cfg.Channels.Telegram = tg
	}
	if in.DiscordToken != "" {
		dc := cfg.Channels.Discord
		if dc == nil {
			dc = &config.ChannelConfig{}
		}
		dc.Active = true
		dc.Token = in.DiscordToken
		cfg.Channels.Discord = dc
	}

	if in.Heartbeat {
		cfg.Heartbeat = config.HeartbeatConfig{
			Active:  true,
			Channel: in.HeartbeatChannel,
			To:      in.HeartbeatTo,
			Every:   in.HeartbeatEvery,
		}
	}

	if len(in.OwnerIDs) > 0 {
		cfg.Bot.OwnerIDs = append([]string{}, in.OwnerIDs...)
	}

	return cfg
}

const templateHeartbeatTo = "123456789"

// sanitizeOnboardConfig strips values that shipped with the old template so a
// fresh install never starts a bot with a fake token or blocks every real user.
func sanitizeOnboardConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for name, p := range cfg.Providers {
		if isTemplateValue(p.APIKey) {
			delete(cfg.Providers, name)
			if cfg.DefaultProvider == name {
				cfg.DefaultProvider = ""
			}
		}
	}
	if cfg.Channels.Telegram != nil && isTemplateValue(cfg.Channels.Telegram.Token) {
		cfg.Channels.Telegram.Token = ""
		cfg.Channels.Telegram.Active = false
	}
	if cfg.Channels.Discord != nil && isTemplateValue(cfg.Channels.Discord.Token) {
		cfg.Channels.Discord.Token = ""
		cfg.Channels.Discord.Active = false
	}
	// Drop empty, unused channel blocks so new agents start with a clean file.
	if cfg.Channels.Telegram != nil && !cfg.Channels.Telegram.Active && strings.TrimSpace(cfg.Channels.Telegram.Token) == "" {
		cfg.Channels.Telegram = nil
	}
	if cfg.Channels.Discord != nil && !cfg.Channels.Discord.Active && strings.TrimSpace(cfg.Channels.Discord.Token) == "" {
		cfg.Channels.Discord = nil
	}
	if isTemplateValue(cfg.Heartbeat.To) || cfg.Heartbeat.To == templateHeartbeatTo {
		cfg.Heartbeat.Active = false
		cfg.Heartbeat.To = ""
	}
	if isTemplateValue(cfg.Transcriber.APIKey) {
		cfg.Transcriber = config.TranscriberConfig{}
	}
	if isTemplateValue(cfg.Synthesizer.APIKey) {
		cfg.Synthesizer = config.SynthesizerConfig{}
	}

	owners := cfg.Bot.OwnerIDs[:0]
	for _, id := range cfg.Bot.OwnerIDs {
		if isTemplateValue(id) {
			continue
		}
		owners = append(owners, id)
	}
	cfg.Bot.OwnerIDs = owners
}

func isTemplateValue(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "YOUR_")
}

func ensureDefaultAgent(name string) error {
	registry, err := agentdir.LoadRegistry()
	if err != nil {
		return err
	}
	if registry.Default != "" && agentdir.AgentInitialized(registry.Default) {
		return nil
	}
	return agentdir.SetDefault(name)
}

// --- Input validation ---

func validateAgentNameInput(name string) error {
	if name == "" {
		return fmt.Errorf("agent name is required")
	}
	if err := agentdir.ValidateName(name); err != nil {
		return err
	}
	if agentdir.AgentInitialized(name) {
		return fmt.Errorf("agent %q is already initialized; use --agent %s onboard to reconfigure it", name, name)
	}
	root, err := agentdir.AgentRoot(name)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(root); statErr == nil {
		return fmt.Errorf("%s already exists; choose another name or use --agent %s onboard", root, name)
	}
	return nil
}

func validateTelegramToken(token string) error {
	if token == "" {
		return fmt.Errorf("Telegram bot token is required")
	}
	if !strings.Contains(token, ":") {
		return fmt.Errorf("Telegram bot token looks invalid (expected <bot-id>:<secret>)")
	}
	return nil
}

func validateChatID(value string) error {
	if value == "" {
		return fmt.Errorf("chat ID is required")
	}
	digits := strings.TrimPrefix(value, "-")
	if digits == "" {
		return fmt.Errorf("chat ID must be numeric")
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return fmt.Errorf("chat ID %q must be numeric", value)
		}
	}
	return nil
}

func parseOwnerIDs(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	seen := make(map[string]bool)
	var ids []string
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if err := validateChatID(id); err != nil {
			return nil, err
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

func heartbeatPlaceholder(channel string) string {
	if channel == "discord" {
		return "Discord channel ID"
	}
	return "Telegram chat ID (use a negative ID for groups)"
}

// --- Rendering ---

func (m onboardModel) renderContent() string {
	var b strings.Builder

	padding := lipgloss.NewStyle().PaddingLeft(4)
	titleStyle := theme.HighlightStyle.MarginBottom(1)

	switch m.step {
	case stepWelcome:
		b.WriteString(titleStyle.Render("Welcome to Myaaw!"))
		b.WriteString("\n\nThis wizard creates one isolated agent and configures its\nprovider, channels, and heartbeat.\n\n")
		b.WriteString(theme.SecondaryStyle.Render("Press Enter to start."))

	case stepAgentChoice:
		b.WriteString(titleStyle.Render("Select Agent"))
		b.WriteString("\n\nWhich agent do you want to configure?\n\n")
		b.WriteString(m.renderChoices())

	case stepAgentName:
		b.WriteString(titleStyle.Render("Agent Name"))
		b.WriteString("\n\nThis becomes the folder name under ~/.myaaw/.\n")
		b.WriteString(theme.SecondaryStyle.Render("Letters, numbers, '_' and '-' only. Example: personal, agent-b, work-2025.") + "\n\n")
		b.WriteString(m.textInput.View())

	case stepInitHome:
		b.WriteString(titleStyle.Render("Initialize Agent"))
		b.WriteString(fmt.Sprintf("\n\nAgent: %s\nDirectory: %s\n\n", m.agentName, m.myaawHome))
		b.WriteString(theme.SecondaryStyle.Render("Press Enter to create the layout."))

	case stepProviderType:
		b.WriteString(titleStyle.Render("LLM Provider"))
		b.WriteString("\n\nChoose the AI engine for this agent:\n\n")
		b.WriteString(m.renderChoices())

	case stepProviderName:
		b.WriteString(titleStyle.Render("Provider Name"))
		b.WriteString("\n\nName stored in config.json. Press Enter to keep the default.\n\n")
		b.WriteString(m.textInput.View())

	case stepProviderBaseURL:
		b.WriteString(titleStyle.Render("Base URL"))
		b.WriteString("\n\nLeave empty for the public endpoint.\n")
		b.WriteString(theme.SecondaryStyle.Render("Use this for OpenAI-compatible gateways, DeepSeek, Ollama, etc.") + "\n\n")
		b.WriteString(m.textInput.View())

	case stepProviderKey:
		b.WriteString(titleStyle.Render(strings.Title(m.providerName) + " API Key"))
		b.WriteString("\n\nStored in config/config.json and masked by `myaaw config dump`.\n\n")
		b.WriteString(m.textInput.View())

	case stepProviderModel:
		b.WriteString(titleStyle.Render("Default Model"))
		switch {
		case m.modelsLoading:
			b.WriteString("\n\n" + theme.SecondaryStyle.Render("Fetching available models from "+m.providerName+"..."))
		case m.modelManual:
			b.WriteString("\n\n")
			if m.modelsErr != nil {
				b.WriteString(theme.WarningStyle.Render("⚠  Could not fetch models: "+m.modelsErr.Error()) + "\n\n")
			}
			b.WriteString("Enter the model name manually:\n\n")
			b.WriteString(m.textInput.View())
		default:
			b.WriteString("\n\nPick the model this agent should use by default:\n\n")
			b.WriteString(m.renderChoices())
		}

	case stepChannelChoice:
		b.WriteString(titleStyle.Render("Select Channels"))
		b.WriteString("\n\nWhere should your bot live?\n\n")
		b.WriteString(m.renderChoices())

	case stepTelegramMode:
		b.WriteString(titleStyle.Render("Telegram Connection Mode"))
		b.WriteString("\n\nPolling needs no public URL. Webhook needs a public domain.\n\n")
		b.WriteString(m.renderChoices())

	case stepTelegramToken:
		b.WriteString(titleStyle.Render("Telegram Bot Token"))
		b.WriteString("\n\nGet this from @BotFather on Telegram.\n\n")
		b.WriteString(m.textInput.View())

	case stepDiscordToken:
		b.WriteString(titleStyle.Render("Discord Bot Token"))
		b.WriteString("\n\nGet this from the Discord Developer Portal.\n\n")
		b.WriteString(m.textInput.View())

	case stepHeartbeatChannel:
		b.WriteString(titleStyle.Render("Heartbeat Channel"))
		b.WriteString("\n\nShould the agent send a periodic self-check?\n\n")
		b.WriteString(m.renderChoices())

	case stepHeartbeatID:
		b.WriteString(titleStyle.Render("Heartbeat Destination"))
		b.WriteString(fmt.Sprintf("\n\nEnter the ID on %s:\n\n", strings.Title(m.heartbeatChannel)))
		b.WriteString(m.textInput.View())

	case stepHeartbeatEvery:
		b.WriteString(titleStyle.Render("Heartbeat Interval"))
		b.WriteString("\n\nHow often should the heartbeat run?\n")
		b.WriteString(theme.SecondaryStyle.Render("Use a Go duration: 15m, 30m, 1h, 6h.") + "\n\n")
		b.WriteString(m.textInput.View())

	case stepOwner:
		b.WriteString(titleStyle.Render("Bot Owner"))
		b.WriteString("\n\nIDs allowed to talk to the bot when bot.type is private.\n")
		b.WriteString(theme.SecondaryStyle.Render("Comma separated. Leave empty only if you plan to allow everyone.") + "\n\n")
		b.WriteString(m.textInput.View())

	case stepSummary:
		b.WriteString(titleStyle.Render("Review"))
		b.WriteString("\n\n")
		b.WriteString(m.renderSummary())
		b.WriteString("\n")
		b.WriteString(m.renderChoices())

	case stepInstallGlobal:
		b.WriteString(titleStyle.Render("Install Globally"))
		switch {
		case m.isRunning:
			b.WriteString("\n\n" + theme.SecondaryStyle.Render("Installing binary..."))
		case m.globalPathDone:
			b.WriteString("\n\n" + strings.Join(m.cmdOutput, "\n"))
			b.WriteString("\n\n" + theme.SecondaryStyle.Render("Press Enter to finish."))
		default:
			if len(m.cmdOutput) > 0 {
				b.WriteString("\n\n" + strings.Join(m.cmdOutput, "\n"))
			}
			b.WriteString("\n\nCopy the binary to ~/.local/bin so `myaaw` works anywhere?\n\n")
			b.WriteString(m.renderChoices())
		}

	case stepDone:
		b.WriteString(theme.SuccessStyle.Render("✨ Onboarding Complete!"))
		b.WriteString(fmt.Sprintf("\n\nAgent: %s\nConfig: %s\n\n", m.agentName, m.configFile))
		b.WriteString("Next steps:\n")
		b.WriteString(theme.HighlightStyle.Render(fmt.Sprintf("  myaaw --agent %s status", m.agentName)) + "\n")
		b.WriteString(theme.HighlightStyle.Render(fmt.Sprintf("  myaaw --agent %s chat", m.agentName)) + "\n")
		b.WriteString(theme.HighlightStyle.Render(fmt.Sprintf("  myaaw server --agent %s start", m.agentName)) + "\n\n")
		b.WriteString(theme.SecondaryStyle.Render("Press Enter to finish and exit."))
	}

	return padding.Render(b.String())
}

func (m onboardModel) renderSummary() string {
	var b strings.Builder
	row := func(label, value string) {
		b.WriteString(fmt.Sprintf("%-12s %s\n", label+":", value))
	}

	row("Agent", fmt.Sprintf("%s  (%s)", m.agentName, m.myaawHome))
	row("Port", m.port())
	row("Provider", fmt.Sprintf("%s [%s]", m.providerName, m.providerType))
	row("Base URL", m.providerBaseURL)
	row("Model", m.providerModel)
	row("Telegram", m.channelSummary("telegram", m.telegramToken, m.telegramMode))
	row("Discord", m.channelSummary("discord", m.discordToken, ""))
	if m.heartbeat {
		row("Heartbeat", fmt.Sprintf("%s → %s every %s", m.heartbeatChannel, m.heartbeatTo, m.heartbeatEvery))
	} else {
		row("Heartbeat", "disabled")
	}
	if len(m.ownerIDs) > 0 {
		row("Owner IDs", strings.Join(m.ownerIDs, ", "))
	} else {
		row("Owner IDs", "none")
	}

	if len(m.ownerIDs) == 0 {
		b.WriteString("\n" + theme.WarningStyle.Render("⚠  No owner IDs: with bot.type \"private\" nobody can chat with this bot."))
	}
	if m.providerType != "gemini" {
		b.WriteString("\n" + theme.MutedStyle.Render("ℹ  `myaaw voice` (real-time) needs Gemini; `voice-classic` works with any provider."))
	}
	if m.a2aWarning != nil {
		b.WriteString("\n" + theme.WarningStyle.Render("⚠  A2A contacts were not seeded: "+m.a2aWarning.Error()))
	}
	return b.String()
}

func (m onboardModel) channelSummary(channel, token, mode string) string {
	if token == "" {
		return "not configured"
	}
	if mode != "" {
		return fmt.Sprintf("%s (%s)", mode, channel)
	}
	return "enabled"
}

func (m onboardModel) port() string {
	if port := readEnvValue(m.envFile, "PORT"); port != "" {
		return port
	}
	return "auto"
}

func (m onboardModel) renderChoices() string {
	s := ""
	for i, choice := range m.choices {
		cursor := "  "
		style := theme.BaseStyle
		if m.cursor == i {
			cursor = "👉"
			style = theme.HighlightStyle
		}
		s += fmt.Sprintf("%s %s\n", cursor, style.Render(choice))
	}
	return s
}

func (m onboardModel) renderTimeline() string {
	steps := []string{
		"Welcome",
		"Agent",
		"Initialize",
		"Provider",
		"Channels",
		"Heartbeat",
		"Owner",
		"Review",
		"Install",
		"Done",
	}

	timelineIdx := 0
	switch m.step {
	case stepWelcome:
		timelineIdx = 0
	case stepAgentChoice, stepAgentName:
		timelineIdx = 1
	case stepInitHome:
		timelineIdx = 2
	case stepProviderType, stepProviderName, stepProviderBaseURL, stepProviderKey, stepProviderModel:
		timelineIdx = 3
	case stepChannelChoice, stepTelegramMode, stepTelegramToken, stepDiscordToken:
		timelineIdx = 4
	case stepHeartbeatChannel, stepHeartbeatID, stepHeartbeatEvery:
		timelineIdx = 5
	case stepOwner:
		timelineIdx = 6
	case stepSummary:
		timelineIdx = 7
	case stepInstallGlobal:
		timelineIdx = 8
	case stepDone:
		timelineIdx = 9
	}

	var b strings.Builder
	for i, label := range steps {
		bullet := "○"
		style := theme.MutedStyle

		if i < timelineIdx {
			bullet = "◉"
			style = theme.SuccessStyle
		} else if i == timelineIdx {
			bullet = "◉"
			style = theme.HighlightStyle
		}

		b.WriteString(style.Render(fmt.Sprintf("%s %s", bullet, label)) + "\n")
		if i < len(steps)-1 {
			lineStyle := theme.MutedStyle
			if i < timelineIdx {
				lineStyle = theme.SuccessStyle
			}
			b.WriteString(lineStyle.Render("│") + "\n")
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(lipgloss.Color(theme.ColorMuted)).
		PaddingRight(2).
		Render(b.String())
}

// --- Filesystem helpers ---

func setupMyaawHome(targetDir string) error {
	homeReady := false
	configReady := false
	if _, err := os.Stat(filepath.Join(targetDir, "home", "AGENTS.md")); err == nil {
		homeReady = true
	}
	if _, err := os.Stat(filepath.Join(targetDir, "config", "config.json")); err == nil {
		configReady = true
	}
	if homeReady && configReady {
		return nil
	}
	return extractEmbedDir(myaaw.DefaultMyaawDir, ".myaaw", targetDir)
}

// ensureEnvFile writes the embedded .env template when the file does not
// exist. The bool reports whether the file was created.
func ensureEnvFile(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(myaaw.DefaultEnvExample), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func loadConfigFromFile(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfigToFile(path string, cfg *config.Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func extractEmbedDir(embeddedFS embed.FS, src string, dest string) error {
	entries, err := embeddedFS.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return err
			}
			if err := extractEmbedDir(embeddedFS, srcPath, destPath); err != nil {
				return err
			}
			continue
		}
		data, err := embeddedFS.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
