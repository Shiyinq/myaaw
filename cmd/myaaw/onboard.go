package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"myaaw"
	"myaaw/internal/agent"
	"myaaw/internal/agent/tools"
	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/daemon"
	"myaaw/internal/provider"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
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

// onboardOptions mirrors every question the interactive wizard asks, so the
// same setup can run headless in scripts and CI.
type onboardOptions struct {
	yes       bool
	json      bool
	force     bool
	install   bool
	provider  string
	provName  string
	baseURL   string
	apiKey    string
	model     string
	channel   string
	tgMode    string
	tgToken   string
	webhook   string
	dcToken   string
	hbChannel string
	hbTo      string
	hbEvery   string
	owner     string
	port      int
	restart   bool
	skills    string
	noTools   string
	// disableToolsSet reports whether --disable-tools was passed, so an
	// update without the flag can preserve the current disabled set.
	disableToolsSet bool
}

var onboardOpts onboardOptions

func init() {
	rootCmd.AddCommand(onboardCmd)
	registerOnboardFlags(onboardCmd)
}

// registerOnboardFlags binds the shared options to a command. Both `onboard`
// and `agent create` use it so the two commands stay in sync.
func registerOnboardFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.BoolVar(&onboardOpts.yes, "yes", false, "run without prompts (requires --agent)")
	f.BoolVar(&onboardOpts.json, "json", false, "print a machine-readable summary (implies --yes)")
	f.BoolVar(&onboardOpts.force, "force", false, "reconfigure an agent that already exists")
	f.BoolVar(&onboardOpts.install, "install", false, "copy the binary to ~/.local/bin")
	f.StringVar(&onboardOpts.provider, "provider", "", "provider type: openai or gemini")
	f.StringVar(&onboardOpts.provName, "provider-name", "", "provider name stored in config.json (default: the type)")
	f.StringVar(&onboardOpts.baseURL, "base-url", "", "provider base URL (default: the public endpoint)")
	f.StringVar(&onboardOpts.apiKey, "api-key", "", "provider API key (prefer MYAAW_API_KEY)")
	f.StringVar(&onboardOpts.model, "model", "", "default model (default: the provider default)")
	f.StringVar(&onboardOpts.channel, "channel", "", "channels: telegram, discord, both, or skip")
	f.StringVar(&onboardOpts.tgMode, "telegram-mode", "polling", "telegram mode: polling or webhook")
	f.StringVar(&onboardOpts.tgToken, "telegram-token", "", "telegram bot token (prefer TELEGRAM_BOT_TOKEN)")
	f.StringVar(&onboardOpts.webhook, "webhook-url", "", "public https URL for telegram webhook mode")
	f.StringVar(&onboardOpts.dcToken, "discord-token", "", "discord bot token (prefer DISCORD_BOT_TOKEN)")
	f.StringVar(&onboardOpts.hbChannel, "heartbeat-channel", "", "heartbeat channel: telegram, discord, or skip")
	f.StringVar(&onboardOpts.hbTo, "heartbeat-to", "", "heartbeat destination chat/channel ID")
	f.StringVar(&onboardOpts.hbEvery, "heartbeat-every", "", "heartbeat interval, e.g. 30m or 1h")
	f.StringVar(&onboardOpts.owner, "owner", "", "owner IDs, comma separated")
	f.IntVar(&onboardOpts.port, "port", 0, "agent server port (default: first free port)")
	f.BoolVar(&onboardOpts.restart, "restart", false, "restart the agent daemon after writing the config")
	f.StringVar(&onboardOpts.skills, "skills", "", "skills to install, comma separated (default: none, or 'all')")
	f.StringVar(&onboardOpts.noTools, "disable-tools", "", "built-in tools to disable, comma separated")
}

func runOnboard(cmd *cobra.Command, args []string) {
	onboardOpts.disableToolsSet = cmd.Flags().Changed("disable-tools")
	runOnboarding(strings.TrimSpace(agentFlag), false)
}

