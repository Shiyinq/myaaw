package main

import (
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
	cliAdapter "myaaw/internal/channel/cli"
	"myaaw/internal/config"
	"myaaw/internal/services/bot/repository"
	"myaaw/internal/services/bot/service"
	"os"
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
			runOneShot(botService, adapter, chatMessage)
		} else {
			runInteractive(botService, adapter)
		}
	},
}

func init() {
	chatCmd.Flags().StringVarP(&chatMessage, "message", "m", "", "Send a one-shot message (non-interactive)")
}

func createBotService() service.BotService {
	userRepo := repository.NewUserRepository(config.DB, config.RedisClient)
	convRepo := repository.NewConversationRepository(config.DB, config.RedisClient)
	return service.NewBotService(userRepo, convRepo)
}

func createIncomingMessage(text string) *channel.IncomingMessage {
	payload, _ := json.Marshal(map[string]interface{}{
		"user_id": 0,
		"text":    text,
	})
	adapter := cliAdapter.NewCLIAdapter()
	msg, _ := adapter.ParseIncoming(payload)
	return msg
}

// --- One-Shot Mode ---

func runOneShot(botService service.BotService, adapter *cliAdapter.CLIAdapter, message string) {
	msg := createIncomingMessage(message)

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

// --- Interactive TUI Mode ---

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
	// Components
	viewport  viewport.Model
	textInput textinput.Model

	// State
	messages  []chatMessage_
	state     tuiState
	streaming string
	err       error

	// External Services
	botService service.BotService
	adapter    *cliAdapter.CLIAdapter
	renderer   *glamour.TermRenderer

	// Communication
	sub chan nextChunkMsg
}

type chatMessage_ struct {
	role string
	text string
}

var (
	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")). // White (Cat's white fur)
			Bold(true)

	botStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF9F69")). // Peach/Orange (Cat fur)
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#FF9F69")). // Peach background
			Bold(true).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)

	userBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			MarginBottom(1)

	botBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF9F69")).
			Padding(0, 1).
			MarginBottom(1)

	// Custom ASCII Art for Center Screen (Empty State)
	// source https://emojicombos.com/cat
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

	// Create centered cat placeholder for empty state
	// Note: We use Place() to center it in the viewport
	termWidth := 80
	termHeight := 20
	// Use lipgloss to center the ascii art
	placeholder := lipgloss.NewStyle().
		Width(termWidth).
		Height(termHeight).
		Align(lipgloss.Center).
		AlignVertical(lipgloss.Center).
		Foreground(lipgloss.Color("240")). // Dim color for watermark
		Render(catAscii)

	vp.SetContent(placeholder)

	return model{
		botService: botService,
		adapter:    adapter,
		state:      stateInput,
		renderer:   renderer,
		textInput:  ti,
		viewport:   vp,
		messages:   []chatMessage_{}, // Empty messages initially (no welcome text)
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

			m.messages = append(m.messages, chatMessage_{role: "user", text: text})
			m.textInput.Reset()
			m.state = stateWaiting
			m.streaming = ""
			m.updateViewportContent()
			return m, m.sendMessage(text)
		}

	case nextChunkMsg:
		if msg.err != nil {
			m.messages = append(m.messages, chatMessage_{role: "bot", text: fmt.Sprintf("❌ Error: %v", msg.err)})
			m.state = stateInput
			m.streaming = ""
			m.updateViewportContent()
			return m, nil
		}

		if msg.done {
			m.messages = append(m.messages, chatMessage_{role: "bot", text: m.streaming})
			m.streaming = ""
			m.state = stateInput
			m.updateViewportContent()
			return m, nil
		}

		if len(msg.chunk.ToolCalls) > 0 {
			m.streaming += fmt.Sprintf("\n🛠️  *Using %s...*\n", msg.chunk.ToolCalls[0].Function.Name)
		} else if msg.chunk.Text != "" {
			m.streaming += msg.chunk.Text
		}

		m.updateViewportContent()
		return m, waitForChunk(m.sub)
	}

	// Handle Viewport inputs (scrolling)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown:
			m.viewport, vpCmd = m.viewport.Update(msg)
		}
	default:
		// Pass other messages (like mouse events or window resize) to viewport
		m.viewport, vpCmd = m.viewport.Update(msg)
	}

	m.textInput, tiCmd = m.textInput.Update(msg)

	return m, tea.Batch(tiCmd, vpCmd)
}

func waitForChunk(sub chan nextChunkMsg) tea.Cmd {
	return func() tea.Msg {
		return <-sub
	}
}

func (m *model) sendMessage(text string) tea.Cmd {
	m.sub = make(chan nextChunkMsg)

	go func() {
		msg := createIncomingMessage(text)
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
				m.sub <- nextChunkMsg{chunk: channel.StreamChunk{Text: out.Text}}
				m.sub <- nextChunkMsg{done: true}
			}
		}
	}()

	return waitForChunk(m.sub)
}

func (m *model) updateViewportContent() {
	// If no messages, render the watermark/placeholder
	if len(m.messages) == 0 {
		placeholder := lipgloss.NewStyle().
			Width(m.viewport.Width).
			Height(m.viewport.Height).
			Align(lipgloss.Center).
			AlignVertical(lipgloss.Center).
			Foreground(lipgloss.Color("240")). // Dim color
			Render(catAscii)
		m.viewport.SetContent(placeholder)
		return
	}

	var b strings.Builder

	for _, msg := range m.messages {
		if msg.role == "user" {
			// User message: Icon and text on same line, no "You" text
			content := userStyle.Render("❯ ") + strings.TrimSpace(msg.text)
			b.WriteString(userBoxStyle.Width(m.viewport.Width-4).Render(content) + "\n")
		} else {
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
	footer := footerStyle.Render("  Esc/Ctrl+C: Quit • ↑/↓: Scroll • Type /exit to leave  ")

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n%s",
		header,
		m.viewport.View(),
		m.textInput.View(),
		footer,
	)
}

func runInteractive(botService service.BotService, adapter *cliAdapter.CLIAdapter) {
	f, err := os.OpenFile("myaaw-chat.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	p := tea.NewProgram(initialModel(botService, adapter), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		log.Fatal("Error running TUI:", err)
	}
}
