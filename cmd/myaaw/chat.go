package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
	cliAdapter "myaaw/internal/channel/cli"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/services/bot/repository"
	"myaaw/internal/services/bot/service"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var chatMessage string

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Chat with your AI assistant",
	Long:  "Start an interactive TUI chat session, or send a one-shot message with -m flag.",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()
		config.ConnectDatabases()

		botService := createBotService()
		adapter := cliAdapter.NewCLIAdapter()

		if chatMessage != "" {
			text, images, err := parseFileAttachments(chatMessage)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Error parsing attachments: %v\n", err)
				os.Exit(1)
			}
			runOneShot(botService, adapter, text, images)
		} else {
			runInteractive(botService, adapter)
		}
	},
}

func init() {
	chatCmd.Flags().StringVarP(&chatMessage, "message", "m", "", "Send a one-shot message (non-interactive)")
}

func createBotService() service.BotService {
	userRepo := repository.NewUserRepository(config.DB)
	convRepo := repository.NewConversationRepository(config.DB)
	return service.NewBotService(userRepo, convRepo)
}

func createIncomingMessage(text string, images []string) *channel.IncomingMessage {
	payload, _ := json.Marshal(map[string]interface{}{
		"user_id": 0,
		"text":    text,
	})
	adapter := cliAdapter.NewCLIAdapter()
	msg, _ := adapter.ParseIncoming(payload)
	msg.Images = images
	return msg
}

