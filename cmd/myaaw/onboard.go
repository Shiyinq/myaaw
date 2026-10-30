package main

import (
	"encoding/json"
	"fmt"
	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/daemon"
	"myaaw/internal/provider"
	"os"
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
	yes             bool
	json            bool
	force           bool
	install         bool
	provider        string
	provName        string
	baseURL         string
	apiKey          string
	model           string
	channel         string
	tgMode          string
	tgToken         string
	webhook         string
	dcToken         string
	hbChannel       string
	hbTo            string
	hbEvery         string
	owner           string
	tgOwner         string
	dcOwner         string
	port            int
	restart         bool
	skills          string
	noTools         string
	keepProvider    bool
	botType         string
	disableChannels string
	listCaps        bool
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
	f.StringVar(&onboardOpts.hbChannel, "heartbeat-channel", "", "heartbeat channel: telegram, discord, off, or skip")
	f.StringVar(&onboardOpts.hbTo, "heartbeat-to", "", "heartbeat destination chat/channel ID")
	f.StringVar(&onboardOpts.hbEvery, "heartbeat-every", "", "heartbeat interval, e.g. 30m or 1h")
	f.StringVar(&onboardOpts.owner, "owner", "", "owner IDs for any channel, comma separated")
	f.StringVar(&onboardOpts.tgOwner, "telegram-owner", "", "Telegram owner user IDs, comma separated (optional)")
	f.StringVar(&onboardOpts.dcOwner, "discord-owner", "", "Discord owner user IDs, comma separated (optional)")
	f.IntVar(&onboardOpts.port, "port", 0, "agent server port (default: first free port)")
	f.BoolVar(&onboardOpts.restart, "restart", false, "restart the agent daemon after writing the config")
	f.StringVar(&onboardOpts.skills, "skills", "", "skills to install, comma separated (default: none, or 'all')")
	f.StringVar(&onboardOpts.noTools, "disable-tools", "", "built-in tools to disable, comma separated")
	f.BoolVar(&onboardOpts.keepProvider, "keep-provider", false, "keep the current provider and model (update only)")
	f.StringVar(&onboardOpts.botType, "bot-type", "", "access type: private or public (default: private)")
	f.BoolVar(&onboardOpts.listCaps, "list-capabilities", false, "list built-in skills and tools, then exit")
	f.StringVar(&onboardOpts.disableChannels, "disable-channel", "", "channels to turn off: telegram, discord, or both")
}

func runOnboard(cmd *cobra.Command, args []string) {
	onboardOpts.disableToolsSet = cmd.Flags().Changed("disable-tools")
	runOnboarding(strings.TrimSpace(agentFlag), false)
}

// runOnboarding is the single implementation behind `myaaw onboard` and
// `myaaw agent create`. When createNow is true the agent layout is created
// before the TUI starts and the wizard jumps straight to the provider step.
func runOnboarding(agentName string, createNow bool) {
	if onboardOpts.listCaps {
		printCapabilities(onboardOpts.json)
		return
	}
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
	stepChannelExisting
	stepBotType
	stepTelegramMode
	stepTelegramToken
	stepTelegramWebhookURL
	stepDiscordToken
	stepHeartbeatChannel
	stepHeartbeatID
	stepHeartbeatEvery
	stepOwnerTelegram
	stepOwnerDiscord
	stepSummary
	stepInstallGlobal
	stepRestart
	stepDone
)

