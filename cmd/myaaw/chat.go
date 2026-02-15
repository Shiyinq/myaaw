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
			Foreground(lipgloss.Color("6")).
			Bold(true)

	botStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
)

func initialModel(botService service.BotService, adapter *cliAdapter.CLIAdapter) model {
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(80),
	)

	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Focus()
	ti.CharLimit = 1000
	ti.Width = 80

	vp := viewport.New(80, 20)
	vp.SetContent("🤖 Welcome to Myaaw! Type a message to start chatting.\n\n")

	return model{
		botService: botService,
		adapter:    adapter,
		state:      stateInput,
		renderer:   renderer,
		textInput:  ti,
		viewport:   vp,
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
		m.viewport.Height = msg.Height - 4 // Leave space for input and header
		m.textInput.Width = msg.Width

		// Update renderer width
		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(msg.Width-2),
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
			m.streaming += fmt.Sprintf("\n🛠️  Using %s...\n", msg.chunk.ToolCalls[0].Function.Name)
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

	// Handle Text Input
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
	var b strings.Builder

	// Header
	b.WriteString(botStyle.Render("🤖 Myaaw Interactive Chat") + "\n")
	b.WriteString(dimStyle.Render("Type /exit to quit • Esc to quit • ↑/↓/PgUp/PgDn to scroll") + "\n\n")

	for _, msg := range m.messages {
		if msg.role == "user" {
			b.WriteString(userStyle.Render("You: ") + msg.text + "\n\n")
		} else {
			response := msg.text
			rendered, err := m.renderer.Render(response)
			if err == nil {
				response = rendered
			}
			b.WriteString(botStyle.Render("Myaaw: ") + "\n" + response + "\n")
		}
	}

	if m.state == stateWaiting {
		b.WriteString(botStyle.Render("Myaaw: ") + "\n")
		if m.streaming != "" {
			rendered, err := m.renderer.Render(m.streaming)
			if err == nil {
				b.WriteString(rendered)
			} else {
				b.WriteString(m.streaming)
			}
			b.WriteString(dimStyle.Render(" ✨"))
		} else {
			b.WriteString(dimStyle.Render("⏳ Thinking..."))
		}
		b.WriteString("\n")
	}

	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}

func (m model) View() string {
	return fmt.Sprintf(
		"%s\n%s",
		m.viewport.View(),
		m.textInput.View(),
	)
}

func runInteractive(botService service.BotService, adapter *cliAdapter.CLIAdapter) {
	// Redirect logs to file
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