func runOneShot(botService service.BotService, adapter *cliAdapter.CLIAdapter, message string, images []string) {
	msg := createIncomingMessage(message, images)

	if config.StreamResponse {
		_, err := adapter.SendStream(msg, func(onChunk func(chunk channel.StreamChunk)) error {
			_, err := botService.BotStream(msg, onChunk)
			return err
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		out, err := botService.Bot(msg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
			os.Exit(1)
		}
		adapter.Send(msg, out)
	}
}

type tuiState int

const (
	stateInput tuiState = iota
	stateWaiting
)

type nextChunkMsg struct {
	chunk channel.StreamChunk
	err   error
	done  bool
}

type model struct {
	viewport  viewport.Model
	textInput textinput.Model

	messages         []chatMessage_
	state            tuiState
	streaming        string
	streamingThought string
	err              error
	thoughtExpanded  bool

	botService service.BotService
	adapter    *cliAdapter.CLIAdapter
	renderer   *glamour.TermRenderer

	sub chan nextChunkMsg

	suggestions      []string
	suggestionIndex  int
	isAutocompleting bool
}

type chatMessage_ struct {
	role    string
	text    string
	thought string
}

var (
	userStyle = theme.BaseStyle.
			Bold(true)

	botStyle = theme.HighlightStyle.
			Bold(true)

	dimStyle = theme.MutedStyle

	headerStyle = theme.HeaderStyle

	footerStyle = theme.MutedStyle

	userBoxStyle = theme.BoxStyle.
			BorderForeground(lipgloss.Color(theme.ColorText)).
			MarginBottom(1)

	botBoxStyle = theme.BoxStyle.
			BorderForeground(lipgloss.Color(theme.ColorPrimary)).
			MarginBottom(1)

	catAscii = `
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣴⣿⣿⡷⣄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣴⣿⡿⠋⠈⠻⣮⣳⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣠⣴⣾⡿⠋⠀⠀⠀⠀⠙⣿⣿⣤⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣤⣶⣿⡿⠟⠛⠉⠀⠀⠀⠀⠀⠀⠀⠈⠛⠛⠿⠿⣿⣷⣶⣤⣄⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣴⣾⡿⠟⠋⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠉⠛⠻⠿⣿⣶⣦⣄⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⣀⣠⣤⣤⣀⡀⠀⠀⣀⣴⣿⡿⠛⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠛⠿⣿⣷⣦⣄⡀⠀⠀⠀⠀⠀⠀⠀⢀⣀⣤⣄⠀⠀
⢀⣤⣾⡿⠟⠛⠛⢿⣿⣶⣾⣿⠟⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠛⠿⣿⣷⣦⣀⣀⣤⣶⣿⡿⠿⢿⣿⡀⠀
⣿⣿⠏⠀⢰⡆⠀⠀⠉⢿⣿⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⠻⢿⡿⠟⠋⠁⠀⠀⢸⣿⠇⠀
⣿⡟⠀⣀⠈⣀⡀⠒⠃⠀⠙⣿⡆⠀⠀⠀⠀⠀⠀⠀⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⣿⠇⠀
⣿⡇⠀⠛⢠⡋⢙⡆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣾⣿⣿⠄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⣿⠀⠀
⣿⣧⠀⠀⠀⠓⠛⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠘⠛⠋⠀⠀⢸⣧⣤⣤⣶⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢰⣿⡿⠀⠀
⣿⣿⣤⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠉⠉⠻⣷⣶⣶⡆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣿⣿⠁⠀⠀
⠈⠛⠻⠿⢿⣿⣷⣶⣦⣤⣄⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣴⣿⣷⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣾⣿⡏⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠉⠙⠛⠻⠿⢿⣿⣷⣶⣦⣤⣄⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⠿⠛⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠘⢿⣿⡄⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠉⠙⠛⠻⠿⢿⣿⣷⣶⣦⣤⣄⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⢿⣿⡄⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠉⠛⠛⠿⠿⣿⣷⣶⣶⣤⣤⣀⡀⠀⠀⠀⢀⣴⡆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⢿⡿⣄
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠉⠛⠛⠿⠿⣿⣷⣶⡿⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⣿⣹
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⣿⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣀⣀⠀⠀⠀⠀⠀⠀⢸⣧
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢻⣿⣆⠀⠀⠀⠀⠀⠀⢀⣀⣠⣤⣶⣾⣿⣿⣿⣿⣤⣄⣀⡀⠀⠀⠀⣿
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠻⢿⣻⣷⣶⣾⣿⣿⡿⢯⣛⣛⡋⠁⠀⠀⠉⠙⠛⠛⠿⣿⣿⡷⣶⣿
`
)

func initialModel(botService service.BotService, adapter *cliAdapter.CLIAdapter) model {
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(80),
	)

	ti := textinput.New()
	ti.Placeholder = "Type a message to Myaaw... 🐾"
	ti.Focus()
	ti.CharLimit = 1000
	ti.Width = 80
	ti.Prompt = userStyle.Render(" 🐾 ❯ ")

	vp := viewport.New(80, 20)

	termWidth := 80
	termHeight := 20
	placeholder := lipgloss.NewStyle().
		Width(termWidth).
		Height(termHeight).
		Align(lipgloss.Center).
		AlignVertical(lipgloss.Center).
		Foreground(lipgloss.Color("240")).
		Render(catAscii)

	vp.SetContent(placeholder)

	return model{
		botService: botService,
		adapter:    adapter,
		state:      stateInput,
		renderer:   renderer,
		textInput:  ti,
		viewport:   vp,
		messages:   []chatMessage_{},
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 6 // Space for header, input, and footer
		m.textInput.Width = msg.Width - 10

		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(msg.Width-8), // Adjusted for border padding
		)
		m.updateViewportContent()
		return m, nil

	case tea.KeyMsg:
		if m.isAutocompleting && len(m.suggestions) > 0 {
			switch msg.String() {
			case "up":
				m.suggestionIndex = (m.suggestionIndex - 1 + len(m.suggestions)) % len(m.suggestions)
				return m, nil
			case "down":
				m.suggestionIndex = (m.suggestionIndex + 1) % len(m.suggestions)
				return m, nil
			case "tab", "enter":
				isDir := m.applySuggestion()
				if !isDir {
					m.isAutocompleting = false
					m.suggestions = nil
				}
				return m, nil
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter:
			if m.state != stateInput {
				return m, nil
			}

			text := strings.TrimSpace(m.textInput.Value())
			if text == "" {
				return m, nil
			}

			if text == "/exit" || text == "/quit" {
				return m, tea.Quit
			}

			processedText, images, err := parseFileAttachments(text)
			if err != nil {
				m.messages = append(m.messages, chatMessage_{role: "bot", text: fmt.Sprintf("❌ Error attachment: %v", err)})
				m.textInput.Reset()
				m.state = stateInput
				m.updateViewportContent()
				return m, nil
			}

			m.messages = append(m.messages, chatMessage_{role: "user", text: text})
			m.textInput.Reset()
			m.state = stateWaiting
			m.streaming = ""
			m.streamingThought = ""
			m.thoughtExpanded = false
			m.isAutocompleting = false
			m.suggestions = nil
			m.updateViewportContent()
			return m, m.sendMessage(processedText, images)
		}

		if msg.String() == "ctrl+t" {
			m.thoughtExpanded = !m.thoughtExpanded
			m.updateViewportContent()
			return m, nil
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.thoughtExpanded = !m.thoughtExpanded
			m.updateViewportContent()
			return m, nil
		}
	case nextChunkMsg:
		if msg.err != nil {
			m.messages = append(m.messages, chatMessage_{role: "bot", text: fmt.Sprintf("❌ Error: %v", msg.err)})
			m.state = stateInput
			m.streaming = ""
			m.streamingThought = ""
			m.updateViewportContent()
			return m, nil
		}

		if msg.done {
			m.messages = append(m.messages, chatMessage_{
				role:    "bot",
				text:    m.streaming,
				thought: m.streamingThought,
			})
			m.streaming = ""
			m.streamingThought = ""
			m.state = stateInput
			m.updateViewportContent()
			return m, nil
		}

		if msg.chunk.Thought != "" {
			// For interactive mode, we usually get deltas or cumulative.
			// But Gemini provider currently seems to emit deltas for Thought?
			// Let's be safe and handle it as cumulative if it grows, or use it as is if it's deltas.
			// Actually, if it's deltas, we just append. If it's cumulative, we need offsets.
			// To be consistent with the adapter:
			m.streamingThought = msg.chunk.Thought
		}

		if len(msg.chunk.Trace) > 0 {
			// In TUI we don't have a lastTraceLen easily accessible per message without more state.
			// However, m.streamingThought is reset per message.
			// Let's check how many tools are already in streamingThought
			m.streamingThought = msg.chunk.Thought
			for _, step := range msg.chunk.Trace {
				if step.Observation != "" {
					m.streamingThought += fmt.Sprintf("\n🛠️  Using %s...\n", step.Action)
				}
			}
		}

		if msg.chunk.Text != "" {
			m.streaming += msg.chunk.Text
		}

		m.updateViewportContent()
		return m, waitForChunk(m.sub)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown:
			m.viewport, vpCmd = m.viewport.Update(msg)
		}
	default:
		m.viewport, vpCmd = m.viewport.Update(msg)
	}

	m.textInput, tiCmd = m.textInput.Update(msg)

	// Check for autocomplete trigger
	if m.state == stateInput {
		m.updateAutocomplete()
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m *model) updateAutocomplete() {
	val := m.textInput.Value()
	cursor := m.textInput.Position()

	// Find the last '@' before the cursor
	lastAt := strings.LastIndex(val[:cursor], "@")
	if lastAt == -1 {
		m.isAutocompleting = false
		m.suggestions = nil
		return
	}

	// Only autocomplete if there's no space between '@' and cursor
	if strings.Contains(val[lastAt:cursor], " ") {
		m.isAutocompleting = false
		m.suggestions = nil
		return
	}

	m.isAutocompleting = true
	prefix := val[lastAt+1 : cursor]
	m.updateSuggestions(prefix)
}

func (m *model) updateSuggestions(prefix string) {
	dir := "."
	originalPrefix := prefix

	if strings.HasSuffix(prefix, "/") {
		dir = prefix
		prefix = ""
	} else if strings.Contains(prefix, "/") {
		dir = filepath.Dir(prefix)
		prefix = filepath.Base(prefix)
	}

	// Resolve ~ in dir
	if strings.HasPrefix(dir, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, dir[1:])
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		m.suggestions = nil
		return
	}

	var matches []string
	for _, f := range entries {
		name := f.Name()
		if f.IsDir() {
			name += "/"
		}

		// If prefix is empty (just @ or ending in /), show all (non-hidden unless prefix starts with .)
		if prefix == "" {
			if !strings.HasPrefix(name, ".") || strings.HasPrefix(originalPrefix, ".") {
				fullPath := name
				if dir != "." && dir != "./" {
					fullPath = filepath.Join(dir, name)
					if f.IsDir() && !strings.HasSuffix(fullPath, "/") {
						fullPath += "/"
					}
				}
				matches = append(matches, fullPath)
			}
			continue
		}

		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			fullPath := name
			if dir != "." && dir != "./" {
				fullPath = filepath.Join(dir, name)
				if f.IsDir() && !strings.HasSuffix(fullPath, "/") {
					fullPath += "/"
				}
			}
			matches = append(matches, fullPath)
		}
	}
	if len(matches) == 0 {
		m.suggestions = []string{"(no files found)"}
		m.suggestionIndex = 0
		return
	}
	m.suggestions = matches
	if m.suggestionIndex >= len(matches) {
		m.suggestionIndex = 0
	}
}

