package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"myaaw"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup for first-time use",
	Long:  "Initialize ~/.myaaw configuration folder, setup .env, and get everything ready.",
	Run:   runOnboard,
}

func init() {
	rootCmd.AddCommand(onboardCmd)
}

func runOnboard(cmd *cobra.Command, args []string) {
	p := tea.NewProgram(initialOnboardModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

// Steps
const (
	stepWelcome = iota
	stepInitHome
	stepProviderChoice
	stepEnvLLM
	stepChannelChoice
	stepTelegramMode
	stepTelegramToken
	stepDiscordToken
	stepHeartbeatChannel
	stepHeartbeatID
	stepOwner
	stepInstallGlobal
	stepDone
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
	myaawHome  string
	envFile    string
	configFile string

	// Collected Data
	providerChoice   string // "openai", "groq", "gemini", "ollama"
	llmKey           string
	channelChoice    string // "telegram", "discord", "both", "skip"
	telegramMode     string // "polling", "webhook"
	telegramToken    string
	telegramNgrok    string
	discordToken     string
	heartbeat        bool
	heartbeatChannel string
	heartbeatTo      string
	ownerID          string
	sudoPassword     string

	// Flags
	globalPathDone bool

	// Command output
	cmdOutput  []string
	isRunning  bool
	outputChan chan tea.Msg
}

func initialOnboardModel() onboardModel {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 40

	home, _ := os.UserHomeDir()
	myaawHome := filepath.Join(home, ".myaaw")

	return onboardModel{
		step:       stepWelcome,
		textInput:  ti,
		homeDir:    home,
		myaawHome:  myaawHome,
		envFile:    filepath.Join(myaawHome, ".env"),
		configFile: filepath.Join(myaawHome, "config", "config.json"),
		choices:    []string{"Yes", "No"}, // Default choices
		outputChan: make(chan tea.Msg, 10),
	}
}

func (m onboardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m onboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			m_next, cmd_next := m.nextStep(msg)
			m = m_next.(onboardModel)
			cmd = cmd_next
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
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

	// Handle async command results
	switch msg := msg.(type) {
	case globalPathFinishedMsg:
		m.isRunning = false
		if msg.err != nil {
			m.globalPathDone = false
			m.cmdOutput = []string{theme.ErrorStyle.Render("Sudo failed: " + msg.err.Error())}
			m.textInput.Reset()
			m.textInput.EchoMode = textinput.EchoPassword // Ensure it's masked on retry
		} else {
			m.globalPathDone = true
			home, _ := os.UserHomeDir()
			targetPath := filepath.Join(home, ".local", "bin", "myaaw")
			if runtime.GOOS == "windows" {
				targetPath = "C:\\Windows\\System32\\myaaw.exe (Manual step recommended)"
			}
			m.cmdOutput = []string{theme.SuccessStyle.Render(fmt.Sprintf("Successfully installed at %s!", targetPath))}
			m.textInput.Reset()
		}
	case cmdOutputMsg:
		m.cmdOutput = append(m.cmdOutput, string(msg))
		if len(m.cmdOutput) > 8 {
			m.cmdOutput = m.cmdOutput[1:]
		}
		return m, m.listenForOutput()
	}

	// Important: If we are running a background command, we MUST keep listening
	// Unless we just received the finish message or cmd is already a listener
	if m.isRunning && cmd == nil {
		cmd = m.listenForOutput()
	}

	return m, cmd
}

func (m onboardModel) View() string {
	// Layout: Timeline on Left, Content on Right

	timeline := m.renderTimeline()
	content := m.renderContent()

	// container style
	mainStyle := lipgloss.NewStyle().Margin(1, 1)

	return mainStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, timeline, content))
}

// --- Logic ---

func (m onboardModel) isTextInputStep() bool {
	switch m.step {
	case stepEnvLLM, stepTelegramToken, stepDiscordToken, stepHeartbeatID, stepOwner:
		return true
	default:
		return false
	}
}