// runOnboarding is the single implementation behind `myaaw onboard` and
// `myaaw agent create`. When createNow is true the agent layout is created
// before the TUI starts and the wizard jumps straight to the provider step.
func runOnboarding(agentName string, createNow bool) {
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

	if onboardOpts.yes || onboardOpts.json || !stdinIsTerminal() {
		os.Exit(runOnboardHeadless(agentName))
	}

	p := tea.NewProgram(newOnboardingModel(agentName, createNow), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

// newOnboardingModel builds the wizard state. With createNow the wizard starts
// at the provider step because `myaaw agent create <name>` already knows the
// agent name. Nothing is written to disk until the review step is saved.
func newOnboardingModel(agentName string, createNow bool) onboardModel {
	m := initialOnboardModel(agentName)
	if !createNow || agentName == "" {
		return m
	}
	m.step = stepProviderType
	m.cursor = 0
	m.choices = []string{"OpenAI (or compatible)", "Gemini"}
	return m
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
	stepSkills
	stepTools
	stepChannelChoice
	stepTelegramMode
	stepTelegramToken
	stepTelegramWebhookURL
	stepDiscordToken
	stepHeartbeatChannel
	stepHeartbeatID
	stepHeartbeatEvery
	stepOwner
	stepSummary
	stepInstallGlobal
	stepRestart
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
	homeDir             string
	agentName           string
	agentExistedAtStart bool
	myaawHome           string
	envFile             string
	configFile          string

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

	channelChoice      string // "telegram", "discord", "both", "skip"
	telegramMode       string // "polling", "webhook"
	telegramToken      string
	telegramWebhookURL string
	discordToken       string

	heartbeat        bool
	heartbeatChannel string
	heartbeatTo      string
	heartbeatEvery   string

	ownerIDs []string

	skills         []string
	skillsSelected map[string]bool
	tools          []string
	toolsSelected  map[string]bool

	warnings       []string
	a2aWarning     error
	restartPending bool
	restartMsg     string
	restartWarning error

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
		step:           stepWelcome,
		textInput:      ti,
		homeDir:        home,
		choices:        []string{"Yes", "No"},
		outputChan:     make(chan tea.Msg, 10),
		skills:         builtinSkillNames(),
		tools:          builtinToolNames(),
		skillsSelected: map[string]bool{},
		toolsSelected:  map[string]bool{},
	}
	for _, name := range m.skills {
		m.skillsSelected[name] = false
	}
	for _, name := range m.tools {
		m.toolsSelected[name] = true
	}
	if agentName != "" {
		m.setAgent(agentName)
		if taken, err := agentdir.AgentNameTaken(agentName); err == nil {
			m.agentExistedAtStart = taken
		}
		if m.agentExistedAtStart {
			m.loadExistingCapabilities()
		}
	}
	return m
}

// loadExistingCapabilities pre-selects the skills already installed and the
// tools that are not disabled in tools.json, so re-running onboarding shows the
// current state instead of resetting it.
func (m *onboardModel) loadExistingCapabilities() {
	if dir, err := agentdir.SkillsDir(m.agentName); err == nil {
		if entries, readErr := os.ReadDir(dir); readErr == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					m.skillsSelected[entry.Name()] = true
				}
			}
		}
	}

	if dir, err := agentdir.ToolsDir(m.agentName); err == nil {
		if data, readErr := os.ReadFile(filepath.Join(dir, "tools.json")); readErr == nil {
			var cfg struct {
				BuiltinTools map[string]bool `json:"builtin_tools"`
			}
			if json.Unmarshal(data, &cfg) == nil {
				for name, enabled := range cfg.BuiltinTools {
					canonical := canonicalToolName(name)
					if _, known := m.toolsSelected[canonical]; known {
						m.toolsSelected[canonical] = enabled
					}
				}
			}
		}
	}
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
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.step == stepWelcome {
				return m, tea.Quit
			}
			m = m.previousStep()
			return m, nil
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
		case " ":
			if m.isMultiSelectStep() {
				m.toggleCurrent()
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
		stepTelegramToken, stepTelegramWebhookURL, stepDiscordToken, stepHeartbeatID, stepHeartbeatEvery, stepOwner:
		return true
	case stepProviderModel:
		return m.modelManual && !m.modelsLoading
	default:
		return false
	}
}

func (m onboardModel) isMultiSelectStep() bool {
	return m.step == stepSkills || m.step == stepTools
}

func (m *onboardModel) toggleCurrent() {
	if m.cursor < 0 || m.cursor >= len(m.choices) {
		return
	}
	name := m.choices[m.cursor]
	switch m.step {
	case stepSkills:
		m.skillsSelected[name] = !m.skillsSelected[name]
	case stepTools:
		m.toolsSelected[name] = !m.toolsSelected[name]
	}
}

// selectedSkills returns the chosen skills in builtin order.
func (m onboardModel) selectedSkills() []string {
	selected := make([]string, 0, len(m.skills))
	for _, name := range m.skills {
		if m.skillsSelected[name] {
			selected = append(selected, name)
		}
	}
	return selected
}

// disabledTools returns the deselected tools in builtin order.
func (m onboardModel) disabledTools() []string {
	disabled := make([]string, 0, len(m.tools))
	for _, name := range m.tools {
		if !m.toolsSelected[name] {
			disabled = append(disabled, name)
		}
	}
	return disabled
}