func (m *model) applySuggestion() bool {
	if len(m.suggestions) == 0 {
		return false
	}
	val := m.textInput.Value()
	cursor := m.textInput.Position()
	lastAt := strings.LastIndex(val[:cursor], "@")
	if lastAt == -1 {
		return false
	}

	suggestion := m.suggestions[m.suggestionIndex]
	if suggestion == "(no files found)" {
		return false
	}
	isDir := strings.HasSuffix(suggestion, "/")

	applied := suggestion
	if !isDir {
		// Wrap in quotes if it contains spaces
		if strings.Contains(applied, " ") {
			applied = "\"" + applied + "\""
		}
		applied += " "
	}

	newVal := val[:lastAt+1] + applied + val[cursor:]
	m.textInput.SetValue(newVal)
	m.textInput.SetCursor(lastAt + 1 + len(applied))

	return isDir
}

func waitForChunk(sub chan nextChunkMsg) tea.Cmd {
	return func() tea.Msg {
		return <-sub
	}
}

func (m *model) sendMessage(text string, images []string) tea.Cmd {
	m.sub = make(chan nextChunkMsg)

	go func() {
		msg := createIncomingMessage(text, images)
		if config.StreamResponse {
			_, err := m.botService.BotStream(msg, func(chunk channel.StreamChunk) {
				m.sub <- nextChunkMsg{chunk: chunk}
			})
			m.sub <- nextChunkMsg{done: true, err: err}
		} else {
			out, err := m.botService.Bot(msg)
			if err != nil {
				m.sub <- nextChunkMsg{done: true, err: err}
			} else {
				m.sub <- nextChunkMsg{chunk: channel.StreamChunk{
					Text:    out.Text,
					Thought: out.Thought,
				}}
				m.sub <- nextChunkMsg{done: true}
			}
		}
	}()

	return waitForChunk(m.sub)
}

