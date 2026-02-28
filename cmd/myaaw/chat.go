package main

import (
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
			m.streamingThought = ""
			m.thoughtExpanded = false
			m.updateViewportContent()
			return m, m.sendMessage(text)
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
	footer := footerStyle.Render("  Esc/Ctrl+C: Quit • ↑/↓: Scroll • Ctrl+T/Click: Toggle Reasoning • Type /exit to leave  ")

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n%s",
		header,
		m.viewport.View(),
		m.textInput.View(),
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