const (
	newAgentOption     = "+ Create a new agent"
	manualModelOption  = "Enter model manually"
	defaultHeartbeatEV = "30m"
	defaultBotType     = "private"

	// Update mode: every section offers an explicit "keep" answer, worded the
	// same way. "Skip" is reserved for a brand new agent, where there is no
	// previous value to keep.
	keepProviderOption  = "Keep current provider"
	keepSkillsOption    = "Keep current skills"
	keepToolsOption     = "Keep current tools"
	keepChannelsOption  = "Keep current"
	keepHeartbeatOption = "Keep current heartbeat"
	keepAccessOption    = "Keep current access"
	skipOption          = "Skip"

	heartbeatOffOption = "Disable heartbeat"
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
	agentFromPicker     bool
	existingTelegram    bool
	existingDiscord     bool
	disableTelegram     bool
	disableDiscord      bool
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

	heartbeat                bool
	heartbeatSet             bool
	heartbeatChannel         string
	heartbeatTo              string
	heartbeatEvery           string
	existingHeartbeat        bool
	existingHeartbeatChannel string
	existingHeartbeatTo      string
	existingHeartbeatEvery   string
	botType                  string

	telegramOwnerIDs []string
	discordOwnerIDs  []string
	ownersAsked      bool
	firstAgent       bool

	skipProvider bool
	skipSkills   bool
	skipTools    bool

	skills            []string
	skillsSelected    map[string]bool
	skillDescriptions map[string]string
	tools             []string
	toolsSelected     map[string]bool
	toolDescriptions  map[string]string

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
		step:              stepWelcome,
		textInput:         ti,
		homeDir:           home,
		choices:           []string{"Yes", "No"},
		outputChan:        make(chan tea.Msg, 10),
		skills:            builtinSkillNames(),
		skillsSelected:    map[string]bool{},
		skillDescriptions: builtinSkillDescriptions(),
		tools:             builtinToolNames(),
		toolsSelected:     map[string]bool{},
		toolDescriptions:  builtinToolDescriptions(),
		botType:           defaultBotType,
	}
	for _, name := range m.skills {
		m.skillsSelected[name] = false
	}
	for _, name := range m.tools {
		m.toolsSelected[name] = true
	}
	// The global install only makes sense on the very first agent; later agents
	// reuse the binary that is already on PATH.
	if agents, err := agentdir.ListAgents(); err == nil {
		m.firstAgent = len(agents) == 0
	} else {
		m.firstAgent = false
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
// current state instead of resetting it. It starts from the defaults so picking
// another agent later in the wizard cannot leak the previous answers.
func (m *onboardModel) loadExistingCapabilities() {
	for _, name := range m.skills {
		m.skillsSelected[name] = false
	}
	for _, name := range m.tools {
		m.toolsSelected[name] = true
	}
	m.telegramOwnerIDs = nil
	m.discordOwnerIDs = nil
	m.existingTelegram = false
	m.existingDiscord = false
	m.disableTelegram = false
	m.disableDiscord = false
	m.existingHeartbeat = false
	m.existingHeartbeatChannel = ""
	m.existingHeartbeatTo = ""
	m.existingHeartbeatEvery = ""
	m.heartbeat = false
	m.heartbeatSet = false
	m.heartbeatChannel = ""
	m.heartbeatTo = ""
	m.heartbeatEvery = ""
	m.botType = defaultBotType

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

	cfg := loadAgentConfig(m.agentName)
	if cfg == nil {
		return
	}
	// Remember which channels are already wired so the owner and heartbeat
	// steps still apply when the user skips the channel section.
	m.existingTelegram = channelInUse(cfg.Channels.Telegram)
	m.existingDiscord = channelInUse(cfg.Channels.Discord)

	// Pre-fill the owner prompts with the current IDs so pressing Enter keeps
	// them instead of replacing the list with whoever is asked first.
	if len(cfg.Bot.OwnerIDs) > 0 {
		m.telegramOwnerIDs = append([]string{}, cfg.Bot.OwnerIDs...)
		m.discordOwnerIDs = append([]string{}, cfg.Bot.OwnerIDs...)
	}
	// Only an explicit "private" locks the channels down; an empty or unknown
	// type behaves like public, so show that instead of silently switching the
	// agent to private on the next save.
	if strings.EqualFold(strings.TrimSpace(cfg.Bot.Type), "private") {
		m.botType = "private"
	} else {
		m.botType = "public"
	}

	// Remember the heartbeat so the review can say "unchanged" and the prompts
	// start from the current destination and interval.
	if cfg.Heartbeat.Active {
		m.existingHeartbeat = true
		m.existingHeartbeatChannel = cfg.Heartbeat.Channel
		m.existingHeartbeatTo = cfg.Heartbeat.To
		m.existingHeartbeatEvery = cfg.Heartbeat.Every
	}
}

// channelInUse reports whether a saved channel is enabled or has a token.
func channelInUse(channel *config.ChannelConfig) bool {
	if channel == nil {
		return false
	}
	return channel.Active || strings.TrimSpace(channel.Token) != ""
}

// channelConfigured reports whether a channel is in use, either because the
// user answered the channel steps in this run or because it was already saved.
func (m onboardModel) channelConfigured(channel string) bool {
	switch channel {
	case "telegram":
		return m.telegramToken != "" || (m.existingTelegram && !m.disableTelegram)
	case "discord":
		return m.discordToken != "" || (m.existingDiscord && !m.disableDiscord)
	}
	return false
}

// channelDisabled reports whether the user asked to turn a saved channel off.
func (m onboardModel) channelDisabled(channel string) bool {
	switch channel {
	case "telegram":
		return m.disableTelegram
	case "discord":
		return m.disableDiscord
	}
	return false
}

// preferredProviderName returns the name to suggest when the user re-enters the
// provider flow.
func (m onboardModel) preferredProviderName(providerType string) string {
	return preferredProviderNameFromConfig(loadAgentConfig(m.agentName), providerType)
}

// preferredProviderNameFromConfig picks the saved provider that should be edited
// for a type. It reads the type field of every entry instead of trusting the
// key, so a provider stored as "deepseek" with type "openai" is updated rather
// than duplicated as a new "openai" entry. The default provider wins when its
// type matches, otherwise the first matching name in sorted order.
func preferredProviderNameFromConfig(cfg *config.Config, providerType string) string {
	if cfg == nil {
		return providerType
	}
	if cfg.DefaultProvider != "" {
		if saved, ok := cfg.Providers[cfg.DefaultProvider]; ok && saved.Type == providerType {
			return cfg.DefaultProvider
		}
	}
	names := make([]string, 0, len(cfg.Providers))
	for name := range cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if cfg.Providers[name].Type == providerType {
			return name
		}
	}
	return providerType
}

// existingAPIKey returns the saved key for a provider, ignoring placeholders
// left over from the old template.
func (m onboardModel) existingAPIKey(name string) string {
	saved, ok := m.existingProvider(name)
	if !ok || isTemplateValue(saved.APIKey) {
		return ""
	}
	return saved.APIKey
}

// existingProvider returns the saved definition for a provider name, if any.
func (m onboardModel) existingProvider(name string) (config.ProviderConfig, bool) {
	if name == "" {
		return config.ProviderConfig{}, false
	}
	cfg := loadAgentConfig(m.agentName)
	if cfg == nil {
		return config.ProviderConfig{}, false
	}
	saved, ok := cfg.Providers[name]
	return saved, ok
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
		default:
			// Text inputs must receive every key, including j, k, and space; only
			// single-choice and multi-select steps use those for navigation.
			if m.isTextInputStep() {
				m.textInput, cmd = m.textInput.Update(msg)
				break
			}
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.choices)-1 {
					m.cursor++
				}
			case " ":
				if m.isMultiSelectStep() {
					m.toggleCurrent()
				}
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
	body := mainStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, timeline, content))
	helpStyle := lipgloss.NewStyle().Margin(0, 1, 1, 2)
	return lipgloss.JoinVertical(lipgloss.Left, body, helpStyle.Render(m.helpLine()))
}

// helpLine keeps the key hints visible on every screen, so going back with Esc
// is discoverable instead of being a hidden shortcut.
func (m onboardModel) helpLine() string {
	parts := make([]string, 0, 4)
	switch {
	case m.step == stepDone:
		parts = append(parts, "Enter · finish")
	case m.step == stepWelcome:
		parts = append(parts, "Enter · start", "Esc · cancel")
	case m.isTextInputStep():
		parts = append(parts, "Enter · next", "Esc · back")
	case m.isMultiSelectStep():
		parts = append(parts, "Space · toggle", "Enter · next", "Esc · back")
	default:
		parts = append(parts, "↑/↓ · move", "Enter · next", "Esc · back")
	}
	if m.step != stepDone && m.step != stepWelcome {
		parts = append(parts, "Ctrl+C · cancel")
	}
	return theme.MutedStyle.Render(strings.Join(parts, "   "))
}

// --- Logic ---