func (m *model) updateViewportContent() {
	if len(m.messages) == 0 {
		placeholder := lipgloss.NewStyle().
			Width(m.viewport.Width).
			Height(m.viewport.Height).
			Align(lipgloss.Center).
			AlignVertical(lipgloss.Center).
			Foreground(lipgloss.Color("240")).
			Render(catAscii)
		m.viewport.SetContent(placeholder)
		return
	}

	var b strings.Builder

	for _, msg := range m.messages {
		if msg.role == "user" {
			content := userStyle.Render("❯ ") + strings.TrimSpace(msg.text)
			b.WriteString(userBoxStyle.Width(m.viewport.Width-4).Render(content) + "\n")
		} else {
			if msg.thought != "" {
				if m.thoughtExpanded {
					rendered, _ := m.renderer.Render(msg.thought)
					b.WriteString(dimStyle.Render(" ▾ Reasoning (ctrl+t to collapse)") + "\n")
					b.WriteString(theme.BoxStyle.Padding(0, 1).BorderForeground(lipgloss.Color("240")).Render(rendered) + "\n")
				} else {
					b.WriteString(dimStyle.Render(" ▸ Reasoning (ctrl+t to expand)") + "\n\n")
				}
			}

			response := msg.text
			rendered, err := m.renderer.Render(response)
			if err == nil {
				response = rendered
			}

			content := botStyle.Render("🐱 Myaaw") + "\n\n" + strings.TrimSpace(response)
			b.WriteString(botBoxStyle.Width(m.viewport.Width-4).Render(content) + "\n")
		}
	}

	if m.state == stateWaiting {
		if m.streamingThought != "" {
			if m.thoughtExpanded {
				rendered, _ := m.renderer.Render(m.streamingThought)
				b.WriteString(dimStyle.Render(" ▾ Reasoning (ctrl+t to collapse)") + "\n")
				b.WriteString(theme.BoxStyle.Padding(0, 1).BorderForeground(lipgloss.Color("240")).Render(rendered) + "\n")
			} else {
				b.WriteString(dimStyle.Render(" ▸ Reasoning (ctrl+t to expand)") + "\n\n")
			}
		}

		if m.streaming != "" {
			rendered, err := m.renderer.Render(m.streaming)
			if err == nil {
				rendered = strings.TrimSpace(rendered)
			} else {
				rendered = m.streaming
			}

			content := botStyle.Render("🐱 Myaaw") + "\n\n" + rendered + botStyle.Render(" 🐾")
			b.WriteString(botBoxStyle.Width(m.viewport.Width-4).Render(content) + "\n")
		} else {
			b.WriteString(dimStyle.Render("⏳ Myaaw is thinking... 🧶"))
		}
		b.WriteString("\n")
	}

	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}