func (m onboardModel) nextStep(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.step {
	case stepWelcome:
		m.step = stepInitHome
		m.cursor = 0
		m.choices = []string{"Initialize .myaaw directory"}

	case stepInitHome:
		setupMyaawHome(m.myaawHome)
		m.step = stepProviderChoice
		m.cursor = 0
		m.choices = []string{"OpenAI", "Gemini"}

	case stepProviderChoice:
		m.providerChoice = strings.ToLower(m.choices[m.cursor])
		m.step = stepEnvLLM
		m.textInput.Placeholder = "sk-..."
		m.textInput.Reset()

	case stepEnvLLM:
		m.llmKey = m.textInput.Value()
		// Credentials will be saved in config.json in saveConfig()
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
			m.textInput.Placeholder = "Discord Bot Token"
			m.textInput.Reset()
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
		m.textInput.Placeholder = "Telegram Bot Token"
		m.textInput.Reset()

	case stepTelegramToken:
		m.telegramToken = m.textInput.Value()
		if m.channelChoice == "both" {
			m.step = stepDiscordToken
			m.textInput.Placeholder = "Discord Bot Token"
			m.textInput.Reset()
		} else {
			m.setupHeartbeatChannel()
		}

	case stepDiscordToken:
		m.discordToken = m.textInput.Value()
		m.setupHeartbeatChannel()

	case stepHeartbeatChannel:
		choice := strings.ToLower(m.choices[m.cursor])
		if choice == "skip" {
			m.heartbeat = false
			m.step = stepOwner
			m.setupOwnerInput()
		} else {
			m.heartbeat = true
			m.heartbeatChannel = choice
			m.step = stepHeartbeatID
			m.textInput.Placeholder = "Recipient User/Channel ID"
			m.textInput.Reset()
		}

	case stepHeartbeatID:
		m.heartbeatTo = m.textInput.Value()
		m.step = stepOwner
		m.setupOwnerInput()

	case stepOwner:
		m.ownerID = m.textInput.Value()
		// Save Configuration
		m.saveConfig()

		m.step = stepInstallGlobal
		m.cursor = 0
		m.choices = []string{"Yes, install globally", "No"}

	case stepInstallGlobal:
		if m.isRunning {
			return m, nil
		}
		if m.globalPathDone {
			m.step = stepDone
			m.cmdOutput = nil
			return m, nil
		}
		if m.cursor == 0 {
			m.isRunning = true
			m.cmdOutput = []string{"Installing binary..."}
			return m, m.runGlobalPathCmd()
		} else {
			m.step = stepDone
		}

	case stepDone:
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "enter" {
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

func (m *onboardModel) setupHeartbeatChannel() {
	m.step = stepHeartbeatChannel
	m.cursor = 0
	m.choices = []string{}
	// Only offer active channels
	if m.telegramToken != "" {
		m.choices = append(m.choices, "Telegram")
	}
	if m.discordToken != "" {
		m.choices = append(m.choices, "Discord")
	}
	m.choices = append(m.choices, "Skip")
}

func (m *onboardModel) setupOwnerInput() {
	m.textInput.Placeholder = "Owner ID (Telegram/Discord ID)"
	m.textInput.Reset()
}

func (m onboardModel) runGlobalPathCmd() tea.Cmd {
	go func() {
		exe, _ := os.Executable()
		if runtime.GOOS == "windows" {
			m.outputChan <- globalPathFinishedMsg{nil}
			return
		}

		home, _ := os.UserHomeDir()
		targetDir := filepath.Join(home, ".local", "bin")
		os.MkdirAll(targetDir, 0755)
		targetPath := filepath.Join(targetDir, "myaaw")

		cmd := exec.Command("mv", exe, targetPath)

		var combinedOut strings.Builder
		cmd.Stdout = &combinedOut
		cmd.Stderr = &combinedOut

		err := cmd.Run()
		if err != nil {
			outStr := strings.TrimSpace(combinedOut.String())
			if outStr != "" {
				m.outputChan <- globalPathFinishedMsg{fmt.Errorf("%v: %s", err, outStr)}
			} else {
				m.outputChan <- globalPathFinishedMsg{err}
			}
			return
		}
		m.outputChan <- globalPathFinishedMsg{nil}
	}()
	return m.listenForOutput()
}

func (m onboardModel) listenForOutput() tea.Cmd {
	return func() tea.Msg {
		return <-m.outputChan
	}
}

type cmdOutputMsg string
type globalPathFinishedMsg struct{ err error }

func (m *onboardModel) setupHeartbeatInput() {
	m.textInput.Placeholder = "Heartbeat Output ID (Empty to skip)"
	m.textInput.Reset()
}

func (m onboardModel) saveConfig() {
	// Logic to load, update, save config.json
	cfg, _ := loadConfigFromFile(m.configFile)
	if cfg == nil {
		cfg = &config.Config{}
	}

	// Update Provider
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}
	baseURL := ""
	apiKey := m.llmKey
	
	cfg.DefaultProvider = m.providerChoice
	cfg.Providers[m.providerChoice] = config.ProviderConfig{
		Type:    m.providerChoice,
		APIKey:  apiKey,
		BaseURL: baseURL,
	}

	// Update Channels
	if m.telegramToken != "" {
		if cfg.Channels.Telegram == nil {
			cfg.Channels.Telegram = &config.ChannelConfig{}
		}
		cfg.Channels.Telegram.Active = true
		cfg.Channels.Telegram.Token = m.telegramToken
		cfg.Channels.Telegram.Mode = m.telegramMode
	}
	if m.discordToken != "" {
		if cfg.Channels.Discord == nil {
			cfg.Channels.Discord = &config.ChannelConfig{}
		}
		cfg.Channels.Discord.Active = true
		cfg.Channels.Discord.Token = m.discordToken
	}

	// Heartbeat
	if m.heartbeat {
		cfg.Heartbeat.Active = true
		cfg.Heartbeat.To = m.heartbeatTo
		// Default to telegram if active, else discord
		if m.telegramToken != "" {
			cfg.Heartbeat.Channel = "telegram"
		} else {
			cfg.Heartbeat.Channel = "discord"
		}
		cfg.Heartbeat.Every = "30m"
	}

	// Owner
	if m.ownerID != "" {
		cfg.Bot.OwnerIDs = []string{m.ownerID}
	}

	saveConfigToFile(m.configFile, cfg)
}

// --- Rendering ---

func (m onboardModel) renderContent() string {
	var b strings.Builder

	padding := lipgloss.NewStyle().PaddingLeft(4)
	titleStyle := theme.HighlightStyle.MarginBottom(1)

	switch m.step {
	case stepWelcome:
		b.WriteString(titleStyle.Render("Welcome to Myaaw!"))
		b.WriteString("\n\nThis wizard will guide you through setting up your AI assistant.\n")
		b.WriteString("We'll configure your keys, channels, and infrastructure.\n\n")
		b.WriteString(theme.SecondaryStyle.Render("Press Enter to start."))

	case stepInitHome:
		b.WriteString(titleStyle.Render("Initialize Configuration"))
		b.WriteString(fmt.Sprintf("\n\nDirectory: %s\n\n", m.myaawHome))
		b.WriteString(m.renderChoices())

	case stepProviderChoice:
		b.WriteString(titleStyle.Render("Select Default LLM Provider"))
		b.WriteString("\n\nChoose the AI engine for your assistant:\n\n")
		b.WriteString(m.renderChoices())

	case stepEnvLLM:
		b.WriteString(titleStyle.Render(strings.Title(m.providerChoice) + " API Key"))
		b.WriteString("\n\nEnter your API key to connect to the provider.\n")
		b.WriteString(m.textInput.View())

	case stepChannelChoice:
		b.WriteString(titleStyle.Render("Select Channels"))
		b.WriteString("\n\nWhere should your bot live?\n\n")
		b.WriteString(m.renderChoices())

	case stepTelegramMode:
		b.WriteString(titleStyle.Render("Telegram Connection Mode"))
		b.WriteString("\n\n")
		b.WriteString(m.renderChoices())

	case stepTelegramToken:
		b.WriteString(titleStyle.Render("Telegram Bot Token"))
		b.WriteString("\n\nGet this from @BotFather on Telegram.\n")
		b.WriteString(m.textInput.View())

	case stepDiscordToken:
		b.WriteString(titleStyle.Render("Discord Bot Token"))
		b.WriteString("\n\nGet this from the Discord Developer Portal.\n")
		b.WriteString(m.textInput.View())

	case stepHeartbeatChannel:
		b.WriteString(titleStyle.Render("Heartbeat Channel"))
		b.WriteString("\n\nSelect a channel for status updates:\n\n")
		b.WriteString(m.renderChoices())

	case stepHeartbeatID:
		b.WriteString(titleStyle.Render("Heartbeat User/Channel ID"))
		b.WriteString(fmt.Sprintf("\n\nEnter the ID on %s:\n", strings.Title(m.heartbeatChannel)))
		b.WriteString(m.textInput.View())

	case stepOwner:
		b.WriteString(titleStyle.Render("Bot Owner"))
		b.WriteString("\n\nEnter your User ID (Telegram/Discord) to be the bot admin.\n")
		b.WriteString(m.textInput.View())

	case stepInstallGlobal:
		b.WriteString(titleStyle.Render("Install Globally"))
		if m.isRunning {
			b.WriteString("\n\n" + theme.SecondaryStyle.Render("Moving binary..."))
		} else if m.globalPathDone {
			b.WriteString("\n\n" + strings.Join(m.cmdOutput, "\n"))
			b.WriteString("\n\n" + theme.SecondaryStyle.Render("Press Enter to finish."))
		} else {
			if len(m.cmdOutput) > 0 {
				b.WriteString("\n" + strings.Join(m.cmdOutput, "\n") + "\n")
			}
			b.WriteString("\n\nMake 'myaaw' available globally in your terminal?\n\n")
			b.WriteString(m.renderChoices())
		}

	case stepDone:
		b.WriteString(theme.SuccessStyle.Render("✨ Onboarding Complete!"))
		b.WriteString("\n\nYou're all set. Try running:\n")
		b.WriteString(theme.HighlightStyle.Render("  myaaw start") + " (to run bot in background for Telegram/Discord)\n")
		b.WriteString(theme.HighlightStyle.Render("  myaaw chat") + " (to chat directly in the terminal)\n\n")
		b.WriteString(theme.SecondaryStyle.Render("Press Enter to finish and exit."))
	}

	return padding.Render(b.String())
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
		"Disclaimer",
		"Initialize",
		"API Keys",
		"Channels",
		"Heartbeat",
		"Owner",
		"Install Global",
		"Done",
	}

	// Map current machine step to timeline index
	timelineIdx := 0
	switch m.step {
	case stepWelcome:
		timelineIdx = 0
	case stepInitHome:
		timelineIdx = 1
	case stepEnvLLM:
		timelineIdx = 2
	case stepChannelChoice, stepTelegramMode, stepTelegramToken, stepDiscordToken:
		timelineIdx = 3
	case stepHeartbeatChannel, stepHeartbeatID:
		timelineIdx = 4
	case stepOwner:
		timelineIdx = 5
	case stepInstallGlobal:
		timelineIdx = 6
	case stepDone:
		timelineIdx = 7
	}

	var b strings.Builder
	for i, label := range steps {
		bullet := "○"
		style := theme.MutedStyle

		if i < timelineIdx {
			bullet = "◉"
			style = theme.SuccessStyle // Completed
		} else if i == timelineIdx {
			bullet = "◉"
			style = theme.HighlightStyle // Active
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

// --- Copied Utilities (Adapted) ---

func setupMyaawHome(targetDir string) {
	if _, err := os.Stat(targetDir); err == nil {
		return // Already exists
	}
	extractEmbedDir(myaaw.DefaultMyaawDir, ".myaaw", targetDir)
}

func ensureEnvFile(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.WriteFile(path, []byte(myaaw.DefaultEnvExample), 0644)
	}
}

func setupGlobalPath() {
	exe, _ := os.Executable()
	if runtime.GOOS != "windows" {
		home, _ := os.UserHomeDir()
		targetDir := filepath.Join(home, ".local", "bin")
		os.MkdirAll(targetDir, 0755)
		cmd := exec.Command("mv", exe, filepath.Join(targetDir, "myaaw"))
		cmd.Run()
	}
}

// Reuse existing helpers from original file
func updateEnvFile(path, key, value string) {
	input, err := os.ReadFile(path)
	if err != nil {
		return
	}

	lines := strings.Split(string(input), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
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
	return os.WriteFile(path, data, 0644)
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
			os.MkdirAll(destPath, 0755)
			extractEmbedDir(embeddedFS, srcPath, destPath)
		} else {
			data, _ := embeddedFS.ReadFile(srcPath)
			os.WriteFile(destPath, data, 0644)
		}
	}
	return nil
}

// onboarding utilities