func (m onboardModel) isTextInputStep() bool {
	switch m.step {
	case stepAgentName, stepProviderName, stepProviderBaseURL, stepProviderKey,
		stepTelegramToken, stepTelegramWebhookURL, stepDiscordToken, stepHeartbeatID, stepHeartbeatEvery,
		stepOwnerTelegram, stepOwnerDiscord:
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
	if name == keepSkillsOption || name == keepToolsOption {
		return
	}
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
// builtinSkillDescriptions reads the description from each bundled SKILL.md.
func builtinSkillDescriptions() map[string]string {
	return onboardSkillDescriptions()
}

func builtinToolDescriptions() map[string]string {
	return onboardToolDescriptions()
}

// truncateOneLine shortens a description so it fits next to a skill or tool
// name without wrapping the whole list.
func truncateOneLine(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if limit <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

// capabilityEntry describes one built-in skill or tool. It backs both the
// --list-capabilities output and the --json variant.
type capabilityEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type capabilitiesSummary struct {
	Skills []capabilityEntry `json:"skills"`
	Tools  []capabilityEntry `json:"tools"`
}

// builtinCapabilities lists the embedded skills and the built-in tools with
// their descriptions, in registry order. The catalog itself lives in the
// shared onboard core so the control-plane API serves the same list.
func builtinCapabilities() capabilitiesSummary {
	caps := onboardBuiltinCapabilities()

	summary := capabilitiesSummary{
		Skills: make([]capabilityEntry, 0, len(caps.Skills)),
		Tools:  make([]capabilityEntry, 0, len(caps.Tools)),
	}
	for _, entry := range caps.Skills {
		summary.Skills = append(summary.Skills, capabilityEntry{Name: entry.Name, Description: entry.Description})
	}
	for _, entry := range caps.Tools {
		summary.Tools = append(summary.Tools, capabilityEntry{Name: entry.Name, Description: entry.Description})
	}
	return summary
}

// printCapabilities writes the built-in skill and tool names so headless users
// can discover valid values for --skills and --disable-tools.
func printCapabilities(asJSON bool) {
	summary := builtinCapabilities()
	if asJSON {
		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			fmt.Printf("Could not encode capabilities: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}

	fmt.Println("Skills:")
	for _, entry := range summary.Skills {
		fmt.Printf("  %-14s %s\n", entry.Name, truncateOneLine(entry.Description, 100))
	}
	fmt.Println("\nBuilt-in tools:")
	for _, entry := range summary.Tools {
		fmt.Printf("  %-18s %s\n", entry.Name, truncateOneLine(entry.Description, 100))
	}
	fmt.Println("\nUse these names with --skills and --disable-tools.")
}

// canonicalToolName maps the alias accepted by the runtime filter to the name
// used by the registry.
// existingDisabledTools reads tools/tools.json and returns the built-in tools
// that are currently disabled, in registry order.
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
			m.agentFromPicker = false
			m.step = stepAgentName
			m.resetInput("myagent", false)
			return m, nil
		}
		m.setAgent(choice)
		m.agentFromPicker = true
		m.agentExistedAtStart = true
		m.loadExistingCapabilities()
		m.step = stepInitHome

	case stepAgentName:
		name := strings.TrimSpace(m.textInput.Value())
		if err := validateAgentNameInput(name); err != nil {
			m.err = err
			return m, nil
		}
		m.setAgent(name)
		m.agentFromPicker = false
		m.agentExistedAtStart = false
		m.step = stepInitHome

	case stepInitHome:
		if err := m.ensureNameStillFree(); err != nil {
			m.err = err
			return m, nil
		}
		m.step = stepProviderType
		m.cursor = 0
		m.choices = m.providerTypeChoices()

	case stepProviderType:
		choice := m.choices[m.cursor]
		if m.agentExistedAtStart && choice == keepProviderOption {
			m.skipProvider = true
			m.enterSkillsStep()
			return m, nil
		}
		switch choice {
		case "Gemini":
			m.providerType = "gemini"
		case "OpenAI (or compatible)":
			m.providerType = "openai"
		default:
			m.providerType = strings.ToLower(choice)
		}
		m.skipProvider = false
		// Prefill the saved provider name when it matches the chosen type, so
		// changing only the model edits that provider instead of creating a
		// second entry named after the type.
		m.providerName = m.preferredProviderName(m.providerType)
		m.step = stepProviderName
		m.resetInput(m.providerName, false)
		m.textInput.SetValue(m.providerName)

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
		m.providerModel = ""
		// Reconfiguring an existing provider keeps its endpoint and model, so
		// changing only the model does not require retyping everything.
		if saved, ok := m.existingProvider(name); ok && saved.Type == m.providerType {
			if saved.BaseURL != "" {
				m.providerBaseURL = saved.BaseURL
			}
			m.providerModel = saved.DefaultModel
		}
		m.step = stepProviderBaseURL
		m.resetInput(m.providerBaseURL, false)
		m.textInput.SetValue(m.providerBaseURL)

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
		placeholder := "API key"
		if m.existingAPIKey(m.providerName) != "" {
			placeholder = "Press Enter to keep the saved key"
		}
		m.resetInput(placeholder, true)

	case stepProviderKey:
		key := strings.TrimSpace(m.textInput.Value())
		if key == "" {
			// An empty answer keeps the stored key for this provider.
			key = m.existingAPIKey(m.providerName)
		}
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
		m.enterSkillsStep()

	case stepSkills:
		if m.agentExistedAtStart && m.choices[m.cursor] == keepSkillsOption {
			m.skipSkills = true
			m.enterToolsStep()
			return m, nil
		}
		m.skipSkills = false
		m.enterToolsStep()
		if len(m.tools) == 0 {
			m.enterChannelStep()
		}

	case stepTools:
		if m.agentExistedAtStart && m.choices[m.cursor] == keepToolsOption {
			m.skipTools = true
			m.enterChannelStep()
			return m, nil
		}
		m.skipTools = false
		m.enterChannelStep()

	case stepChannelChoice:
		choice := m.choices[m.cursor]
		switch {
		case choice == skipOption || choice == keepChannelsOption:
			m.channelChoice = "skip"
			m.enterChannelExistingStep()
		case choice == "Discord":
			m.channelChoice = "discord"
			m.step = stepDiscordToken
			m.resetInput("Discord Bot Token", true)
		default: // Telegram or Both
			m.channelChoice = strings.ToLower(choice)
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
		m.disableTelegram = false
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
		m.enterChannelExistingStep()

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
		m.enterChannelExistingStep()

	case stepDiscordToken:
		token := strings.TrimSpace(m.textInput.Value())
		if token == "" {
			m.err = fmt.Errorf("Discord bot token is required")
			return m, nil
		}
		m.discordToken = token
		m.disableDiscord = false
		m.enterChannelExistingStep()

	case stepChannelExisting:
		switch m.choices[m.cursor] {
		case "Disable Telegram":
			m.disableTelegram = true
		case "Disable Discord":
			m.disableDiscord = true
		case "Disable both":
			m.disableTelegram = true
			m.disableDiscord = true
		default: // keep current channels
			m.disableTelegram = false
			m.disableDiscord = false
		}
		m.enterBotTypeStep()

	case stepBotType:
		switch m.choices[m.cursor] {
		case keepAccessOption:
			// Leave the saved bot.type untouched.
		case "Public (anyone can chat)":
			m.botType = "public"
		default:
			m.botType = "private"
		}
		m.setupHeartbeatChannel()

	case stepHeartbeatChannel:
		choice := m.choices[m.cursor]
		switch {
		case choice == heartbeatOffOption:
			m.heartbeat = false
			m.heartbeatSet = true
			m.setupOwnerInputs()
			return m, nil
		case choice == skipOption || choice == keepHeartbeatOption:
			// Keeping the current heartbeat on update, or leaving it off.
			m.heartbeatSet = false
			m.heartbeat = false
			m.setupOwnerInputs()
			return m, nil
		}
		m.heartbeat = true
		m.heartbeatSet = true
		m.heartbeatChannel = strings.ToLower(choice)
		m.step = stepHeartbeatID
		m.resetInput(heartbeatPlaceholder(m.heartbeatChannel), false)
		if m.existingHeartbeatChannel == m.heartbeatChannel && m.existingHeartbeatTo != "" {
			m.heartbeatTo = m.existingHeartbeatTo
			m.textInput.SetValue(m.heartbeatTo)
		}

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
		if m.existingHeartbeatChannel == m.heartbeatChannel && m.existingHeartbeatEvery != "" {
			m.heartbeatEvery = m.existingHeartbeatEvery
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
		m.setupOwnerInputs()

	case stepOwnerTelegram:
		owners, err := parseOwnerIDs(m.textInput.Value())
		if err != nil {
			m.err = err
			return m, nil
		}
		m.telegramOwnerIDs = owners
		if m.channelConfigured("discord") {
			m.step = stepOwnerDiscord
			m.resetInput("Discord user ID (optional)", false)
			m.textInput.SetValue(strings.Join(m.discordOwnerIDs, ", "))
			return m, nil
		}
		m.step = stepSummary
		m.cursor = 0
		m.choices = []string{"Save configuration", "Cancel"}

	case stepOwnerDiscord:
		owners, err := parseOwnerIDs(m.textInput.Value())
		if err != nil {
			m.err = err
			return m, nil
		}
		m.discordOwnerIDs = owners
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
		m.step = m.afterSaveStep()
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
		return fmt.Errorf("agent %q is already taken; pick another name, or run `myaaw --agent %s onboard` to update it", m.agentName, m.agentName)
	}
	return nil
}

// initializeAgent creates the layout, registers the agent, assigns a port, and
// wires A2A. It is only called when the user saves the review step, so an
// interrupted wizard leaves nothing behind.
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
func (m onboardModel) input() onboardInput {
	providerName := m.providerName
	if m.skipProvider {
		providerName = ""
	}
	skills := m.selectedSkills()
	if m.skipSkills {
		skills = nil
	}
	return onboardInput{
		ProviderName:       providerName,
		ProviderType:       m.providerType,
		ProviderBaseURL:    m.providerBaseURL,
		ProviderAPIKey:     m.llmKey,
		ProviderModel:      m.providerModel,
		TelegramToken:      m.telegramToken,
		TelegramMode:       m.telegramMode,
		TelegramWebhookURL: m.telegramWebhookURL,
		DiscordToken:       m.discordToken,
		DisableTelegram:    m.disableTelegram && m.telegramToken == "",
		DisableDiscord:     m.disableDiscord && m.discordToken == "",
		BotType:            m.botType,
		Heartbeat:          m.heartbeat,
		HeartbeatSet:       m.heartbeatSet,
		HeartbeatChannel:   m.heartbeatChannel,
		HeartbeatTo:        m.heartbeatTo,
		HeartbeatEvery:     m.heartbeatEvery,
		OwnerIDs:           m.ownerIDsAll(),
		OwnersSet:          m.ownersAsked,
		Skills:             skills,
		DisabledTools:      m.disabledTools(),
		UpdateTools:        !m.skipTools,
	}
}

func (m *onboardModel) setupOwnerInputs() {
	switch {
	case m.channelConfigured("telegram"):
		m.ownersAsked = true
		m.step = stepOwnerTelegram
		m.resetInput("Telegram user ID (optional)", false)
		m.textInput.SetValue(strings.Join(m.telegramOwnerIDs, ", "))
	case m.channelConfigured("discord"):
		m.ownersAsked = true
		m.step = stepOwnerDiscord
		m.resetInput("Discord user ID (optional)", false)
		m.textInput.SetValue(strings.Join(m.discordOwnerIDs, ", "))
	default:
		m.step = stepSummary
		m.cursor = 0
		m.choices = []string{"Save configuration", "Cancel"}
	}
}

// providerTypeChoices adds an explicit keep-current option when updating.
func (m onboardModel) providerTypeChoices() []string {
	if m.agentExistedAtStart {
		return []string{keepProviderOption, "OpenAI (or compatible)", "Gemini"}
	}
	return []string{"OpenAI (or compatible)", "Gemini"}
}

func (m onboardModel) providerTypeCursor() int {
	if m.agentExistedAtStart {
		switch m.providerType {
		case "gemini":
			return 2
		case "openai":
			return 1
		default:
			return 0
		}
	}
	if m.providerType == "gemini" {
		return 1
	}
	return 0
}

func (m onboardModel) skillChoices() []string {
	choices := append([]string{}, m.skills...)
	if m.agentExistedAtStart {
		choices = append([]string{keepSkillsOption}, choices...)
	}
	return choices
}

func (m onboardModel) toolChoices() []string {
	choices := append([]string{}, m.tools...)
	if m.agentExistedAtStart {
		choices = append([]string{keepToolsOption}, choices...)
	}
	return choices
}

func (m *onboardModel) enterSkillsStep() {
	m.step = stepSkills
	m.cursor = 0
	m.choices = m.skillChoices()
}

func (m *onboardModel) enterToolsStep() {
	m.step = stepTools
	m.cursor = 0
	m.choices = m.toolChoices()
}

func (m *onboardModel) enterChannelStep() {
	m.step = stepChannelChoice
	m.cursor = 0
	m.choices = m.channelChoices()
}

// channelChoices puts the "do nothing" answer first on an update, matching the
// provider, skills, and tools steps. "Skip" is used for a new agent.
func (m onboardModel) channelChoices() []string {
	if m.agentExistedAtStart {
		return []string{keepChannelsOption, "Telegram", "Discord", "Both"}
	}
	return []string{"Telegram", "Discord", "Both", skipOption}
}

// enterChannelExistingStep offers to turn saved channels off, which the plain
// "Keep current" answer cannot express. It is skipped when no channel is saved
// yet or when the user just configured one.
func (m *onboardModel) enterChannelExistingStep() {
	if m.agentExistedAtStart && len(m.channelExistingChoices()) > 1 {
		m.step = stepChannelExisting
		m.cursor = 0
		m.choices = m.channelExistingChoices()
		return
	}
	m.enterBotTypeStep()
}

// channelExistingChoices lists the keep/disable options for saved channels. A
// channel that was configured in this run is not offered for disabling.
func (m onboardModel) channelExistingChoices() []string {
	choices := []string{keepChannelsOption}
	telegram := m.existingTelegram && m.telegramToken == ""
	discord := m.existingDiscord && m.discordToken == ""
	if telegram {
		choices = append(choices, "Disable Telegram")
	}
	if discord {
		choices = append(choices, "Disable Discord")
	}
	if telegram && discord {
		choices = append(choices, "Disable both")
	}
	return choices
}

func (m *onboardModel) enterBotTypeStep() {
	m.step = stepBotType
	m.choices = m.botTypeChoices()
	m.cursor = m.botTypeCursor()
}

func (m onboardModel) botTypeChoices() []string {
	choices := []string{"Private (only owner IDs can chat)", "Public (anyone can chat)"}
	if m.agentExistedAtStart {
		choices = append([]string{keepAccessOption}, choices...)
	}
	return choices
}

// botTypeCursor starts on "Keep current access" during an update, so pressing
// Enter through the wizard never changes the access mode by accident.
func (m onboardModel) botTypeCursor() int {
	if m.agentExistedAtStart {
		return 0
	}
	if m.botType == "public" {
		return 1
	}
	return 0
}

// heartbeatChoices keeps the current heartbeat as an explicit option on update
// and adds an off switch, so a running heartbeat can be stopped without editing
// config.json.
func (m onboardModel) heartbeatChoices() []string {
	choices := make([]string, 0, 4)
	if m.channelConfigured("telegram") {
		choices = append(choices, "Telegram")
	}
	if m.channelConfigured("discord") {
		choices = append(choices, "Discord")
	}
	if m.agentExistedAtStart && m.existingHeartbeat {
		choices = append(choices, heartbeatOffOption)
	}
	if m.agentExistedAtStart {
		choices = append(choices, keepHeartbeatOption)
	} else {
		choices = append(choices, skipOption)
	}
	return choices
}

// ownerIDsAll merges the per-channel IDs into the single bot.owner_ids list.
func (m onboardModel) ownerIDsAll() []string {
	seen := make(map[string]bool)
	owners := make([]string, 0, len(m.telegramOwnerIDs)+len(m.discordOwnerIDs))
	for _, id := range append(append([]string{}, m.telegramOwnerIDs...), m.discordOwnerIDs...) {
		if !seen[id] {
			seen[id] = true
			owners = append(owners, id)
		}
	}
	return owners
}

// afterSaveStep skips the global install question unless this is the first
// agent on the machine.
func (m onboardModel) afterSaveStep() int {
	if m.firstAgent {
		return stepInstallGlobal
	}
	if m.restartPending {
		return stepRestart
	}
	return stepDone
}

func (m onboardModel) heartbeatBackStep() int {
	if m.heartbeat {
		return stepHeartbeatEvery
	}
	return stepHeartbeatChannel
}

func (m onboardModel) ownerBackStep() int {
	if m.channelConfigured("discord") {
		return stepOwnerDiscord
	}
	if m.channelConfigured("telegram") {
		return stepOwnerTelegram
	}
	return m.heartbeatBackStep()
}

func (m *onboardModel) setupHeartbeatChannel() {
	m.step = stepHeartbeatChannel
	m.choices = m.heartbeatChoices()
	m.cursor = 0
	if m.existingHeartbeat {
		for i, choice := range m.choices {
			if strings.EqualFold(choice, m.existingHeartbeatChannel) {
				m.cursor = i
				break
			}
		}
	}
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
		models, err := provider.ModelsFor(providerType, baseURL, apiKey)
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
		if m.providerModel != "" {
			m.textInput.SetValue(m.providerModel)
		}
		return m
	}
	m.modelManual = false
	m.choices = append(append([]string{}, msg.models...), manualModelOption)
	m.cursor = 0
	// Start on the model that is already configured, if the provider still
	// offers it.
	for i, choice := range m.choices {
		if choice == m.providerModel {
			m.cursor = i
			break
		}
	}
	return m
}

// --- Config merge ---

// mergeOnboardConfig applies wizard answers to an existing config. Channels and
// heartbeat are only touched when the user actually configured them, so
// re-running onboarding never silently drops working settings.
// sanitizeOnboardConfig strips values that shipped with the old template so a
// fresh install never starts a bot with a fake token or blocks every real user.
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
		switch {
		case agentFlag != "":
			m.step = stepWelcome
		case m.agentFromPicker:
			m.step = stepAgentChoice
			m.choices = append(m.initializedAgents(), newAgentOption)
			m.cursor = 0
			for i, choice := range m.choices {
				if choice == m.agentName {
					m.cursor = i
					break
				}
			}
		default:
			m.step = stepAgentName
			m.resetInput("myagent", false)
			m.textInput.SetValue(m.agentName)
		}

	case stepProviderType:
		m.step = stepInitHome

	case stepProviderName:
		m.step = stepProviderType
		m.choices = m.providerTypeChoices()
		m.cursor = m.providerTypeCursor()

	case stepProviderBaseURL:
		m.step = stepProviderName
		m.resetInput(m.providerType, false)
		m.textInput.SetValue(m.providerName)

	case stepProviderKey:
		m.step = stepProviderBaseURL
		m.resetInput(m.providerBaseURL, false)
		m.textInput.SetValue(m.providerBaseURL)

	case stepProviderModel:
		m.step = stepProviderKey
		m.resetInput("API key", true)
		m.textInput.SetValue(m.llmKey)
		m.modelsLoading = false

	case stepTools:
		m.step = stepSkills
		m.cursor = 0
		m.choices = m.skillChoices()

	case stepSkills:
		if m.skipProvider {
			m.step = stepProviderType
			m.cursor = 0
			m.choices = m.providerTypeChoices()
			break
		}
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
		m.choices = m.toolChoices()

	case stepTelegramMode:
		m.step = stepChannelChoice
		m.cursor = 0
		m.choices = m.channelChoices()

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
		m.choices = m.channelChoices()

	case stepHeartbeatChannel:
		m.step = stepBotType
		m.choices = m.botTypeChoices()
		m.cursor = m.botTypeCursor()

	case stepBotType:
		if m.agentExistedAtStart && len(m.channelExistingChoices()) > 1 {
			m.step = stepChannelExisting
			m.choices = m.channelExistingChoices()
			m.cursor = 0
			break
		}
		m.step = stepChannelChoice
		m.cursor = 0
		m.choices = m.channelChoices()

	case stepChannelExisting:
		m.step = stepChannelChoice
		m.cursor = 0
		m.choices = m.channelChoices()

	case stepHeartbeatID:
		m.step = stepHeartbeatChannel
		m.choices = m.heartbeatChoices()
		m.cursor = 0
		for i, choice := range m.choices {
			if strings.EqualFold(choice, m.heartbeatChannel) {
				m.cursor = i
				break
			}
		}

	case stepHeartbeatEvery:
		m.step = stepHeartbeatID
		m.resetInput(heartbeatPlaceholder(m.heartbeatChannel), false)
		m.textInput.SetValue(m.heartbeatTo)

	case stepOwnerDiscord:
		if m.channelConfigured("telegram") {
			m.step = stepOwnerTelegram
			m.resetInput("Telegram user ID (optional)", false)
			m.textInput.SetValue(strings.Join(m.telegramOwnerIDs, ", "))
			break
		}
		m.step = m.heartbeatBackStep()

	case stepOwnerTelegram:
		m.step = m.heartbeatBackStep()

	case stepSummary:
		m.step = m.ownerBackStep()
		switch m.step {
		case stepOwnerTelegram:
			m.resetInput("Telegram user ID (optional)", false)
			m.textInput.SetValue(strings.Join(m.telegramOwnerIDs, ", "))
		case stepOwnerDiscord:
			m.resetInput("Discord user ID (optional)", false)
			m.textInput.SetValue(strings.Join(m.discordOwnerIDs, ", "))
		case stepHeartbeatEvery:
			m.resetInput(defaultHeartbeatEV, false)
			m.textInput.SetValue(m.heartbeatEvery)
		case stepHeartbeatChannel:
			m.cursor = 0
		}

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
		b.WriteString("\n\nChoose the AI engine for this agent:\n")
		if m.agentExistedAtStart {
			b.WriteString(theme.SecondaryStyle.Render("Pick 'Keep current provider' to leave the provider and model unchanged.") + "\n")
		}
		b.WriteString("\n")
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
		b.WriteString("\n\nStored in config/config.json and masked by `myaaw config dump`.\n")
		if m.existingAPIKey(m.providerName) != "" {
			b.WriteString(theme.SecondaryStyle.Render("A key is already saved for this provider; press Enter to keep it.") + "\n")
		}
		b.WriteString("\n")
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
			b.WriteString(fmt.Sprintf("\n\nInstalled skills are pre-selected. Enter on '%s' to change nothing.\n", keepSkillsOption))
		} else {
			b.WriteString("\n\nNo skill is selected by default. Space to toggle, Enter to continue.\n")
		}
		b.WriteString(theme.SecondaryStyle.Render("Only the skills you select get a folder under skills/<name>/.") + "\n\n")
		b.WriteString(m.renderMultiSelect(m.skillsSelected))

	case stepTools:
		b.WriteString(titleStyle.Render("Built-in Tools"))
		if m.agentExistedAtStart {
			b.WriteString(fmt.Sprintf("\n\nCurrent settings are pre-selected. Enter on '%s' to change nothing.\n", keepToolsOption))
		} else {
			b.WriteString("\n\nEvery built-in tool is enabled by default. Space to disable, Enter to continue.\n")
		}
		b.WriteString(theme.SecondaryStyle.Render("Disabled tools are written to tools/tools.json as false.") + "\n\n")
		b.WriteString(m.renderMultiSelect(m.toolsSelected))

	case stepChannelChoice:
		b.WriteString(titleStyle.Render("Select Channels"))
		b.WriteString("\n\nWhere should your bot live?\n")
		if m.agentExistedAtStart {
			b.WriteString(theme.SecondaryStyle.Render("'Keep current' leaves the channel settings exactly as they are.") + "\n")
		}
		b.WriteString("\n")
		b.WriteString(m.renderChoices())

	case stepChannelExisting:
		b.WriteString(titleStyle.Render("Existing Channels"))
		b.WriteString("\n\nTurn a saved channel off, or keep them all as they are?\n")
		b.WriteString(theme.SecondaryStyle.Render("Disabling keeps the token so you can turn it back on later.") + "\n\n")
		b.WriteString(m.renderChoices())

	case stepBotType:
		b.WriteString(titleStyle.Render("Bot Access"))
		b.WriteString("\n\nWho is allowed to chat with this bot?\n")
		if m.agentExistedAtStart {
			b.WriteString(theme.SecondaryStyle.Render(fmt.Sprintf("Currently %s. '%s' leaves it unchanged.", m.botType, keepAccessOption)) + "\n")
		}
		b.WriteString(theme.SecondaryStyle.Render("Private relies on owner IDs for Telegram and Discord. Public lets anyone chat.") + "\n")
		b.WriteString(theme.SecondaryStyle.Render("Terminal, REST API, and A2A are never filtered by owner IDs.") + "\n\n")
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
		b.WriteString("\n\nShould the agent send a periodic self-check?\n")
		if m.existingHeartbeat {
			b.WriteString(theme.SecondaryStyle.Render(fmt.Sprintf("Currently %s → %s every %s.", m.existingHeartbeatChannel, m.existingHeartbeatTo, m.existingHeartbeatEvery)) + "\n")
		}
		if m.agentExistedAtStart {
			b.WriteString(theme.SecondaryStyle.Render(fmt.Sprintf("'%s' keeps it as it is; '%s' turns it off.", keepHeartbeatOption, heartbeatOffOption)) + "\n")
		}
		b.WriteString("\n")
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

	case stepOwnerTelegram:
		b.WriteString(titleStyle.Render("Telegram Owner ID"))
		b.WriteString("\n\nOptional. Leave it empty and add it later when you know your ID.\n")
		if m.agentExistedAtStart {
			b.WriteString(theme.SecondaryStyle.Render("Existing IDs are pre-filled; delete them only if you want to remove them.") + "\n")
		}
		b.WriteString(theme.SecondaryStyle.Render("You can get it from @userinfobot. Comma separated for several IDs.") + "\n\n")
		b.WriteString(m.textInput.View())

	case stepOwnerDiscord:
		b.WriteString(titleStyle.Render("Discord Owner ID"))
		b.WriteString("\n\nOptional. Leave it empty and add it later when you know your ID.\n")
		if m.agentExistedAtStart {
			b.WriteString(theme.SecondaryStyle.Render("Existing IDs are pre-filled; delete them only if you want to remove them.") + "\n")
		}
		b.WriteString(theme.SecondaryStyle.Render("Enable Developer Mode, right-click your profile, then Copy User ID.") + "\n\n")
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
	if m.skipProvider {
		row("Provider", "unchanged (current)")
	} else {
		row("Provider", fmt.Sprintf("%s [%s]", m.providerName, m.providerType))
		row("Base URL", m.providerBaseURL)
		row("Model", m.providerModel)
	}
	if m.skipSkills {
		row("Skills", "unchanged (current)")
	} else {
		skills := "none"
		if selected := m.selectedSkills(); len(selected) > 0 {
			skills = strings.Join(selected, ", ")
		}
		row("Skills", skills)
	}
	disabled := m.disabledTools()
	if m.skipTools {
		row("Tools", "unchanged (current)")
	} else {
		row("Tools", fmt.Sprintf("%d enabled, %d disabled", len(m.tools)-len(disabled), len(disabled)))
		if len(disabled) > 0 {
			row("Disabled", strings.Join(disabled, ", "))
		}
	}
	row("Telegram", m.channelSummary("telegram", m.telegramToken, m.telegramMode))
	if m.telegramMode == "webhook" && m.telegramWebhookURL != "" {
		row("Webhook", m.telegramWebhookURL+"/webhook/telegram")
	}
	row("Discord", m.channelSummary("discord", m.discordToken, ""))
	row("Bot type", m.botType)
	switch {
	case m.heartbeat:
		row("Heartbeat", fmt.Sprintf("%s → %s every %s", m.heartbeatChannel, m.heartbeatTo, m.heartbeatEvery))
	case m.heartbeatSet:
		row("Heartbeat", "disabled")
	case m.existingHeartbeat:
		row("Heartbeat", "unchanged (current)")
	default:
		row("Heartbeat", "disabled")
	}
	if m.channelConfigured("telegram") {
		row("Telegram owner", joinOwnerIDs(m.telegramOwnerIDs))
	}
	if m.channelConfigured("discord") {
		row("Discord owner", joinOwnerIDs(m.discordOwnerIDs))
	}
	if !m.channelConfigured("telegram") && !m.channelConfigured("discord") {
		row("Owner IDs", joinOwnerIDs(m.ownerIDsAll()))
	}

	if len(m.ownerIDsAll()) == 0 && m.botType == "private" && (m.channelConfigured("telegram") || m.channelConfigured("discord")) {
		b.WriteString("\n" + theme.WarningStyle.Render("⚠  No owner IDs: with bot.type \"private\", Telegram and Discord ignore every message.") +
			"\n" + theme.MutedStyle.Render("   Terminal (`myaaw chat`), the REST API, and A2A are not filtered by owner IDs."))
	}
	if m.providerType != "gemini" {
		b.WriteString("\n" + theme.MutedStyle.Render("ℹ  `myaaw voice` (real-time) needs Gemini; `voice-classic` works with any provider."))
	}
	if m.a2aWarning != nil {
		b.WriteString("\n" + theme.WarningStyle.Render("⚠  A2A contacts were not seeded: "+m.a2aWarning.Error()))
	}
	return b.String()
}

func joinOwnerIDs(ids []string) string {
	if len(ids) == 0 {
		return "none (optional)"
	}
	return strings.Join(ids, ", ")
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
	if m.channelDisabled(channel) {
		return "disabled"
	}
	if token == "" {
		if m.agentExistedAtStart && m.channelConfigured(channel) {
			return "unchanged (current)"
		}
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
		if choice == keepSkillsOption || choice == keepToolsOption {
			box = "[–]"
		} else if selected[choice] {
			box = "[x]"
		}
		line := fmt.Sprintf("%s %s %s", cursor, box, style.Render(choice))
		if description := m.descriptionFor(choice); description != "" {
			line += theme.MutedStyle.Render(" — " + truncateOneLine(description, 64))
		}
		s.WriteString(line + "\n")
	}
	return s.String()
}

func (m onboardModel) descriptionFor(name string) string {
	if m.step == stepSkills {
		return m.skillDescriptions[name]
	}
	return m.toolDescriptions[name]
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

func (m onboardModel) timelineLabel() string {
	switch m.step {
	case stepWelcome:
		return "Welcome"
	case stepAgentChoice, stepAgentName:
		return "Agent"
	case stepInitHome:
		return "Initialize"
	case stepProviderType, stepProviderName, stepProviderBaseURL, stepProviderKey, stepProviderModel:
		return "Provider"
	case stepSkills, stepTools:
		return "Skills & Tools"
	case stepChannelChoice, stepChannelExisting, stepTelegramMode, stepTelegramToken, stepTelegramWebhookURL, stepDiscordToken:
		return "Channels"
	case stepBotType:
		return "Access"
	case stepHeartbeatChannel, stepHeartbeatID, stepHeartbeatEvery:
		return "Heartbeat"
	case stepOwnerTelegram, stepOwnerDiscord:
		return "Owner"
	case stepSummary:
		return "Review"
	case stepInstallGlobal, stepRestart:
		return "Install"
	case stepDone:
		return "Done"
	}
	return "Welcome"
}

func (m onboardModel) renderTimeline() string {
	steps := []string{
		"Welcome",
		"Agent",
		"Initialize",
		"Provider",
		"Skills & Tools",
		"Channels",
		"Access",
		"Heartbeat",
		"Owner",
		"Review",
	}
	if m.firstAgent {
		steps = append(steps, "Install")
	}
	steps = append(steps, "Done")

	current := m.timelineLabel()
	if current == "Install" && !m.firstAgent {
		current = "Done"
	}
	timelineIdx := 0
	for i, label := range steps {
		if label == current {
			timelineIdx = i
			break
		}
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
	BotType       string            `json:"bot_type,omitempty"`
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
	if summary.BotType != "" {
		fmt.Printf("Bot type: %s\n", summary.BotType)
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
	summary.BotType = cfg.Bot.Type
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

	// An update usually only touches part of the config, so the provider can be
	// left alone explicitly (--keep-provider) or by omitting --provider.
	keepProvider := opts.keepProvider || (strings.TrimSpace(opts.provider) == "" && existing != nil)
	if keepProvider {
		if existing == nil {
			return in, fmt.Errorf("--provider is required for a new agent (openai or gemini)")
		}
	} else {
		providerType := strings.ToLower(strings.TrimSpace(opts.provider))
		if providerType != "openai" && providerType != "gemini" {
			return in, fmt.Errorf("--provider must be openai or gemini")
		}

		providerName := strings.TrimSpace(opts.provName)
		if providerName == "" {
			// Reuse an existing provider with the same type, even when its name
			// differs from the type (for example "deepseek" of type openai).
			providerName = preferredProviderNameFromConfig(existing, providerType)
		}
		if strings.ContainsAny(providerName, " \t") {
			return in, fmt.Errorf("--provider-name cannot contain spaces")
		}

		baseURL := strings.TrimRight(strings.TrimSpace(opts.baseURL), "/")
		if baseURL == "" && existing != nil {
			// Keep the saved endpoint of the provider being updated.
			baseURL = strings.TrimRight(existing.Providers[providerName].BaseURL, "/")
		}
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
		if isTemplateValue(apiKey) {
			apiKey = ""
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
	}

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

	botType := strings.ToLower(strings.TrimSpace(opts.botType))
	switch botType {
	case "", "private", "public":
	default:
		return in, fmt.Errorf("--bot-type must be private or public")
	}
	in.BotType = botType

	for _, name := range strings.Split(opts.disableChannels, ",") {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "":
		case "telegram":
			in.DisableTelegram = true
		case "discord":
			in.DisableDiscord = true
		case "both":
			in.DisableTelegram = true
			in.DisableDiscord = true
		default:
			return in, fmt.Errorf("--disable-channel must be telegram, discord, or both")
		}
	}

	hbChannel := strings.ToLower(strings.TrimSpace(opts.hbChannel))
	switch hbChannel {
	case "":
		// Leave heartbeat untouched.
	case "skip":
		// Nothing to write; keep existing heartbeat as is.
	case "off", "disable", "disabled":
		in.HeartbeatSet = true
		in.Heartbeat = false
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
		in.HeartbeatSet = true
		in.HeartbeatChannel = hbChannel
		in.HeartbeatTo = strings.TrimSpace(opts.hbTo)
		in.HeartbeatEvery = every
	}

	owners, err := parseOwnerIDs(opts.owner)
	if err != nil {
		return in, fmt.Errorf("--owner: %v", err)
	}
	tgOwners, err := parseOwnerIDs(opts.tgOwner)
	if err != nil {
		return in, fmt.Errorf("--telegram-owner: %v", err)
	}
	dcOwners, err := parseOwnerIDs(opts.dcOwner)
	if err != nil {
		return in, fmt.Errorf("--discord-owner: %v", err)
	}
	in.OwnerIDs = mergeOwnerIDs(owners, tgOwners, dcOwners)
	// Headless only rewrites the owner list when a flag actually carries an ID,
	// so a run without --owner keeps whatever is saved.
	in.OwnersSet = len(in.OwnerIDs) > 0

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
		in.UpdateTools = true
	case existing != nil:
		// Updating without --disable-tools keeps the tools that are already off.
		in.DisabledTools = existingDisabledTools(agentName)
		in.UpdateTools = false
	default:
		in.UpdateTools = true
	}

	if opts.port < 0 || opts.port > 65535 {
		return in, fmt.Errorf("--port must be between 1 and 65535")
	}
	return in, nil
}

// parseOnboardSelection validates a comma-separated list (or "all") against
// the known built-in names and returns them in the canonical order.
// mergeOwnerIDs concatenates owner ID groups, dropping duplicates.
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
// saveBuiltinToolSelection writes builtin_tools to tools/tools.json while
// preserving custom_tools and mcp_servers. Every built-in is listed, so a
// reconfigure re-enables tools that are no longer disabled.
// ensureEnvFile writes the embedded .env template when the file does not
// exist. The bool reports whether the file was created.
// saveConfigToFile writes config.json through a temp file so a crash mid-write
// cannot leave a truncated config behind.
// writeFileAtomic writes data through a temp file in the same directory.
// extractEmbedDirExcept copies an embedded directory while skipping the given
// top-level entries.