func (m model) View() string {
	header := headerStyle.Render(" 🐾 MYAAW CLI 🐾 ")

	footerText := "  Esc/Ctrl+C: Quit • ↑/↓: Scroll Chat • @: Attach File • Ctrl+T/Click: Toggle Reasoning  "
	if m.isAutocompleting && len(m.suggestions) > 0 {
		footerText = "  Esc: Cancel • ↑/↓: Navigate • Tab/Enter: Select  "
	}
	footer := footerStyle.Render(footerText)

	var autocomplete string
	if m.isAutocompleting && len(m.suggestions) > 0 {
		var b strings.Builder

		maxItems := 10
		start := 0
		if m.suggestionIndex >= maxItems {
			start = m.suggestionIndex - maxItems + 1
		}
		end := start + maxItems
		if end > len(m.suggestions) {
			end = len(m.suggestions)
		}

		b.WriteString(theme.HighlightStyle.Render("  Suggestions (Arrow Up/Down to navigate, Tab/Enter to select):") + "\n")

		if start > 0 {
			b.WriteString(theme.MutedStyle.Render("    ↑ ... more") + "\n")
		}

		for i := start; i < end; i++ {
			s := m.suggestions[i]
			marker := "  "
			style := lipgloss.NewStyle()
			if s == "(no files found)" {
				style = theme.MutedStyle.Italic(true)
			} else if i == m.suggestionIndex {
				marker = "❯ "
				style = theme.HighlightStyle.Bold(true)
			}
			b.WriteString(fmt.Sprintf("%s%s\n", marker, style.Render(s)))
		}

		if end < len(m.suggestions) {
			b.WriteString(theme.MutedStyle.Render("    ↓ ... more") + "\n")
		}

		autocomplete = b.String()
	}

	inputView := m.textInput.View()
	if autocomplete != "" {
		// Place autocomplete above footer, but separated
		return fmt.Sprintf(
			"%s\n\n%s\n\n%s\n%s\n%s",
			header,
			m.viewport.View(),
			autocomplete,
			inputView,
			footer,
		)
	}

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n\n%s",
		header,
		m.viewport.View(),
		inputView,
		footer,
	)
}

func runInteractive(botService service.BotService, adapter *cliAdapter.CLIAdapter) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Error getting home directory:", err)
	}

	logDir := filepath.Join(homeDir, ".myaaw", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatal("Error creating log directory:", err)
	}

	logPath := filepath.Join(logDir, "myaaw-chat-cli.log")
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening log file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	p := tea.NewProgram(initialModel(botService, adapter), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		log.Fatal("Error running TUI:", err)
	}
}

func parseFileAttachments(text string) (string, []string, error) {
	// 1. Collect potential paths from @ tags
	reAt := regexp.MustCompile(`@(?:"([^"]+)"|([^\s"]+))`)
	matchesAt := reAt.FindAllStringSubmatch(text, -1)

	// 2. Collect potential paths from absolute/home paths (common in drag-and-drop)
	// This matches strings starting with / or ~ and continues until a space (not preceded by \) or end of string.
	rePath := regexp.MustCompile(`(?:^|\s)(/[^\s\\]*(?:\\.[^\s\\]*)*|~[^\s\\]*(?:\\.[^\s\\]*)*)`)
	matchesPath := rePath.FindAllStringSubmatch(text, -1)

	var images []string
	processedText := text

	// Process @ matches
	for _, match := range matchesAt {
		original := match[0]
		path := match[1]
		if path == "" {
			path = match[2]
		}
		processedText, images, _ = processPath(processedText, original, path, images)
	}

	// Process absolute path matches
	for _, match := range matchesPath {
		original := strings.TrimSpace(match[1])
		// Unescape spaces: "\ " -> " "
		path := strings.ReplaceAll(original, `\ `, " ")

		// Check if it's a valid existing file
		tempPath := path
		if strings.HasPrefix(tempPath, "~") {
			home, _ := os.UserHomeDir()
			tempPath = filepath.Join(home, tempPath[1:])
		}
		if _, err := os.Stat(tempPath); err == nil {
			processedText, images, _ = processPath(processedText, match[1], path, images)
		}
	}

	return processedText, images, nil
}

func processPath(text, original, path string, images []string) (string, []string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[1:])
		}
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	markdownLink := fmt.Sprintf("[%s](file://%s)", filepath.Base(absPath), absPath)
	text = strings.Replace(text, original, markdownLink, 1)

	ext := strings.ToLower(filepath.Ext(path))
	if isImageExtension(ext) {
		data, err := os.ReadFile(path)
		if err == nil {
			base64Data := base64.StdEncoding.EncodeToString(data)
			images = append(images, base64Data)
		}
	}

	return text, images, nil
}

func isImageExtension(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	}
	return false
}
