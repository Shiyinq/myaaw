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

// nextChunkMsg carries a piece of streamed data or completion signal
type nextChunkMsg struct {
	chunk channel.StreamChunk
	err   error
	done  bool
}

type model struct {
	input      string
	messages   []chatMessage_
	state      tuiState
	streaming  string
	width      int
	quitting   bool
	botService service.BotService
	adapter    *cliAdapter.CLIAdapter
	renderer   *glamour.TermRenderer

	// Channel for receiving stream chunks from the goroutine
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

	toolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")).
			Italic(true)
)

func initialModel(botService service.BotService, adapter *cliAdapter.CLIAdapter) model {
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)

	return model{
		botService: botService,
		adapter:    adapter,
		state:      stateInput,
		renderer:   renderer,
		width:      80,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			if m.state != stateInput {
				return m, nil
			}

			text := strings.TrimSpace(m.input)
			if text == "" {
				return m, nil
			}

			if text == "/exit" || text == "/quit" || text == "exit" || text == "quit" {
				m.quitting = true
				return m, tea.Quit
			}

			m.messages = append(m.messages, chatMessage_{role: "user", text: text})
			m.input = ""
			m.state = stateWaiting
			m.streaming = ""

			return m, m.sendMessage(text)

		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil

		case tea.KeySpace:
			if m.state == stateInput {
				m.input += " "
			}
			return m, nil

		case tea.KeyRunes:
			if m.state == stateInput {
				m.input += string(msg.Runes)
			}
			return m, nil
		}

	case nextChunkMsg:
		if msg.err != nil {
			m.messages = append(m.messages, chatMessage_{role: "bot", text: fmt.Sprintf("❌ Error: %v", msg.err)})
			m.state = stateInput
			m.streaming = ""
			return m, nil
		}

		if msg.done {
			// Stream finished
			m.messages = append(m.messages, chatMessage_{role: "bot", text: m.streaming})
			m.streaming = ""
			m.state = stateInput
			return m, nil
		}

		// Process chunk
		if len(msg.chunk.ToolCalls) > 0 {
			m.streaming += fmt.Sprintf("\n🛠️  Using %s...\n", msg.chunk.ToolCalls[0].Function.Name)
		} else if msg.chunk.Text != "" {
			m.streaming += msg.chunk.Text
		}

		// Continue listening for next chunk
		return m, waitForChunk(m.sub)
	}

	return m, nil
}

// waitForChunk returns a Cmd that waits for the next value from the channel
func waitForChunk(sub chan nextChunkMsg) tea.Cmd {
	return func() tea.Msg {
		return <-sub
	}
}

func (m *model) sendMessage(text string) tea.Cmd {
	m.sub = make(chan nextChunkMsg)

	// Start streaming in a goroutine
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

	// Wait for the first chunk
	return waitForChunk(m.sub)
}

func (m model) View() string {
	if m.quitting {
		return "\n👋 Goodbye!\n"
	}

	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(botStyle.Render("🤖 Myaaw Interactive Chat"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Type /exit to quit, Ctrl+C to cancel"))
	b.WriteString("\n")
	b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	for _, msg := range m.messages {
		if msg.role == "user" {
			b.WriteString(userStyle.Render("You: "))
			b.WriteString(msg.text)
			b.WriteString("\n\n")
		} else {
			b.WriteString(botStyle.Render("Myaaw: "))

			rendered, err := m.renderer.Render(msg.text)
			if err != nil {
				b.WriteString(msg.text)
			} else {
				b.WriteString(rendered)
			}
			b.WriteString("\n")
		}
	}

	if m.state == stateWaiting {
		if m.streaming != "" {
			b.WriteString(botStyle.Render("Myaaw: "))

			// Render partial streaming markdown if possible, otherwise plain text
			rendered, err := m.renderer.Render(m.streaming)
			if err != nil {
				b.WriteString(m.streaming)
			} else {
				b.WriteString(rendered)
			}

			b.WriteString(dimStyle.Render(" ✨"))
			b.WriteString("\n")
		} else {
			b.WriteString(dimStyle.Render("⏳ Thinking..."))
			b.WriteString("\n")
		}
	}

	if m.state == stateInput {
		b.WriteString(userStyle.Render("You: "))
		b.WriteString(m.input)
		b.WriteString(dimStyle.Render("█"))
		b.WriteString("\n")
	}

	return b.String()
}

func runInteractive(botService service.BotService, adapter *cliAdapter.CLIAdapter) {
	// Redirect logs to file to avoid messing up TUI
	f, err := os.OpenFile("myaaw-chat.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	p := tea.NewProgram(initialModel(botService, adapter), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatal("Error running TUI:", err)
	}
}