// builtinSkillNames lists the skill templates embedded in the binary.
func builtinSkillNames() []string {
	entries, err := myaaw.DefaultMyaawDir.ReadDir(".myaaw/skills")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func builtinToolNames() []string {
	return tools.GetBuiltinToolNames()
}

// canonicalToolName maps the alias accepted by the runtime filter to the name
// used by the registry.
func canonicalToolName(name string) string {
	if name == "python" {
		return "execute_python"
	}
	return name
}

// existingDisabledTools reads tools/tools.json and returns the built-in tools
// that are currently disabled, in registry order.
func existingDisabledTools(agentName string) []string {
	dir, err := agentdir.ToolsDir(agentName)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "tools.json"))
	if err != nil {
		return nil
	}
	var cfg struct {
		BuiltinTools map[string]bool `json:"builtin_tools"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return nil
	}

	enabled := make(map[string]bool, len(cfg.BuiltinTools))
	for name, value := range cfg.BuiltinTools {
		enabled[canonicalToolName(name)] = value
	}

	disabled := make([]string, 0, len(cfg.BuiltinTools))
	for _, tool := range builtinToolNames() {
		if value, ok := enabled[tool]; ok && !value {
			disabled = append(disabled, tool)
		}
	}
	return disabled
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
		m.agentExistedAtStart = true
		m.step = stepInitHome

	case stepAgentName:
		name := strings.TrimSpace(m.textInput.Value())
		if err := validateAgentNameInput(name); err != nil {
			m.err = err
			return m, nil
		}
		m.setAgent(name)
		m.agentExistedAtStart = false
		m.step = stepInitHome

	case stepInitHome:
		if err := m.ensureNameStillFree(); err != nil {
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
		m.step = stepSkills
		m.cursor = 0
		m.choices = append([]string{}, m.skills...)
		if len(m.skills) == 0 {
			m.step = stepTools
			m.choices = append([]string{}, m.tools...)
		}
		if len(m.choices) == 0 {
			m.step = stepChannelChoice
			m.choices = []string{"Telegram", "Discord", "Both", "Skip"}
		}

	case stepSkills:
		m.step = stepTools
		m.cursor = 0
		m.choices = append([]string{}, m.tools...)
		if len(m.tools) == 0 {
			m.step = stepChannelChoice
			m.choices = []string{"Telegram", "Discord", "Both", "Skip"}
		}

	case stepTools:
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
		if m.telegramMode == "webhook" {
			m.step = stepTelegramWebhookURL
			m.resetInput("https://agent-b.example.com", false)
			m.textInput.SetValue(m.telegramWebhookURL)
			return m, nil
		}
		if m.channelChoice == "both" {
			m.step = stepDiscordToken
			m.resetInput("Discord Bot Token", true)
			return m, nil
		}
		m.setupHeartbeatChannel()

	case stepTelegramWebhookURL:
		publicURL := strings.TrimRight(strings.TrimSpace(m.textInput.Value()), "/")
		if err := validateWebhookURL(publicURL); err != nil {
			m.err = err
			return m, nil
		}
		m.telegramWebhookURL = publicURL
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
			m.step = m.afterInstallStep()
			return m, nil
		}
		if len(m.cmdOutput) > 0 && m.choices != nil && m.choices[0] == "Try again" {
			// A previous install attempt failed; retry or continue without it.
			if m.cursor == 1 {
				m.step = m.afterInstallStep()
				return m, nil
			}
		}
		if m.cursor == 0 {
			m.isRunning = true
			m.cmdOutput = []string{"Installing binary..."}
			return m, m.runGlobalPathCmd()
		}
		m.step = m.afterInstallStep()

	case stepRestart:
		if m.restartWarning != nil && m.cursor == 1 {
			m.step = stepDone
			return m, nil
		}
		if m.cursor == 0 {
			if err := restartAgentDaemon(m.agentName); err != nil {
				m.restartWarning = err
				m.choices = []string{"Retry restart", "Skip"}
				m.cursor = 0
				return m, nil
			}
			m.restartMsg = "Agent daemon restarted."
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

// ensureNameStillFree guards against another process creating the same agent
// while the wizard is open. Existing agents are allowed (reconfiguration).
func (m onboardModel) ensureNameStillFree() error {
	if m.agentExistedAtStart {
		return nil
	}
	taken, err := agentdir.AgentNameTaken(m.agentName)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("agent %q was created while you were onboarding; choose another name", m.agentName)
	}
	return nil
}

// initializeAgent creates the layout, registers the agent, assigns a port, and
// wires A2A. It is only called when the user saves the review step, so an
// interrupted wizard leaves nothing behind.
func initializeAgent(name string) (a2aWarning error, err error) {
	root, err := agentdir.AgentRoot(name)
	if err != nil {
		return nil, err
	}
	if err := setupMyaawHome(root); err != nil {
		return nil, fmt.Errorf("create agent layout: %w", err)
	}
	if err := agentdir.RewriteLegacyPaths(root, name); err != nil {
		return nil, fmt.Errorf("rewrite legacy paths: %w", err)
	}
	if err := agentdir.RegisterAgent(name); err != nil {
		return nil, fmt.Errorf("register agent: %w", err)
	}
	if err := agentdir.SetActive(name); err != nil {
		return nil, err
	}
	if err := ensureDefaultAgent(name); err != nil {
		return nil, fmt.Errorf("set default agent: %w", err)
	}
	if err := prepareAgentEnv(name, 0); err != nil {
		return nil, err
	}

	// Ensures the A2A token exists and every agent appears in the others'
	// contacts. Existing tokens are preserved, so re-running is safe.
	if contactErr := agentdir.EnsureA2AContacts(name); contactErr != nil {
		a2aWarning = contactErr
	}
	if _, err := agent.EnsureWorkspace(); err != nil {
		return nil, fmt.Errorf("prepare workspace: %w", err)
	}
	return a2aWarning, nil
}

// persist writes the whole agent: layout, .env, config.json, registry, and A2A
// wiring. Nothing is written before this runs, so cancelling the wizard never
// leaves partial data behind.
func (m *onboardModel) persist() error {
	if m.agentName == "" {
		return fmt.Errorf("agent name is missing")
	}
	if err := m.ensureNameStillFree(); err != nil {
		return err
	}

	a2aWarning, err := initializeAgent(m.agentName)
	if err != nil {
		return err
	}
	m.a2aWarning = a2aWarning

	_, warnings, err := applyOnboarding(m.agentName, m.input(), 0)
	if err != nil {
		return err
	}
	m.warnings = warnings

	if manager, err := daemon.NewAgentManager(m.agentName); err == nil {
		if _, running, _ := manager.Status(); running {
			m.restartPending = true
		}
	}
	return nil
}

// applyOnboarding merges the answers into config.json and returns the written
// config plus non-fatal warnings (for example a failed webhook registration).
func applyOnboarding(name string, in onboardInput, port int) (*config.Config, []string, error) {
	if err := prepareAgentEnv(name, port); err != nil {
		return nil, nil, err
	}
	root, err := agentdir.AgentRoot(name)
	if err != nil {
		return nil, nil, err
	}
	configPath := filepath.Join(root, "config", "config.json")

	cfg, err := loadConfigFromFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("read config.json: %w", err)
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	sanitizeOnboardConfig(cfg)

	merged := mergeOnboardConfig(cfg, in)
	if err := saveConfigToFile(configPath, merged); err != nil {
		return nil, nil, fmt.Errorf("write config.json: %w", err)
	}

	if err := installSelectedSkills(name, in.Skills); err != nil {
		return nil, nil, err
	}
	if err := saveBuiltinToolSelection(name, in.DisabledTools); err != nil {
		return nil, nil, err
	}

	var warnings []string
	if in.TelegramToken != "" && in.TelegramMode == "webhook" {
		publicURL := strings.TrimRight(strings.TrimSpace(in.TelegramWebhookURL), "/")
		if publicURL == "" {
			warnings = append(warnings, fmt.Sprintf("webhook mode is enabled but no public URL was set; run `myaaw --agent %s webhook set <domain>`", name))
		} else {
			if envPath, envErr := agentdir.EnvPath(name); envErr == nil {
				_ = agentdir.UpsertEnvValue(envPath, "MYAAW_BASE_URL", publicURL)
				// Contacts advertise MYAAW_BASE_URL, so refresh them after the change.
				if contactErr := agentdir.EnsureA2AContacts(name); contactErr != nil {
					warnings = append(warnings, "A2A contacts were not refreshed for the public URL: "+contactErr.Error())
				}
			}
			if regErr := registerTelegramWebhook(in.TelegramToken, publicURL); regErr != nil {
				warnings = append(warnings, "Telegram webhook was not registered: "+regErr.Error())
			}
		}
	}
	return merged, warnings, nil
}

func (m onboardModel) input() onboardInput {
	return onboardInput{
		ProviderName:       m.providerName,
		ProviderType:       m.providerType,
		ProviderBaseURL:    m.providerBaseURL,
		ProviderAPIKey:     m.llmKey,
		ProviderModel:      m.providerModel,
		TelegramToken:      m.telegramToken,
		TelegramMode:       m.telegramMode,
		TelegramWebhookURL: m.telegramWebhookURL,
		DiscordToken:       m.discordToken,
		Heartbeat:          m.heartbeat,
		HeartbeatChannel:   m.heartbeatChannel,
		HeartbeatTo:        m.heartbeatTo,
		HeartbeatEvery:     m.heartbeatEvery,
		OwnerIDs:           m.ownerIDs,
		Skills:             m.selectedSkills(),
		DisabledTools:      m.disabledTools(),
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
	ProviderName       string
	ProviderType       string
	ProviderBaseURL    string
	ProviderAPIKey     string
	ProviderModel      string
	TelegramToken      string
	TelegramMode       string
	TelegramWebhookURL string
	DiscordToken       string
	Heartbeat          bool
	HeartbeatChannel   string
	HeartbeatTo        string
	HeartbeatEvery     string
	OwnerIDs           []string
	Skills             []string
	DisabledTools      []string
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
	taken, err := agentdir.AgentNameTaken(name)
	if err != nil {
		return err
	}
	if taken {
		return fmt.Errorf("agent %q already exists; use --agent %s onboard to reconfigure it", name, name)
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

func validateWebhookURL(value string) error {
	if value == "" {
		return fmt.Errorf("a public https:// URL is required for webhook mode")
	}
	if !strings.HasPrefix(value, "https://") {
		return fmt.Errorf("Telegram requires an https:// URL, got %q", value)
	}
	if strings.HasSuffix(value, "/webhook/telegram") {
		return fmt.Errorf("enter only the base URL, without the /webhook/telegram path")
	}
	return nil
}

// afterInstallStep picks the next step once the install question is done.
func (m onboardModel) afterInstallStep() int {
	if m.restartPending {
		return stepRestart
	}
	return stepDone
}

// restartAgentDaemon restarts the agent server so it picks up the new config.
func restartAgentDaemon(name string) error {
	manager, err := daemon.NewManager("myaaw-server-" + name)
	if err != nil {
		return err
	}
	if err := manager.Stop(); err != nil {
		return fmt.Errorf("stop daemon: %w", err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := manager.Start([]string{"--agent", name, "server", "run"}); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	return nil
}

// previousStep implements one-step-back so Esc does not throw away answers.
func (m onboardModel) previousStep() onboardModel {
	m.err = nil
	switch m.step {
	case stepAgentChoice:
		m.step = stepWelcome

	case stepAgentName:
		if len(m.initializedAgents()) > 0 {
			m.step = stepAgentChoice
			m.cursor = 0
			m.choices = append(m.initializedAgents(), newAgentOption)
		} else {
			m.step = stepWelcome
		}

	case stepInitHome:
		if agentFlag != "" {
			m.step = stepWelcome
		} else {
			m.step = stepAgentName
			m.resetInput("myagent", false)
		}

	case stepProviderType:
		m.step = stepInitHome

	case stepProviderName:
		m.step = stepProviderType
		m.cursor = 0
		if m.providerType == "gemini" {
			m.cursor = 1
		}
		m.choices = []string{"OpenAI (or compatible)", "Gemini"}

	case stepProviderBaseURL:
		m.step = stepProviderName
		m.resetInput(m.providerType, false)
		m.textInput.SetValue(m.providerName)

	case stepProviderKey:
		m.step = stepProviderBaseURL
		m.resetInput(provider.DefaultBaseURL(m.providerType), false)
		m.textInput.SetValue(m.providerBaseURL)

	case stepProviderModel:
		m.step = stepProviderKey
		m.resetInput("API key", true)
		m.textInput.SetValue(m.llmKey)
		m.modelsLoading = false

	case stepTools:
		m.step = stepSkills
		m.cursor = 0
		m.choices = append([]string{}, m.skills...)

	case stepSkills:
		m.step = stepProviderModel
		if m.modelManual || len(m.models) == 0 {
			m.modelManual = true
			m.choices = nil
			m.resetInput("e.g. gpt-4o", false)
			m.textInput.SetValue(m.providerModel)
			break
		}
		m.choices = append(append([]string{}, m.models...), manualModelOption)
		m.cursor = 0
		for i, choice := range m.choices {
			if choice == m.providerModel {
				m.cursor = i
				break
			}
		}

	case stepChannelChoice:
		m.step = stepTools
		m.cursor = 0
		m.choices = append([]string{}, m.tools...)

	case stepTelegramMode:
		m.step = stepChannelChoice
		m.cursor = 0
		m.choices = []string{"Telegram", "Discord", "Both", "Skip"}

	case stepTelegramToken:
		m.step = stepTelegramMode
		m.cursor = 0
		if m.telegramMode == "webhook" {
			m.cursor = 1
		}
		m.choices = []string{"Polling (Recommended)", "Webhook"}

	case stepTelegramWebhookURL:
		m.step = stepTelegramToken
		m.resetInput("Telegram Bot Token", true)
		m.textInput.SetValue(m.telegramToken)

	case stepDiscordToken:
		m.step = stepChannelChoice
		m.cursor = 0
		m.choices = []string{"Telegram", "Discord", "Both", "Skip"}

	case stepHeartbeatChannel:
		m.step = stepChannelChoice
		m.cursor = 0
		m.choices = []string{"Telegram", "Discord", "Both", "Skip"}

	case stepHeartbeatID:
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

	case stepHeartbeatEvery:
		m.step = stepHeartbeatID
		m.resetInput(heartbeatPlaceholder(m.heartbeatChannel), false)
		m.textInput.SetValue(m.heartbeatTo)

	case stepOwner:
		if m.heartbeat {
			m.step = stepHeartbeatEvery
			m.resetInput(defaultHeartbeatEV, false)
			m.textInput.SetValue(m.heartbeatEvery)
			break
		}
		m.step = stepHeartbeatChannel
		m.cursor = 0

	case stepSummary:
		m.step = stepOwner
		m.resetInput("123456789, 987654321 (optional, comma separated)", false)
		m.textInput.SetValue(strings.Join(m.ownerIDs, ", "))

	case stepInstallGlobal:
		m.step = stepSummary
		m.cursor = 0
		m.choices = []string{"Save configuration", "Cancel"}

	case stepRestart:
		m.step = stepSummary
		m.cursor = 0
		m.choices = []string{"Save configuration", "Cancel"}

	default:
		m.step = stepWelcome
	}
	return m
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
		b.WriteString(titleStyle.Render("Agent Directory"))
		b.WriteString(fmt.Sprintf("\n\nAgent: %s\nDirectory: %s\n\n", m.agentName, m.myaawHome))
		b.WriteString(theme.SecondaryStyle.Render("Nothing is written yet; the agent is created when you save the review."))

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

	case stepSkills:
		b.WriteString(titleStyle.Render("Skills"))
		if m.agentExistedAtStart {
			b.WriteString("\n\nInstalled skills are pre-selected. Unchecking does not delete an existing folder.\n")
		} else {
			b.WriteString("\n\nNo skill is selected by default. Space to toggle, Enter to continue.\n")
		}
		b.WriteString(theme.SecondaryStyle.Render("Only the skills you select get a folder under skills/<name>/.") + "\n\n")
		b.WriteString(m.renderMultiSelect(m.skillsSelected))

	case stepTools:
		b.WriteString(titleStyle.Render("Built-in Tools"))
		b.WriteString("\n\nEvery built-in tool is enabled by default. Space to disable, Enter to continue.\n")
		b.WriteString(theme.SecondaryStyle.Render("Disabled tools are written to tools/tools.json as false.") + "\n\n")
		b.WriteString(m.renderMultiSelect(m.toolsSelected))

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

	case stepTelegramWebhookURL:
		b.WriteString(titleStyle.Render("Public Webhook URL"))
		b.WriteString("\n\nTelegram will POST updates to <url>/webhook/telegram.\n")
		b.WriteString(theme.SecondaryStyle.Render("Must be public HTTPS with a valid certificate. An ngrok URL works.") + "\n\n")
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
		b.WriteString(m.renderWarnings())
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

	case stepRestart:
		b.WriteString(titleStyle.Render("Restart Server"))
		b.WriteString("\n\nThe daemon for this agent is running. Restart it to load the new config?\n\n")
		if m.restartWarning != nil {
			b.WriteString(theme.ErrorStyle.Render("⚠  "+m.restartWarning.Error()) + "\n\n")
		}
		b.WriteString(m.renderChoices())

	case stepDone:
		b.WriteString(theme.SuccessStyle.Render("✨ Onboarding Complete!"))
		b.WriteString(fmt.Sprintf("\n\nAgent: %s\nConfig: %s\n\n", m.agentName, m.configFile))
		if m.restartMsg != "" {
			b.WriteString(theme.SuccessStyle.Render("↻ "+m.restartMsg) + "\n\n")
		}
		b.WriteString(m.renderWarnings())
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
	skills := "none"
	if selected := m.selectedSkills(); len(selected) > 0 {
		skills = strings.Join(selected, ", ")
	}
	row("Skills", skills)
	disabled := m.disabledTools()
	row("Tools", fmt.Sprintf("%d enabled, %d disabled", len(m.tools)-len(disabled), len(disabled)))
	if len(disabled) > 0 {
		row("Disabled", strings.Join(disabled, ", "))
	}
	row("Telegram", m.channelSummary("telegram", m.telegramToken, m.telegramMode))
	if m.telegramMode == "webhook" && m.telegramWebhookURL != "" {
		row("Webhook", m.telegramWebhookURL+"/webhook/telegram")
	}
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

func (m onboardModel) renderWarnings() string {
	if len(m.warnings) == 0 && m.a2aWarning == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, warning := range m.warnings {
		b.WriteString(theme.WarningStyle.Render("⚠  "+warning) + "\n")
	}
	if m.a2aWarning != nil {
		b.WriteString(theme.WarningStyle.Render("⚠  A2A contacts were not seeded: "+m.a2aWarning.Error()) + "\n")
	}
	b.WriteString("\n")
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

func (m onboardModel) renderMultiSelect(selected map[string]bool) string {
	var s strings.Builder
	for i, choice := range m.choices {
		cursor := "  "
		style := theme.BaseStyle
		if m.cursor == i {
			cursor = "👉"
			style = theme.HighlightStyle
		}
		box := "[ ]"
		if selected[choice] {
			box = "[x]"
		}
		s.WriteString(fmt.Sprintf("%s %s %s\n", cursor, box, style.Render(choice)))
	}
	return s.String()
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
		"Skills & Tools",
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
	case stepSkills, stepTools:
		timelineIdx = 4
	case stepChannelChoice, stepTelegramMode, stepTelegramToken, stepTelegramWebhookURL, stepDiscordToken:
		timelineIdx = 5
	case stepHeartbeatChannel, stepHeartbeatID, stepHeartbeatEvery:
		timelineIdx = 6
	case stepOwner:
		timelineIdx = 7
	case stepSummary:
		timelineIdx = 8
	case stepInstallGlobal, stepRestart:
		timelineIdx = 9
	case stepDone:
		timelineIdx = 10
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

// --- Headless mode ---

type onboardSummary struct {
	Agent         string            `json:"agent"`
	Config        string            `json:"config"`
	Env           string            `json:"env"`
	Port          string            `json:"port"`
	Provider      string            `json:"provider"`
	ProviderType  string            `json:"provider_type"`
	BaseURL       string            `json:"base_url,omitempty"`
	Model         string            `json:"model"`
	Channels      map[string]string `json:"channels,omitempty"`
	Heartbeat     string            `json:"heartbeat,omitempty"`
	Owners        []string          `json:"owners,omitempty"`
	Skills        []string          `json:"skills,omitempty"`
	DisabledTools []string          `json:"disabled_tools,omitempty"`
	Webhook       string            `json:"webhook,omitempty"`
	Installed     string            `json:"installed,omitempty"`
	Warnings      []string          `json:"warnings,omitempty"`
}

// runOnboardHeadless applies the same configuration as the wizard, driven by
// flags and environment variables. It returns a process exit code.
func runOnboardHeadless(agentName string) int {
	if agentName == "" {
		return headlessError("--agent <name> is required when running non-interactively")
	}
	if err := agentdir.ValidateName(agentName); err != nil {
		return headlessError(err.Error())
	}
	taken, err := agentdir.AgentNameTaken(agentName)
	if err != nil {
		return headlessError(err.Error())
	}
	if taken && !onboardOpts.force {
		return headlessError(fmt.Sprintf("agent %q already exists; pass --force to reconfigure it", agentName))
	}

	in, err := headlessInput(agentName, onboardOpts)
	if err != nil {
		return headlessError(err.Error())
	}
	a2aWarning, err := initializeAgent(agentName)
	if err != nil {
		return headlessError(err.Error())
	}
	cfg, warnings, err := applyOnboarding(agentName, in, onboardOpts.port)
	if err != nil {
		return headlessError(err.Error())
	}
	if a2aWarning != nil {
		warnings = append(warnings, "A2A contacts were not seeded: "+a2aWarning.Error())
	}

	installed := ""
	if onboardOpts.install {
		path, installErr := installBinary()
		if installErr != nil {
			warnings = append(warnings, "binary was not installed: "+installErr.Error())
		} else {
			installed = path
			if !pathContains(filepath.Dir(path)) {
				warnings = append(warnings, filepath.Dir(path)+" is not in PATH")
			}
		}
	}

	if onboardOpts.restart {
		if manager, err := daemon.NewAgentManager(agentName); err == nil {
			if _, running, _ := manager.Status(); running {
				if err := restartAgentDaemon(agentName); err != nil {
					warnings = append(warnings, "daemon was not restarted: "+err.Error())
				}
			}
		}
	}

	summary := buildOnboardSummary(agentName, cfg, in, warnings, installed)
	if onboardOpts.json {
		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return headlessError(err.Error())
		}
		fmt.Println(string(data))
		return 0
	}

	fmt.Printf("Agent:    %s\n", summary.Agent)
	fmt.Printf("Config:   %s\n", summary.Config)
	fmt.Printf("Env:      %s\n", summary.Env)
	fmt.Printf("Port:     %s\n", summary.Port)
	fmt.Printf("Provider: %s (%s)\n", summary.Provider, summary.ProviderType)
	if summary.BaseURL != "" {
		fmt.Printf("Base URL: %s\n", summary.BaseURL)
	}
	fmt.Printf("Model:    %s\n", summary.Model)
	if len(summary.Skills) > 0 {
		fmt.Printf("Skills:   %s\n", strings.Join(summary.Skills, ", "))
	} else {
		fmt.Printf("Skills:   none\n")
	}
	toolCount := len(builtinToolNames())
	fmt.Printf("Tools:    %d enabled, %d disabled\n", toolCount-len(summary.DisabledTools), len(summary.DisabledTools))
	if len(summary.DisabledTools) > 0 {
		fmt.Printf("Disabled: %s\n", strings.Join(summary.DisabledTools, ", "))
	}
	for _, name := range []string{"telegram", "discord"} {
		state := summary.Channels[name]
		if state == "" {
			state = "not configured"
		}
		fmt.Printf("%-9s %s\n", strings.Title(name)+":", state)
	}
	if summary.Webhook != "" {
		fmt.Printf("Webhook:  %s\n", summary.Webhook)
	}
	if summary.Heartbeat != "" {
		fmt.Printf("Heartbeat: %s\n", summary.Heartbeat)
	}
	if len(summary.Owners) > 0 {
		fmt.Printf("Owners:   %s\n", strings.Join(summary.Owners, ", "))
	}
	if summary.Installed != "" {
		fmt.Printf("Installed: %s\n", summary.Installed)
	}
	for _, warning := range summary.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}
	return 0
}

func buildOnboardSummary(agentName string, cfg *config.Config, in onboardInput, warnings []string, installed string) onboardSummary {
	root, _ := agentdir.AgentRoot(agentName)
	envPath, _ := agentdir.EnvPath(agentName)

	summary := onboardSummary{
		Agent:         agentName,
		Config:        filepath.Join(root, "config", "config.json"),
		Env:           envPath,
		Port:          readEnvValue(envPath, "PORT"),
		Channels:      map[string]string{},
		Skills:        in.Skills,
		DisabledTools: in.DisabledTools,
		Warnings:      warnings,
		Installed:     installed,
	}
	if cfg == nil {
		return summary
	}

	summary.Provider = cfg.DefaultProvider
	if p, ok := cfg.Providers[cfg.DefaultProvider]; ok {
		summary.ProviderType = p.Type
		summary.BaseURL = p.BaseURL
		summary.Model = p.DefaultModel
	}
	if cfg.Channels.Telegram != nil && cfg.Channels.Telegram.Active {
		summary.Channels["telegram"] = cfg.Channels.Telegram.Mode
	}
	if cfg.Channels.Discord != nil && cfg.Channels.Discord.Active {
		summary.Channels["discord"] = "enabled"
	}
	if cfg.Heartbeat.Active {
		summary.Heartbeat = fmt.Sprintf("%s %s every %s", cfg.Heartbeat.Channel, cfg.Heartbeat.To, cfg.Heartbeat.Every)
	}
	summary.Owners = cfg.Bot.OwnerIDs
	if summary.Channels["telegram"] == "webhook" {
		if base := readEnvValue(envPath, "MYAAW_BASE_URL"); strings.HasPrefix(base, "https://") {
			summary.Webhook = base + "/webhook/telegram"
		}
	}
	return summary
}

func headlessError(message string) int {
	fmt.Fprintf(os.Stderr, "Error: %s\n", message)
	return 1
}

// headlessInput validates flags and environment variables, falling back to the
// agent's existing values where that is unambiguous.
func headlessInput(agentName string, opts onboardOptions) (onboardInput, error) {
	var in onboardInput
	existing := loadAgentConfig(agentName)

	providerType := strings.ToLower(strings.TrimSpace(opts.provider))
	if providerType == "" {
		return in, fmt.Errorf("--provider is required (openai or gemini)")
	}
	if providerType != "openai" && providerType != "gemini" {
		return in, fmt.Errorf("--provider must be openai or gemini")
	}

	providerName := strings.TrimSpace(opts.provName)
	if providerName == "" {
		providerName = providerType
	}
	if strings.ContainsAny(providerName, " \t") {
		return in, fmt.Errorf("--provider-name cannot contain spaces")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(opts.baseURL), "/")
	if baseURL == "" {
		baseURL = provider.DefaultBaseURL(providerType)
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return in, fmt.Errorf("--base-url must start with http:// or https://")
	}

	apiKey := firstNonEmpty(opts.apiKey, os.Getenv("MYAAW_API_KEY"))
	if apiKey == "" && existing != nil {
		apiKey = existing.Providers[providerName].APIKey
	}
	if apiKey == "" {
		return in, fmt.Errorf("--api-key or MYAAW_API_KEY is required")
	}

	model := strings.TrimSpace(opts.model)
	if model == "" && existing != nil {
		model = existing.Providers[providerName].DefaultModel
	}
	if model == "" {
		model = provider.DefaultModel(providerType)
	}
	if model == "" {
		return in, fmt.Errorf("--model is required")
	}

	in.ProviderName = providerName
	in.ProviderType = providerType
	in.ProviderBaseURL = baseURL
	in.ProviderAPIKey = apiKey
	in.ProviderModel = model

	channel := strings.ToLower(strings.TrimSpace(opts.channel))
	if channel == "" {
		channel = "skip"
	}
	switch channel {
	case "telegram", "discord", "both", "skip":
	default:
		return in, fmt.Errorf("--channel must be telegram, discord, both, or skip")
	}

	telegramMode := strings.ToLower(strings.TrimSpace(opts.tgMode))
	if telegramMode == "" {
		telegramMode = "polling"
	}
	if telegramMode != "polling" && telegramMode != "webhook" {
		return in, fmt.Errorf("--telegram-mode must be polling or webhook")
	}
	in.TelegramMode = telegramMode

	tgToken := firstNonEmpty(opts.tgToken, os.Getenv("TELEGRAM_BOT_TOKEN"))
	if tgToken == "" && existing != nil && existing.Channels.Telegram != nil {
		tgToken = existing.Channels.Telegram.Token
	}
	if channel == "telegram" || channel == "both" {
		if err := validateTelegramToken(tgToken); err != nil {
			return in, fmt.Errorf("%v (use --telegram-token or TELEGRAM_BOT_TOKEN)", err)
		}
		in.TelegramToken = tgToken
	}

	webhookURL := strings.TrimRight(strings.TrimSpace(opts.webhook), "/")
	if webhookURL == "" {
		webhookURL = strings.TrimRight(strings.TrimSpace(os.Getenv("MYAAW_WEBHOOK_URL")), "/")
	}
	if in.TelegramToken != "" && telegramMode == "webhook" {
		if err := validateWebhookURL(webhookURL); err != nil {
			return in, fmt.Errorf("%v (use --webhook-url)", err)
		}
	}
	in.TelegramWebhookURL = webhookURL

	dcToken := firstNonEmpty(opts.dcToken, os.Getenv("DISCORD_BOT_TOKEN"))
	if dcToken == "" && existing != nil && existing.Channels.Discord != nil {
		dcToken = existing.Channels.Discord.Token
	}
	if channel == "discord" || channel == "both" {
		if dcToken == "" {
			return in, fmt.Errorf("--discord-token or DISCORD_BOT_TOKEN is required for %s", channel)
		}
		in.DiscordToken = dcToken
	}

	hbChannel := strings.ToLower(strings.TrimSpace(opts.hbChannel))
	switch hbChannel {
	case "":
		// Leave heartbeat untouched.
	case "skip":
		// Nothing to write; keep existing heartbeat as is.
	case "telegram":
		if tgToken == "" {
			return in, fmt.Errorf("--heartbeat-channel telegram needs a telegram token")
		}
	case "discord":
		if dcToken == "" {
			return in, fmt.Errorf("--heartbeat-channel discord needs a discord token")
		}
	default:
		return in, fmt.Errorf("--heartbeat-channel must be telegram, discord, or skip")
	}

	if hbChannel == "telegram" || hbChannel == "discord" {
		if err := validateChatID(strings.TrimSpace(opts.hbTo)); err != nil {
			return in, fmt.Errorf("--heartbeat-to: %v", err)
		}
		every := strings.TrimSpace(opts.hbEvery)
		if every == "" {
			every = defaultHeartbeatEV
		}
		if d, err := time.ParseDuration(every); err != nil || d <= 0 {
			return in, fmt.Errorf("--heartbeat-every must be a positive duration such as 30m or 1h")
		}
		in.Heartbeat = true
		in.HeartbeatChannel = hbChannel
		in.HeartbeatTo = strings.TrimSpace(opts.hbTo)
		in.HeartbeatEvery = every
	}

	owners, err := parseOwnerIDs(opts.owner)
	if err != nil {
		return in, fmt.Errorf("--owner: %v", err)
	}
	in.OwnerIDs = owners

	skills, err := parseOnboardSelection(opts.skills, builtinSkillNames(), "--skills")
	if err != nil {
		return in, err
	}
	in.Skills = skills

	disabledTools, err := parseOnboardSelection(opts.noTools, builtinToolNames(), "--disable-tools")
	if err != nil {
		return in, err
	}
	switch {
	case opts.disableToolsSet || strings.TrimSpace(opts.noTools) != "":
		in.DisabledTools = disabledTools
	case existing != nil:
		// Updating without --disable-tools keeps the tools that are already off.
		in.DisabledTools = existingDisabledTools(agentName)
	}

	if opts.port < 0 || opts.port > 65535 {
		return in, fmt.Errorf("--port must be between 1 and 65535")
	}
	return in, nil
}

// parseOnboardSelection validates a comma-separated list (or "all") against
// the known built-in names and returns them in the canonical order.
func parseOnboardSelection(raw string, known []string, flag string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.EqualFold(raw, "all") {
		return append([]string{}, known...), nil
	}

	valid := make(map[string]bool, len(known))
	for _, name := range known {
		valid[name] = true
	}

	chosen := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if !valid[name] {
			return nil, fmt.Errorf("%s: unknown name %q (valid: %s)", flag, name, strings.Join(known, ", "))
		}
		chosen[name] = true
	}

	selected := make([]string, 0, len(chosen))
	for _, name := range known {
		if chosen[name] {
			selected = append(selected, name)
		}
	}
	return selected, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func loadAgentConfig(name string) *config.Config {
	root, err := agentdir.AgentRoot(name)
	if err != nil {
		return nil
	}
	cfg, err := loadConfigFromFile(filepath.Join(root, "config", "config.json"))
	if err != nil {
		return nil
	}
	return cfg
}

// --- Filesystem helpers ---

// installSelectedSkills copies only the chosen skill templates. Existing
// folders are never overwritten, and unselected skills are not created.
func installSelectedSkills(name string, selected []string) error {
	skillsDir, err := agentdir.SkillsDir(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}

	known := make(map[string]bool)
	for _, skill := range builtinSkillNames() {
		known[skill] = true
	}

	for _, skill := range selected {
		if !known[skill] {
			return fmt.Errorf("unknown skill %q", skill)
		}
		dest := filepath.Join(skillsDir, skill)
		if _, statErr := os.Stat(dest); statErr == nil {
			continue
		}
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		src := path.Join(".myaaw", "skills", skill)
		if err := extractEmbedDir(myaaw.DefaultMyaawDir, src, dest); err != nil {
			return fmt.Errorf("install skill %q: %w", skill, err)
		}
	}
	return nil
}

// saveBuiltinToolSelection writes builtin_tools to tools/tools.json while
// preserving custom_tools and mcp_servers. Every built-in is listed, so a
// reconfigure re-enables tools that are no longer disabled.
func saveBuiltinToolSelection(name string, disabled []string) error {
	names := builtinToolNames()
	known := make(map[string]bool, len(names))
	builtin := make(map[string]bool, len(names))
	for _, tool := range names {
		known[tool] = true
		builtin[tool] = true
	}
	for _, tool := range disabled {
		if !known[tool] {
			return fmt.Errorf("unknown built-in tool %q", tool)
		}
		builtin[tool] = false
	}

	toolsDir, err := agentdir.ToolsDir(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return err
	}
	configPath := filepath.Join(toolsDir, "tools.json")

	raw := map[string]any{}
	if data, readErr := os.ReadFile(configPath); readErr == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("read tools.json: %w", err)
		}
	}
	raw["builtin_tools"] = builtin

	data, err := json.MarshalIndent(raw, "", "    ")
	if err != nil {
		return err
	}
	return writeFileAtomic(configPath, append(data, '\n'))
}

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
	// Skills are opt-in: the template copy skips them and onboarding installs
	// only the selected ones.
	if err := extractEmbedDirExcept(myaaw.DefaultMyaawDir, ".myaaw", targetDir, "skills"); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(targetDir, "skills"), 0o755)
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

// saveConfigToFile writes config.json through a temp file so a crash mid-write
// cannot leave a truncated config behind.
func saveConfigToFile(path string, cfg *config.Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

// writeFileAtomic writes data through a temp file in the same directory.
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
		// Windows cannot reliably replace an existing file with Rename.
		_ = os.Remove(path)
		if retryErr := os.Rename(tmpPath, path); retryErr != nil {
			os.Remove(tmpPath)
			return retryErr
		}
	}
	return nil
}

func extractEmbedDir(embeddedFS embed.FS, src string, dest string) error {
	return extractEmbedDirExcept(embeddedFS, src, dest)
}

// extractEmbedDirExcept copies an embedded directory while skipping the given
// top-level entries.
func extractEmbedDirExcept(embeddedFS embed.FS, src string, dest string, skip ...string) error {
	skipSet := make(map[string]bool, len(skip))
	for _, name := range skip {
		skipSet[name] = true
	}

	entries, err := embeddedFS.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() && skipSet[entry.Name()] {
			continue
		}
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
