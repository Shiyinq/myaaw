package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"myaaw/internal/agent"
	"myaaw/internal/agent/tools/filesystem"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	cliAdapter "myaaw/internal/channel/cli"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	botModel "myaaw/internal/services/bot/model"
	"myaaw/internal/services/bot/repository"
	"myaaw/internal/services/bot/service"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var chatMessage string

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Chat with your AI assistant",
	Long:  "Start an interactive TUI chat session, or send a one-shot message with -m flag.",
	Run: func(cmd *cobra.Command, args []string) {
		if agentFlag == "" && chatMessage == "" {
			if err := chooseAgentForChat(); err != nil {
				fmt.Fprintf(os.Stderr, "❌ %v\n", err)
				os.Exit(1)
			}
		}

		startAgentBackgroundLoaders()
		config.LoadBaseConfig()
		config.ConnectDatabases()

		botService, convRepo := createBotService()
		adapter := cliAdapter.NewCLIAdapter()

		if chatMessage != "" {
			text, images, err := parseFileAttachments(chatMessage)
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ Error parsing attachments: %v\n", err)
				os.Exit(1)
			}
			runOneShot(botService, adapter, text, images)
		} else {
			runInteractive(botService, convRepo, adapter)
		}
	},
}

func init() {
	chatCmd.Flags().StringVarP(&chatMessage, "message", "m", "", "Send a one-shot message (non-interactive)")
}

func chooseAgentForChat() error {
	names, err := agentdir.ListAgents()
	if err != nil || len(names) <= 1 {
		return nil
	}

	registry, _ := agentdir.LoadRegistry()
	active := agentdir.Active()

	options := make([]huh.Option[string], 0, len(names))
	for _, name := range names {
		label := name
		if registry != nil && name == registry.Default {
			label += " (default)"
		}
		if name == active {
			label += " (current)"
		}
		options = append(options, huh.NewOption(label, name))
	}

	selected := active
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select an agent").
				Options(options...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return err
	}

	if err := agentdir.ValidateName(selected); err != nil {
		return err
	}
	if !agentdir.AgentInitialized(selected) {
		return fmt.Errorf("agent %q does not exist", selected)
	}
	if err := agentdir.SetActive(selected); err != nil {
		return err
	}

	cwd, _ := os.Getwd()
	if err := filesystem.ConfigureForActiveAgent(cwd); err != nil {
		return err
	}
	if _, err := agent.EnsureWorkspace(); err != nil {
		return err
	}
	if cwd != "" {
		if err := filesystem.SetProjectDirectory(cwd); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
		}
	}

	return nil
}

func createBotService() (service.BotService, repository.ConversationRepository) {
	userRepo := repository.NewUserRepository(config.DB)
	convRepo := repository.NewConversationRepository(config.DB)
	return service.NewBotService(userRepo, convRepo), convRepo
}

func createIncomingMessage(text string, images []string, convID string) *channel.IncomingMessage {
	payload, _ := json.Marshal(map[string]interface{}{
		"user_id":         0,
		"text":            text,
		"conversation_id": convID,
	})
	adapter := cliAdapter.NewCLIAdapter()
	msg, _ := adapter.ParseIncoming(payload)
	msg.Images = images
	msg.ConversationID = convID
	return msg
}

func runOneShot(botService service.BotService, adapter *cliAdapter.CLIAdapter, message string, images []string) {
	msg := createIncomingMessage(message, images, "")

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
	stateStopping
	stateSessionSelect
	stateModelSelect
	stateModelLoading
)

type nextChunkMsg struct {
	chunk     channel.StreamChunk
	done      bool
	err       error
	newConvID string
	finalText string
	trace     []provider.ReactStep
}

type stopAckMsg struct {
	err error
}

type sessionsLoadedMsg struct {
	conversations []*botModel.Conversation
	err           error
}

func loadSessionsCmd(convRepo repository.ConversationRepository) tea.Cmd {
	return func() tea.Msg {
		convs, err := convRepo.GetConversationByUserId(0)
		return sessionsLoadedMsg{
			conversations: convs,
			err:           err,
		}
	}
}

type modelsLoadedMsg struct {
	models []string
}

func loadModelsCmd() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.LoadJSONConfigOnly()
		var modelsList []string

		if err == nil && cfg != nil && cfg.Providers != nil {
			type modelInfo struct {
				id        string
				provType  string
				modelName string
			}

			var allFetched []modelInfo

			// Query each provider in parallel
			type result struct {
				id       string
				provType string
				models   []string
			}
			resultChan := make(chan result, len(cfg.Providers))

			for id, p := range cfg.Providers {
				go func(provID, provType, apiKey, baseURL string) {
					factory, exists := provider.LLMproviderFactories[provType]
					if exists {
						llmProv := factory(baseURL, apiKey, p.DefaultModel)
						provModels, mErr := llmProv.Models()
						if mErr == nil {
							resultChan <- result{id: provID, provType: provType, models: provModels}
							return
						}
					}
					// fallback to default model if api fails
					resultChan <- result{id: provID, provType: provType, models: []string{p.DefaultModel}}
				}(id, p.Type, p.APIKey, p.BaseURL)
			}

			// Collect results
			for i := 0; i < len(cfg.Providers); i++ {
				res := <-resultChan
				for _, m := range res.models {
					allFetched = append(allFetched, modelInfo{
						id:        res.id,
						provType:  res.provType,
						modelName: m,
					})
				}
			}

			// Format strings
			var others []string
			var active string
			activeID := config.CurrentProviderID
			activeModel := config.LLMDefaultModel

			for _, info := range allFetched {
				line := fmt.Sprintf("%s (%s: %s)", info.id, info.provType, info.modelName)
				if info.id == activeID && info.modelName == activeModel {
					active = "★ " + line
				} else {
					others = append(others, "  "+line)
				}
			}

			sort.Strings(others)

			if active != "" {
				modelsList = append([]string{active}, others...)
			} else {
				modelsList = others
			}
		}

		if len(modelsList) == 0 {
			modelsList = []string{"  No models found"}
		}

		return modelsLoadedMsg{models: modelsList}
	}
}

type model struct {
	viewport  viewport.Model
	textInput textinput.Model

	messages        []chatMessage_
	state           tuiState
	streamingItems  []timelineItem
	clickTargets    []timelineClickTarget
	err             error
	thoughtExpanded bool

	streamStartedAt        time.Time
	streamingGroupExpanded bool
	stopAck                chan error
	interruptedIndex       int

	botService service.BotService
	convRepo   repository.ConversationRepository
	adapter    *cliAdapter.CLIAdapter
	renderer   *glamour.TermRenderer

	conversations []*botModel.Conversation
	sessionCursor int
	activeConvID  string

	sub chan nextChunkMsg

	suggestions      []string
	suggestionIndex  int
	isAutocompleting bool

	queueFileSize int64
	width         int
	height        int

	pawFrame          int
	cancelFunc        context.CancelFunc
	userScrolledUp    bool
	lastHistoryScroll time.Time
	totalTokens       int

	availableModels []string
	modelCursor     int
}

type pawTickMsg struct{}

func pawTickCmd() tea.Cmd {
	return tea.Tick(180*time.Millisecond, func(t time.Time) tea.Msg {
		return pawTickMsg{}
	})
}

var pawFrames = []string{
	"🐾    ❯ ",
	" 🐾   ❯ ",
	"  🐾  ❯ ",
	"   🐾 ❯ ",
	"    🐾❯ ",
	"   🐾 ❯ ",
	"  🐾  ❯ ",
	" 🐾   ❯ ",
}

type queueMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text"`
	Thought   string    `json:"thought"`
}

type queueTickMsg time.Time

func queueTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return queueTickMsg(t)
	})
}

type chatMessage_ struct {
	role            string
	text            string
	thought         string
	items           []timelineItem
	groupExpanded   bool
	duration        time.Duration
	interrupted     bool
	renderedText    string
	renderedThought string

	cachedView      string
	cachedWidth     int
	cachedExpanded  bool
	cachedSpans     []timelineLineSpan
	cachedGroupSpan timelineLineSpan
}

type timelineClickTarget struct {
	StartLine    int
	EndLine      int
	MessageIndex int // -1 means the live streaming message
	ItemIndex    int
	Group        bool
}

func convertProviderMessages(providerMsgs []provider.Message) []chatMessage_ {
	providerMsgs = agent.MigrateLegacyTranscript(providerMsgs)

	var chatMsgs []chatMessage_
	var current *chatMessage_
	toolIndex := map[string]int{}

	flush := func() {
		if current != nil && len(current.items) > 0 {
			chatMsgs = append(chatMsgs, *current)
		}
		current = nil
		toolIndex = map[string]int{}
	}

	for _, msg := range providerMsgs {
		switch msg.Role {
		case "user":
			flush()
			text := provider.MessageText(msg)
			if text == "" && len(msg.Images) == 0 {
				continue
			}
			chatMsgs = append(chatMsgs, chatMessage_{role: "user", text: text})

		case "assistant":
			if current == nil {
				current = &chatMessage_{role: "bot"}
			}
			if msg.Thought != "" {
				current.items = appendThinkingItem(current.items, msg.Thought)
			}
			if text := provider.MessageText(msg); text != "" {
				current.items = appendTextItem(current.items, text)
			}
			for _, call := range msg.ToolCalls {
				id := call.ID
				if id == "" {
					id = fmt.Sprintf("history-%d-%d", len(chatMsgs), len(current.items))
				}
				current.items = append(current.items, timelineItem{
					Kind:       "tool",
					ToolCallID: id,
					ToolName:   call.Function.Name,
					ToolArgs:   toolArgsToString(call.Function.Arguments),
					Running:    true,
				})
				toolIndex[id] = len(current.items) - 1
			}

		case "tool":
			if current == nil {
				current = &chatMessage_{role: "bot"}
			}
			result := provider.MessageText(msg)
			if idx, ok := toolIndex[msg.ToolCallID]; ok {
				current.items[idx].ToolResult = result
				current.items[idx].IsError = msg.IsError
				current.items[idx].Running = false
			} else {
				current.items = append(current.items, timelineItem{
					Kind:       "tool",
					ToolCallID: msg.ToolCallID,
					ToolName:   msg.Name,
					ToolResult: result,
					IsError:    msg.IsError,
				})
			}
		}
	}
	flush()

	return chatMsgs
}

var mouseFragmentPattern = regexp.MustCompile(`\[?<[0-9]+(;[0-9]+)*[Mm]`)

func looksLikeMouseEscape(value string) bool {
	if strings.Contains(value, "[<") {
		return true
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || (!strings.HasSuffix(trimmed, "M") && !strings.HasSuffix(trimmed, "m")) {
		return false
	}
	trimmed = strings.TrimSuffix(strings.TrimSuffix(trimmed, "M"), "m")
	if !strings.Contains(trimmed, ";") {
		return false
	}
	for _, r := range trimmed {
		if r != ';' && r != '<' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func toolArgsToString(value interface{}) string {
	if value == nil {
		return "{}"
	}
	if text, ok := value.(string); ok {
		if text == "" {
			return "{}"
		}
		return text
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
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

	sessionBoxStyle = theme.BoxStyle.
			BorderForeground(lipgloss.Color(theme.ColorPrimary)).
			Padding(1, 2)

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
⠀⠀⠀⠀⠀⠀⠀⠉⠙⠛⠻⠿⢿⣿⣷⣶⣦⣤⣄⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⠿⠛⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀☘⢿⣿⡄⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠉⠙⠛⠻⠿⢿⣿⣷⣶⣦⣤⣄⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⢿⣿⡄⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠉⠛⠛⠿⠿⣿⣷⣶⣶⣤⣤⣀⡀⠀⠀⠀⢀⣴⡆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⢿⡿⣄
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠉⠛⠛⠿⠿⣿⣷⣶⡿⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⣿⣹
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⣿⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣀⣀⠀⠀⠀⠀⠀⠀⢸⣧
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢻⣿⣆⠀⠀⠀⠀⠀⠀⢀⣀⣠⣤⣶⣾⣿⣿⣿⣿⣤⣄⣀⡀⠀⠀⠀⣿
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠻⢿⣻⣷⣶⣾⣿⣿⡿⢯⣛⣛⡋⠁⠀⠀⠉⠙⠛⠛⠿⣿⣿⡷⣶⣿
`
)

func initialModel(botService service.BotService, convRepo repository.ConversationRepository, adapter *cliAdapter.CLIAdapter) model {
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

	logDir, err := agentdir.ActiveLogsDir()
	var initialSize int64
	if err == nil {
		queuePath := filepath.Join(logDir, "cli_queue.jsonl")
		if stat, statErr := os.Stat(queuePath); statErr == nil {
			initialSize = stat.Size()
		}
	}

	// Session will be created lazily by the service layer on first message send
	return model{
		botService:    botService,
		convRepo:      convRepo,
		adapter:       adapter,
		state:         stateInput,
		renderer:      renderer,
		textInput:     ti,
		viewport:      vp,
		messages:      []chatMessage_{},
		conversations: []*botModel.Conversation{},
		sessionCursor: 0,
		activeConvID:  "NEW",
		queueFileSize: initialSize,
		width:         80,
		height:        24,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, queueTickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
	)

	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		if msg.err == nil {
			m.conversations = msg.conversations
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Prevent terminal right-margin ANSI wrap bug which desyncs bubble tea layout
		m.viewport.Width = msg.Width - 1
		m.viewport.Height = msg.Height - 6 // Space for header, input, and footer
		m.textInput.Width = msg.Width - 10

		customStyle := *styles.DefaultStyles["dark"]
		customStyle.H2.Prefix = ""
		customStyle.H3.Prefix = ""
		customStyle.H4.Prefix = ""
		customStyle.H5.Prefix = ""
		customStyle.H6.Prefix = ""

		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithStyles(customStyle),
			glamour.WithWordWrap(msg.Width-10),
		)
		m.updateViewportContent(false)
		return m, nil

	case tea.MouseMsg:
		// Only the chat view has clickable timeline items.
		if m.state == stateSessionSelect || m.state == stateModelSelect || m.state == stateModelLoading {
			return m, nil
		}
		// Handle mouse scrolling & clicks exclusively so escape sequences never leak into textinput
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.viewport.LineUp(3)
			m.userScrolledUp = true
			m.lastHistoryScroll = time.Now()
			return m, nil
		case tea.MouseButtonWheelDown:
			m.viewport.LineDown(3)
			if m.viewport.AtBottom() {
				m.userScrolledUp = false
			}
			m.lastHistoryScroll = time.Now()
			return m, nil
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionPress {
				const viewportTop = 2 // header + blank line
				if msg.Y >= viewportTop && msg.Y < viewportTop+m.viewport.Height {
					contentY := msg.Y - viewportTop + m.viewport.YOffset
					for _, target := range m.clickTargets {
						if contentY >= target.StartLine && contentY < target.EndLine {
							if target.Group {
								m.toggleTimelineGroup(target.MessageIndex)
							} else {
								m.toggleTimelineItem(target.MessageIndex, target.ItemIndex)
							}
							m.updateViewportContent(false)
							return m, nil
						}
					}
				}
				return m, nil
			}
		}
		return m, nil

	case modelsLoadedMsg:
		m.availableModels = msg.models
		m.state = stateModelSelect
		m.modelCursor = 0
		m.updateViewportContent(true)
		return m, nil

	case tea.KeyMsg:
		// Some terminals do not decode SGR mouse reports correctly; drop the
		// raw escape sequence instead of letting it leak into the text input.
		if looksLikeMouseEscape(msg.String()) {
			return m, nil
		}

		// 1. Session Picker State Handling
		if m.state == stateSessionSelect {
			totalItems := len(m.conversations) + 1 // 0 is "+ Create New Session"
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit

			case "esc", "q":
				// Return to chat view
				m.state = stateInput
				m.textInput.Focus()
				return m, nil

			case "up", "k":
				m.sessionCursor = (m.sessionCursor - 1 + totalItems) % totalItems
				return m, nil

			case "down", "j":
				m.sessionCursor = (m.sessionCursor + 1) % totalItems
				return m, nil

			case "enter":
				if m.sessionCursor == 0 {
					// Create New Session
					newConv, err := m.convRepo.CreateConversation(0, "New Chat")
					if err == nil && newConv != nil {
						m.activeConvID = newConv.Id
					} else {
						m.activeConvID = ""
					}
					m.messages = []chatMessage_{}
					m.totalTokens = 0
					m.state = stateInput
					m.textInput.Focus()
					m.updateViewportContent(true)
					return m, nil
				}

				// Select Existing Session
				selectedIdx := m.sessionCursor - 1
				if selectedIdx >= 0 && selectedIdx < len(m.conversations) {
					selected := m.conversations[selectedIdx]
					m.activeConvID = selected.Id
					m.messages = convertProviderMessages(selected.Messages)

					// Restore token usage from history
					m.totalTokens = 0
					for i := len(selected.Messages) - 1; i >= 0; i-- {
						if selected.Messages[i].Usage.TotalTokens > 0 {
							m.totalTokens = selected.Messages[i].Usage.TotalTokens
							break
						}
					}

					m.state = stateInput
					m.textInput.Focus()
					m.updateViewportContent(true)
					m.viewport.GotoBottom()
				}
				return m, nil

			case "d", "x", "delete", "backspace":
				if m.sessionCursor > 0 {
					selectedIdx := m.sessionCursor - 1
					if selectedIdx >= 0 && selectedIdx < len(m.conversations) {
						target := m.conversations[selectedIdx]
						_ = m.convRepo.DeleteConversationById(target.Id)
						if m.activeConvID == target.Id {
							m.activeConvID = ""
							m.messages = []chatMessage_{}
						}
						if m.sessionCursor >= len(m.conversations) {
							m.sessionCursor = len(m.conversations) - 1
							if m.sessionCursor < 0 {
								m.sessionCursor = 0
							}
						}
						return m, loadSessionsCmd(m.convRepo)
					}
				}
				return m, nil
			}
			return m, nil
		}

		if m.state == stateModelSelect {
			totalItems := len(m.availableModels)
			if totalItems == 0 {
				totalItems = 1
			}
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit

			case "esc", "q":
				m.state = stateInput
				m.textInput.Focus()
				return m, nil

			case "up", "k":
				m.modelCursor = (m.modelCursor - 1 + totalItems) % totalItems
				return m, nil

			case "down", "j":
				m.modelCursor = (m.modelCursor + 1) % totalItems
				return m, nil

			case "enter":
				if len(m.availableModels) > 0 && m.modelCursor < len(m.availableModels) && !strings.HasPrefix(m.availableModels[0], "No models") {
					selectedLine := m.availableModels[m.modelCursor]
					selectedLine = strings.TrimPrefix(selectedLine, "★ ")
					selectedLine = strings.TrimSpace(selectedLine)

					parts := strings.SplitN(selectedLine, " (", 2)
					if len(parts) == 2 {
						selectedID := parts[0]
						modelPart := strings.TrimSuffix(parts[1], ")")
						typeModelParts := strings.SplitN(modelPart, ": ", 2)
						if len(typeModelParts) == 2 {
							modelName := typeModelParts[1]

							cfg, err := config.LoadJSONConfigOnly()
							if err == nil && cfg != nil && cfg.Providers != nil {
								if p, exists := cfg.Providers[selectedID]; exists {
									cfg.DefaultProvider = selectedID
									p.DefaultModel = modelName
									cfg.Providers[selectedID] = p
									_ = config.SaveConfig(cfg)

									config.CurrentProviderID = selectedID
									config.LLMProviderName = p.Type
									config.LLMDefaultModel = modelName
									config.LLMProviderAPIKey = p.APIKey
									config.LLMProviderBaseURL = p.BaseURL
								}
							}
						}
					}
				}
				m.state = stateInput
				m.textInput.Focus()
				return m, nil
			}
			return m, nil
		}

		// 2. Chat / Input State Handling
		if m.isAutocompleting && len(m.suggestions) > 0 {
			switch msg.String() {
			case "up":
				if time.Since(m.lastHistoryScroll) < 200*time.Millisecond {
					m.lastHistoryScroll = time.Now()
					m.viewport.LineUp(2)
					m.userScrolledUp = true
					return m, nil
				}
				m.suggestionIndex = (m.suggestionIndex - 1 + len(m.suggestions)) % len(m.suggestions)
				return m, nil
			case "down":
				if time.Since(m.lastHistoryScroll) < 200*time.Millisecond {
					m.lastHistoryScroll = time.Now()
					m.viewport.LineDown(2)
					if m.viewport.AtBottom() {
						m.userScrolledUp = false
					}
					return m, nil
				}
				m.suggestionIndex = (m.suggestionIndex + 1) % len(m.suggestions)
				return m, nil
			case "tab":
				isDir := m.applySuggestion()
				if !isDir {
					m.isAutocompleting = false
					m.suggestions = nil
				}
				return m, nil
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyEsc:
			if m.state == stateWaiting {
				if m.cancelFunc != nil {
					m.cancelFunc()
					m.cancelFunc = nil
				}
				duration := m.streamElapsed()
				if len(m.streamingItems) > 0 {
					items := append([]timelineItem(nil), m.streamingItems...)
					items = append(items, timelineItem{Kind: "text", Text: "🛑 *(interrupted)*"})
					m.messages = append(m.messages, chatMessage_{role: "bot", items: items, duration: duration, interrupted: true})
				} else {
					m.messages = append(m.messages, chatMessage_{
						role:        "bot",
						text:        "🛑 *(response interrupted)*",
						duration:    duration,
						interrupted: true,
					})
				}
				m.interruptedIndex = len(m.messages) - 1
				m.streamingItems = nil
				m.streamingGroupExpanded = false
				m.state = stateStopping
				m.textInput.Prompt = userStyle.Render(" 🐾 ❯ ")
				m.updateViewportContent(true)

				// Wait for the service to finish persisting the interrupted turn
				// before accepting another message. Starting a new run earlier can
				// read a transcript from before the save and lose the context.
				if m.stopAck != nil {
					return m, waitForStopAck(m.stopAck)
				}
				m.finishStopping(nil)
				return m, nil
			}

			if m.isAutocompleting {
				m.isAutocompleting = false
				m.suggestions = nil
				return m, nil
			}
			if m.textInput.Value() != "" {
				m.textInput.Reset()
				return m, nil
			}
			return m, tea.Quit

		// Direct scrolling keys for viewport that do NOT touch textinput
		case tea.KeyUp:
			m.viewport.LineUp(2)
			m.userScrolledUp = true
			m.lastHistoryScroll = time.Now()
			return m, nil

		case tea.KeyDown:
			m.viewport.LineDown(2)
			if m.viewport.AtBottom() {
				m.userScrolledUp = false
			}
			m.lastHistoryScroll = time.Now()
			return m, nil

		case tea.KeyPgUp:
			m.viewport.HalfViewUp()
			m.userScrolledUp = true
			m.lastHistoryScroll = time.Now()
			return m, nil

		case tea.KeyPgDown:
			m.viewport.HalfViewDown()
			if m.viewport.AtBottom() {
				m.userScrolledUp = false
			}
			m.lastHistoryScroll = time.Now()
			return m, nil

		case tea.KeyEnter:
			if m.state == stateWaiting {
				// Prevent submitting while bot is responding
				return m, nil
			}
			if m.state != stateInput {
				return m, nil
			}

			// If autocomplete was active when Enter was pressed:
			if m.isAutocompleting && len(m.suggestions) > 0 {
				suggestion := m.suggestions[m.suggestionIndex]
				if strings.HasPrefix(suggestion, "/") {
					// Slash command chosen from suggestions: apply and proceed to execute immediately!
					cmd := strings.Fields(suggestion)[0]
					m.textInput.SetValue(cmd)
					m.isAutocompleting = false
					m.suggestions = nil
				} else {
					// File attachment chosen from suggestions: apply and let user continue typing
					isDir := m.applySuggestion()
					if !isDir {
						m.isAutocompleting = false
						m.suggestions = nil
					}
					return m, nil
				}
			}

			text := strings.TrimSpace(m.textInput.Value())
			if text == "" {
				return m, nil
			}

			if text == "/exit" {
				return m, tea.Quit
			}

			if text == "/sessions" || text == "/session" || text == "/list" {
				m.textInput.Reset()
				m.isAutocompleting = false
				m.suggestions = nil
				m.state = stateSessionSelect
				m.sessionCursor = 0
				m.textInput.Blur()
				return m, loadSessionsCmd(m.convRepo)
			}

			if text == "/new" {
				m.textInput.Reset()
				m.isAutocompleting = false
				m.suggestions = nil
				m.userScrolledUp = false
				m.activeConvID = "NEW"
				m.messages = []chatMessage_{}
				m.totalTokens = 0
				m.updateViewportContent(true)
				return m, nil
			}

			if text == "/models" || text == "/model" {
				m.textInput.Reset()
				m.isAutocompleting = false
				m.suggestions = nil
				m.state = stateModelLoading
				m.modelCursor = 0
				m.textInput.Blur()
				m.updateViewportContent(true)
				return m, loadModelsCmd()
			}

			if strings.HasPrefix(text, "/") {
				m.messages = append(m.messages, chatMessage_{
					role: "bot",
					text: fmt.Sprintf("❓ Unknown command: `%s`\n\nAvailable commands:\n• `/sessions` - Open session picker\n• `/new` - Start new session\n• `/exit` - Exit chat", text),
				})
				m.textInput.Reset()
				m.isAutocompleting = false
				m.suggestions = nil
				m.updateViewportContent(true)
				return m, nil
			}

			processedText, images, err := parseFileAttachments(text)
			if err != nil {
				m.messages = append(m.messages, chatMessage_{role: "bot", text: fmt.Sprintf("❌ Error attachment: %v", err)})
				m.textInput.Reset()
				m.state = stateInput
				m.updateViewportContent(true)
				return m, nil
			}

			m.messages = append(m.messages, chatMessage_{role: "user", text: text})
			m.textInput.Reset()
			m.state = stateWaiting
			m.streamingItems = nil
			m.streamingGroupExpanded = true
			m.streamStartedAt = time.Now()
			m.thoughtExpanded = false
			m.isAutocompleting = false
			m.suggestions = nil
			m.userScrolledUp = false
			m.updateViewportContent(true)
			return m, m.sendMessage(processedText, images)
		}

		if msg.String() == "ctrl+s" {
			m.state = stateSessionSelect
			m.sessionCursor = 0
			m.textInput.Blur()
			return m, loadSessionsCmd(m.convRepo)
		}

		if msg.String() == "ctrl+t" && (m.state == stateInput || m.state == stateWaiting || m.state == stateStopping) {
			expanded := m.anyTimelineExpanded() || m.thoughtExpanded
			m.setAllTimelineExpanded(!expanded)
			m.thoughtExpanded = !expanded
			m.updateViewportContent(false)
			return m, nil
		}

	case pawTickMsg:
		if m.state == stateWaiting {
			m.pawFrame = (m.pawFrame + 1) % len(pawFrames)
			m.textInput.Prompt = userStyle.Render(pawFrames[m.pawFrame])
			// Throttle viewport updates to ~5.5fps during streaming to prevent CPU exhaustion on huge chat histories
			m.updateViewportContent(true)
			return m, pawTickCmd()
		}
		return m, nil

	case nextChunkMsg:
		if m.state == stateStopping {
			if msg.done {
				m.completeStoppedResponse(msg)
				m.updateViewportContent(true)
			}
			// Late chunks are ignored; the stop ack unlocks the input.
			return m, nil
		}

		if msg.err != nil {
			if m.state == stateWaiting {
				m.messages = append(m.messages, chatMessage_{role: "bot", text: fmt.Sprintf("❌ Error: %v", msg.err)})
				m.resetStreamState()
				m.updateViewportContent(true)
			}
			return m, nil
		}

		if msg.done {
			if m.state == stateWaiting {
				duration := m.streamElapsed()
				items := m.streamingItems
				if len(msg.trace) > 0 && !hasToolItems(items) {
					// Non-stream responses carry the tool timeline in Trace.
					items = timelineFromTrace(msg.trace, msg.finalText)
				} else if msg.finalText != "" {
					items = setFinalText(items, msg.finalText)
				}
				if len(items) > 0 || msg.finalText != "" {
					m.messages = append(m.messages, chatMessage_{
						role:     "bot",
						text:     msg.finalText,
						items:    items,
						duration: duration,
					})
				}
				m.resetStreamState()
				if msg.newConvID != "" {
					m.activeConvID = msg.newConvID
				}
				m.updateViewportContent(true)
			}
			return m, nil
		}

		if msg.chunk.Kind == "final" {
			if msg.chunk.Text != "" {
				m.streamingItems = setFinalText(m.streamingItems, msg.chunk.Text)
			}
			if msg.chunk.Usage.TotalTokens > 0 {
				m.totalTokens = msg.chunk.Usage.TotalTokens
			}
			if m.state == stateWaiting {
				return m, waitForChunk(m.sub)
			}
			return m, nil
		}

		if msg.chunk.Usage.TotalTokens > 0 {
			m.totalTokens = msg.chunk.Usage.TotalTokens
		}

		switch msg.chunk.Kind {
		case "thinking", "thought":
			m.streamingItems = appendThinkingItem(m.streamingItems, msg.chunk.Thought)
		case "text":
			m.streamingItems = appendTextItem(m.streamingItems, msg.chunk.Text)
		case "tool":
			name := msg.chunk.ToolName
			if name == "" && len(msg.chunk.ToolCalls) > 0 {
				name = msg.chunk.ToolCalls[0].Function.Name
			}
			m.streamingItems = startToolItem(m.streamingItems, msg.chunk.ToolCallID, name, msg.chunk.ToolArguments)
		case "tool_result":
			m.streamingItems = finishToolItem(m.streamingItems, msg.chunk.ToolCallID, msg.chunk.ToolResultText, msg.chunk.IsError)
		case "trace", "usage":
			// Tool lifecycle events already carry the descriptive data.
		default:
			if msg.chunk.Thought != "" {
				m.streamingItems = appendThinkingItem(m.streamingItems, msg.chunk.Thought)
			}
			if msg.chunk.Text != "" {
				m.streamingItems = appendTextItem(m.streamingItems, msg.chunk.Text)
			}
			if len(msg.chunk.ToolCalls) > 0 {
				m.streamingItems = startToolItem(m.streamingItems, msg.chunk.ToolCallID, msg.chunk.ToolCalls[0].Function.Name, msg.chunk.ToolArguments)
			}
		}

		if m.state == stateWaiting {
			return m, waitForChunk(m.sub)
		}
		return m, nil

	case stopAckMsg:
		if m.state == stateStopping {
			m.finishStopping(msg.err)
			m.updateViewportContent(true)
		}
		return m, nil

	case queueTickMsg:
		logDir, err := agentdir.ActiveLogsDir()
		if err != nil {
			return m, queueTickCmd()
		}
		queuePath := filepath.Join(logDir, "cli_queue.jsonl")
		stat, err := os.Stat(queuePath)
		if err == nil && stat.Size() > m.queueFileSize {
			f, err := os.Open(queuePath)
			if err == nil {
				f.Seek(m.queueFileSize, 0)
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					line := scanner.Text()
					var qm queueMessage
					if json.Unmarshal([]byte(line), &qm) == nil {
						m.messages = append(m.messages, chatMessage_{
							role:    "bot",
							text:    qm.Text,
							thought: qm.Thought,
						})
					}
				}
				f.Close()
				m.queueFileSize = stat.Size()
				m.updateViewportContent(true)
			}
		}
		return m, queueTickCmd()
	}

	// Update textInput in stateInput or stateWaiting so user can type while bot is responding
	if m.state == stateInput || m.state == stateWaiting || m.state == stateStopping {
		m.textInput, tiCmd = m.textInput.Update(msg)
		if cleaned := mouseFragmentPattern.ReplaceAllString(m.textInput.Value(), ""); cleaned != m.textInput.Value() {
			m.textInput.SetValue(cleaned)
		}
		if m.state == stateInput {
			m.updateAutocomplete()
		}
	}

	return m, tiCmd
}

func (m *model) updateAutocomplete() {
	val := m.textInput.Value()
	cursor := cursorByteOffset(val, m.textInput.Position())

	wasAutocompleting := m.isAutocompleting

	// Check for slash command autocomplete (only at the start of input). A cursor
	// at position 0 means the user moved to the very beginning of a path like
	// "/Users/...", so there is nothing after the slash to complete.
	if cursor > 0 && strings.HasPrefix(val, "/") && !strings.Contains(val[:cursor], " ") {
		m.isAutocompleting = true
		if !wasAutocompleting {
			m.suggestionIndex = 0
		}
		prefix := val[1:cursor] // text after "/"
		m.updateCommandSuggestions(prefix)
		return
	}

	// Check for file attachment autocomplete with @
	lastAt := strings.LastIndex(val[:cursor], "@")
	if lastAt == -1 {
		m.isAutocompleting = false
		m.suggestions = nil
		return
	}

	if strings.Contains(val[lastAt:cursor], " ") {
		m.isAutocompleting = false
		m.suggestions = nil
		return
	}

	m.isAutocompleting = true
	if !wasAutocompleting {
		m.suggestionIndex = 0
	}
	prefix := val[lastAt+1 : cursor]
	m.updateSuggestions(prefix)
}

// cursorByteOffset converts textInput's rune-based cursor position into a byte
// offset in val, clamped to the string bounds so slicing can never panic.
func cursorByteOffset(val string, pos int) int {
	if pos <= 0 {
		return 0
	}
	count := 0
	for i := range val {
		if count == pos {
			return i
		}
		count++
	}
	return len(val)
}

// runeIndex is the inverse of cursorByteOffset: the rune index of a byte offset,
// as expected by textInput.SetCursor.
func runeIndex(val string, offset int) int {
	if offset <= 0 {
		return 0
	}
	if offset > len(val) {
		offset = len(val)
	}
	return utf8.RuneCountInString(val[:offset])
}

var slashCommands = []struct {
	cmd  string
	desc string
}{
	{"/sessions", "Open session picker"},
	{"/new", "Create a new chat session"},
	{"/models", "Switch LLM model (Provider & Model)"},
	{"/exit", "Exit the chat"},
}

func (m *model) updateCommandSuggestions(prefix string) {
	var matches []string
	for _, c := range slashCommands {
		if strings.HasPrefix(c.cmd[1:], strings.ToLower(prefix)) {
			matches = append(matches, c.cmd+"  "+dimStyle.Render(c.desc))
		}
	}
	if len(matches) == 0 {
		m.isAutocompleting = false
		m.suggestions = nil
		return
	}
	m.suggestions = matches
	if m.suggestionIndex >= len(matches) {
		m.suggestionIndex = 0
	}
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

	suggestion := m.suggestions[m.suggestionIndex]

	// Handle slash command suggestions (via Tab)
	if strings.HasPrefix(suggestion, "/") {
		// Extract just the command part (before the description) and add a space
		cmd := strings.Fields(suggestion)[0]
		m.textInput.SetValue(cmd + " ")
		m.textInput.SetCursor(len(cmd) + 1)
		m.isAutocompleting = false
		m.suggestions = nil
		return false
	}

	// Handle file suggestions
	val := m.textInput.Value()
	cursor := cursorByteOffset(val, m.textInput.Position())
	lastAt := strings.LastIndex(val[:cursor], "@")
	if lastAt == -1 {
		return false
	}

	if suggestion == "(no files found)" {
		return false
	}
	isDir := strings.HasSuffix(suggestion, "/")

	applied := suggestion
	if !isDir {
		if strings.Contains(applied, " ") {
			applied = "\"" + applied + "\""
		}
		applied += " "
	}

	newVal := val[:lastAt+1] + applied + val[cursor:]
	m.textInput.SetValue(newVal)
	m.textInput.SetCursor(runeIndex(newVal, lastAt+1+len(applied)))

	return isDir
}

func waitForChunk(sub chan nextChunkMsg) tea.Cmd {
	return func() tea.Msg {
		return <-sub
	}
}

func waitForStopAck(ack chan error) tea.Cmd {
	return func() tea.Msg {
		return stopAckMsg{err: <-ack}
	}
}

func (m *model) streamElapsed() time.Duration {
	if m.streamStartedAt.IsZero() {
		return 0
	}
	return time.Since(m.streamStartedAt)
}

// finishStopping unlocks the input after the service has finished persisting an
// interrupted turn.
func (m *model) finishStopping(err error) {
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		m.messages = append(m.messages, chatMessage_{
			role: "bot",
			text: fmt.Sprintf("⚠️ Interrupted context may not be saved: %v", err),
		})
	}
	m.resetStreamState()
}

// completeStoppedResponse handles a run that finished normally just as the user
// interrupted it. It replaces the local interrupted marker with the real
// response so nothing is shown twice.
func (m *model) completeStoppedResponse(msg nextChunkMsg) {
	duration := m.streamElapsed()
	items := m.streamingItems
	if len(msg.trace) > 0 && !hasToolItems(items) {
		items = timelineFromTrace(msg.trace, msg.finalText)
	} else if msg.finalText != "" {
		items = setFinalText(items, msg.finalText)
	}

	if msg.err != nil {
		m.replaceInterruptedMessage(chatMessage_{role: "bot", text: fmt.Sprintf("❌ Error: %v", msg.err)})
		m.resetStreamState()
		return
	}
	if len(items) == 0 && msg.finalText == "" {
		m.resetStreamState()
		return
	}

	m.replaceInterruptedMessage(chatMessage_{
		role:     "bot",
		text:     msg.finalText,
		items:    items,
		duration: duration,
	})
	if msg.newConvID != "" {
		m.activeConvID = msg.newConvID
	}
	m.resetStreamState()
}

func (m *model) replaceInterruptedMessage(msg chatMessage_) {
	if m.interruptedIndex >= 0 && m.interruptedIndex < len(m.messages) {
		m.messages[m.interruptedIndex] = msg
		return
	}
	m.messages = append(m.messages, msg)
}

func (m *model) resetStreamState() {
	m.interruptedIndex = -1
	m.streamingItems = nil
	m.streamingGroupExpanded = false
	m.streamStartedAt = time.Time{}
	m.state = stateInput
	m.textInput.Prompt = userStyle.Render(" 🐾 ❯ ")
	m.cancelFunc = nil
	m.stopAck = nil
}

func (m *model) sendMessage(text string, images []string) tea.Cmd {
	m.sub = make(chan nextChunkMsg)
	stopAck := make(chan error, 1)
	m.stopAck = stopAck
	convID := m.activeConvID

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel
	m.pawFrame = 0
	m.textInput.Prompt = userStyle.Render(pawFrames[0])

	go func() {
		defer cancel()
		msg := createIncomingMessage(text, images, convID)

		// Signal that the service has finished (and therefore persisted the
		// turn) before sending the terminal event. If the user pressed Esc, the
		// model waits for this ack before accepting another message.
		signalStopAck := func(err error) {
			select {
			case stopAck <- err:
			default:
			}
		}

		if config.StreamResponse {
			out, err := m.botService.BotStreamContext(ctx, msg, func(chunk channel.StreamChunk) {
				select {
				case <-ctx.Done():
					return
				case m.sub <- nextChunkMsg{chunk: chunk}:
				}
			})
			finalText := ""
			var finalTrace []provider.ReactStep
			if out != nil {
				finalText = out.Text
				finalTrace = out.Trace
			}
			if streamStopped(ctx, err) {
				signalStopAck(err)
				return
			}
			signalStopAck(nil)
			m.sub <- nextChunkMsg{done: true, err: err, newConvID: msg.ConversationID, finalText: finalText, trace: finalTrace}
			return
		}

		out, err := m.botService.BotContext(ctx, msg)
		if streamStopped(ctx, err) {
			signalStopAck(err)
			return
		}
		if err != nil {
			signalStopAck(nil)
			m.sub <- nextChunkMsg{done: true, err: err, newConvID: msg.ConversationID}
			return
		}
		signalStopAck(nil)
		m.sub <- nextChunkMsg{chunk: channel.StreamChunk{
			Text:    out.Text,
			Thought: out.Thought,
			Usage:   out.Usage,
			Trace:   out.Trace,
		}}
		m.sub <- nextChunkMsg{done: true, newConvID: msg.ConversationID, finalText: out.Text, trace: out.Trace}
	}()

	return tea.Batch(waitForChunk(m.sub), pawTickCmd())
}

// streamStopped reports whether a run ended because its context was canceled
// rather than because it produced a final answer.
func streamStopped(ctx context.Context, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	return ctx.Err() != nil && err != nil
}

func (m *model) updateViewportContent(scrollToBottom bool) {
	m.clickTargets = m.clickTargets[:0]
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
	contentLine := 0

	for i, msg := range m.messages {
		if msg.cachedView != "" && msg.cachedWidth == m.viewport.Width && msg.cachedExpanded == m.thoughtExpanded {
			b.WriteString(msg.cachedView)
			if msg.cachedGroupSpan.End > msg.cachedGroupSpan.Start {
				m.clickTargets = append(m.clickTargets, timelineClickTarget{
					StartLine:    contentLine + msg.cachedGroupSpan.Start,
					EndLine:      contentLine + msg.cachedGroupSpan.End,
					MessageIndex: i,
					Group:        true,
				})
			}
			for itemIndex, span := range msg.cachedSpans {
				if span.End <= span.Start {
					continue
				}
				m.clickTargets = append(m.clickTargets, timelineClickTarget{
					StartLine:    contentLine + span.Start,
					EndLine:      contentLine + span.End,
					MessageIndex: i,
					ItemIndex:    itemIndex,
				})
			}
			contentLine += advanceLines(msg.cachedView)
			continue
		}

		var (
			msgBuilder      strings.Builder
			cachedSpans     []timelineLineSpan
			cachedGroupSpan timelineLineSpan
		)

		if msg.role == "user" {
			content := strings.TrimSpace(msg.text)
			msgBuilder.WriteString(userBoxStyle.Width(m.viewport.Width-4).Render(content) + "\n")
		} else {
			var (
				inner     string
				spans     []timelineLineSpan
				groupSpan timelineLineSpan
			)
			if len(msg.items) > 0 {
				inner, spans, groupSpan = m.renderAgentTimeline(msg.items, timelineRenderOptions{
					GroupExpanded: msg.groupExpanded,
					Duration:      msg.duration,
					Interrupted:   msg.interrupted,
				})
			} else {
				// Legacy fallback for plain messages (commands, errors) and
				// in-memory messages created before the timeline model.
				if msg.thought != "" {
					if m.thoughtExpanded {
						if msg.renderedThought == "" {
							if m.renderer == nil {
								msg.renderedThought = strings.TrimSpace(msg.thought)
							} else {
								// Force Markdown hard line breaks by adding two spaces before newlines
								hardLineBreakThought := strings.ReplaceAll(msg.thought, "\n", "  \n")
								rendered, err := m.renderer.Render(hardLineBreakThought)
								if err == nil && strings.TrimSpace(rendered) != "" {
									msg.renderedThought = strings.TrimSpace(rendered)
								} else {
									msg.renderedThought = strings.TrimSpace(msg.thought)
								}
							}
						}
						msgBuilder.WriteString(dimStyle.Render(" ▾ Thinking (click to collapse)") + "\n")
						msgBuilder.WriteString(theme.BoxStyle.Padding(0, 1).BorderForeground(lipgloss.Color("240")).Width(m.viewport.Width-4).Render(msg.renderedThought) + "\n")
					} else {
						msgBuilder.WriteString(dimStyle.Render(" ▸ Thinking (click to expand)") + "\n\n")
					}
				}

				if msg.renderedText == "" {
					if m.renderer == nil {
						msg.renderedText = strings.TrimSpace(msg.text)
					} else {
						rendered, err := m.renderer.Render(msg.text)
						if err == nil && strings.TrimSpace(rendered) != "" {
							msg.renderedText = strings.TrimSpace(rendered)
						} else {
							msg.renderedText = strings.TrimSpace(msg.text)
						}
					}
				}
				inner = msg.renderedText
			}

			if len(msg.items) == 0 && msg.duration > 0 && inner != "" {
				label := fmt.Sprintf("Completed in %s", formatDuration(msg.duration))
				if msg.interrupted {
					label = fmt.Sprintf("Interrupted after %s", formatDuration(msg.duration))
				}
				inner += "\n\n" + dimStyle.Render(label)
			}

			content := botStyle.Render("🐱 Myaaw") + "\n\n" + inner
			msgView := botBoxStyle.Width(m.viewport.Width-4).Render(content) + "\n"
			msgBuilder.WriteString(msgView)

			if groupSpan.End > groupSpan.Start {
				// The bot box adds one top border line and the header adds
				// two lines (title + blank line) before the timeline.
				cachedGroupSpan = timelineLineSpan{Start: 3 + groupSpan.Start, End: 3 + groupSpan.End}
				m.clickTargets = append(m.clickTargets, timelineClickTarget{
					StartLine:    contentLine + cachedGroupSpan.Start,
					EndLine:      contentLine + cachedGroupSpan.End,
					MessageIndex: i,
					Group:        true,
				})
			}
			if len(spans) > 0 {
				cachedSpans = make([]timelineLineSpan, len(spans))
				for j, span := range spans {
					if span.End <= span.Start {
						continue
					}
					cachedSpans[j] = timelineLineSpan{Start: 3 + span.Start, End: 3 + span.End}
					m.clickTargets = append(m.clickTargets, timelineClickTarget{
						StartLine:    contentLine + cachedSpans[j].Start,
						EndLine:      contentLine + cachedSpans[j].End,
						MessageIndex: i,
						ItemIndex:    j,
					})
				}
			}
		}

		msg.cachedView = msgBuilder.String()
		msg.cachedWidth = m.viewport.Width
		msg.cachedExpanded = m.thoughtExpanded
		msg.cachedSpans = cachedSpans
		msg.cachedGroupSpan = cachedGroupSpan
		m.messages[i] = msg

		b.WriteString(msg.cachedView)
		contentLine += advanceLines(msg.cachedView)
	}

	if m.state == stateWaiting {
		elapsed := m.streamElapsed()
		title := botStyle.Render("🐱 Myaaw") + dimStyle.Render(" · working "+formatElapsed(elapsed))
		if len(m.streamingItems) > 0 {
			// Fast render during live stream without running the CPU-heavy
			// glamour parser on every token.
			inner, spans, groupSpan := m.renderAgentTimeline(m.streamingItems, timelineRenderOptions{
				Streaming:     true,
				GroupExpanded: m.streamingGroupExpanded,
			})
			content := title + "\n\n" + inner
			msgView := botBoxStyle.Width(m.viewport.Width-4).Render(content) + "\n"
			b.WriteString(msgView)
			if groupSpan.End > groupSpan.Start {
				m.clickTargets = append(m.clickTargets, timelineClickTarget{
					StartLine:    contentLine + 3 + groupSpan.Start,
					EndLine:      contentLine + 3 + groupSpan.End,
					MessageIndex: -1,
					Group:        true,
				})
			}
			for j, span := range spans {
				if span.End <= span.Start {
					continue
				}
				m.clickTargets = append(m.clickTargets, timelineClickTarget{
					StartLine:    contentLine + 3 + span.Start,
					EndLine:      contentLine + 3 + span.End,
					MessageIndex: -1,
					ItemIndex:    j,
				})
			}
		} else {
			b.WriteString(dimStyle.Render("Myaaw is thinking... " + formatElapsed(elapsed)))
		}
		b.WriteString("\n")
	}

	m.viewport.SetContent(b.String())
	if scrollToBottom && !m.userScrolledUp {
		m.viewport.GotoBottom()
	}
}

func (m model) renderSessionPicker() string {
	header := headerStyle.Render(" 🐾 MYAAW SESSIONS 🐾 ")

	mainPanelHeight := m.height - 4
	if mainPanelHeight < 10 {
		mainPanelHeight = 10
	}

	listWidth := 38
	if m.width > 140 {
		listWidth = 45
	}
	catWidth := m.width - listWidth - 6
	if catWidth < 20 {
		catWidth = 20
	}

	// Calculate maxVisible: 3 lines per conversation, ~5 lines for headers
	availableForConvs := mainPanelHeight - 6
	if availableForConvs < 3 {
		availableForConvs = 3
	}
	maxVisibleConvs := availableForConvs / 3
	if maxVisibleConvs < 1 {
		maxVisibleConvs = 1
	}
	maxVisible := maxVisibleConvs + 1 // including item 0 (New Session)

	totalItems := len(m.conversations) + 1

	start := 0
	if m.sessionCursor >= maxVisible {
		start = m.sessionCursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > totalItems {
		end = totalItems
	}

	// Build left side lines
	var leftLines []string
	leftLines = append(leftLines, theme.HighlightStyle.Bold(true).Render(" 💬 CHAT SESSIONS"))
	leftLines = append(leftLines, theme.MutedStyle.Render(" Select a session or create a new one."))
	leftLines = append(leftLines, "")

	if start > 0 {
		leftLines = append(leftLines, theme.MutedStyle.Render("   ↑ ... more"))
	}

	for i := start; i < end; i++ {
		if i == 0 {
			if m.sessionCursor == 0 {
				leftLines = append(leftLines, theme.HighlightStyle.Bold(true).Render(" ❯ [+] ✦ New Session"))
			} else {
				leftLines = append(leftLines, theme.BaseStyle.Render("   [+] ✦ New Session"))
			}
			leftLines = append(leftLines, "")
		} else {
			convIdx := i - 1
			conv := m.conversations[convIdx]

			title := conv.Title
			if strings.TrimSpace(title) == "" {
				title = "New Chat"
			}
			if len(title) > 30 {
				title = title[:27] + "..."
			}

			timeStr := ""
			if !conv.UpdatedAt.IsZero() {
				timeStr = humanize.Time(conv.UpdatedAt)
			} else if !conv.CreatedAt.IsZero() {
				timeStr = humanize.Time(conv.CreatedAt)
			} else {
				timeStr = "recently"
			}

			msgCount := 0
			for _, message := range conv.Messages {
				if message.Role == "user" || (message.Role == "assistant" && provider.MessageText(message) != "") {
					msgCount++
				}
			}
			meta := fmt.Sprintf("%s • %d msgs", timeStr, msgCount)

			if m.sessionCursor == i {
				leftLines = append(leftLines, fmt.Sprintf(" ❯ %s", theme.HighlightStyle.Bold(true).Render(title)))
				leftLines = append(leftLines, fmt.Sprintf("     %s", theme.SecondaryStyle.Render(meta)))
			} else {
				leftLines = append(leftLines, fmt.Sprintf("   %s", theme.BaseStyle.Render(title)))
				leftLines = append(leftLines, fmt.Sprintf("     %s", theme.MutedStyle.Render(meta)))
			}
			leftLines = append(leftLines, "")
		}
	}

	if end < totalItems {
		leftLines = append(leftLines, theme.MutedStyle.Render("   ↓ ... more"))
	}

	if len(m.conversations) == 0 {
		leftLines = append(leftLines, theme.MutedStyle.Italic(true).Render("   (No sessions yet)"))
	}

	// Pad leftLines to exact mainPanelHeight
	for len(leftLines) < mainPanelHeight {
		leftLines = append(leftLines, "")
	}

	leftColStyle := lipgloss.NewStyle().Width(listWidth)
	var paddedLeftLines []string
	for _, l := range leftLines[:mainPanelHeight] {
		paddedLeftLines = append(paddedLeftLines, leftColStyle.Render(l))
	}

	// Build right side (cat mascot) lines centered vertically & horizontally
	rawCatLines := strings.Split(strings.Trim(catAscii, "\n"), "\n")
	catColStyle := lipgloss.NewStyle().Width(catWidth).Align(lipgloss.Center).Foreground(lipgloss.Color("240"))

	catHeight := len(rawCatLines)
	topPad := 0
	if mainPanelHeight > catHeight {
		topPad = (mainPanelHeight - catHeight) / 2
	}

	var rightLines []string
	for i := 0; i < topPad; i++ {
		rightLines = append(rightLines, catColStyle.Render(""))
	}
	for _, cl := range rawCatLines {
		if len(rightLines) < mainPanelHeight {
			rightLines = append(rightLines, catColStyle.Render(cl))
		}
	}
	for len(rightLines) < mainPanelHeight {
		rightLines = append(rightLines, catColStyle.Render(""))
	}

	// Join left and right columns horizontally per line
	var mainViewLines []string
	for i := 0; i < mainPanelHeight; i++ {
		mainViewLines = append(mainViewLines, paddedLeftLines[i]+"  "+rightLines[i])
	}
	mainView := strings.Join(mainViewLines, "\n")

	footer := footerStyle.Render("  Enter: Select • ↑/↓ or j/k: Navigate • d: Delete • Esc: Back to Chat  ")

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		header,
		mainView,
		footer,
	)
}

func (m model) renderModelPicker() string {
	header := headerStyle.Render(" 🐾 MYAAW MODELS 🐾 ")

	mainPanelHeight := m.height - 4
	if mainPanelHeight < 10 {
		mainPanelHeight = 10
	}

	listWidth := 60
	if m.width > 120 {
		listWidth = m.width/2 - 10
		if listWidth > 90 {
			listWidth = 90
		}
	}
	if listWidth < 50 {
		listWidth = 50
	}

	catWidth := m.width - listWidth - 6
	if catWidth < 20 {
		catWidth = 20
	}

	availableForModels := mainPanelHeight - 6
	if availableForModels < 3 {
		availableForModels = 3
	}
	maxVisible := availableForModels
	totalItems := len(m.availableModels)
	if totalItems == 0 {
		totalItems = 1
	}

	start := 0
	if m.modelCursor >= maxVisible {
		start = m.modelCursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > totalItems {
		end = totalItems
	}

	var leftLines []string
	leftLines = append(leftLines, theme.HighlightStyle.Bold(true).Render(" 🤖 SELECT LLM MODEL"))
	leftLines = append(leftLines, theme.MutedStyle.Render(" Choose the model to use for this chat."))
	leftLines = append(leftLines, "")

	if len(m.availableModels) == 0 && m.state != stateModelLoading {
		leftLines = append(leftLines, theme.ErrorStyle.Render(" No models configured."))
	} else if m.state == stateModelLoading {
		leftLines = append(leftLines, theme.WarningStyle.Render(" ⏳ Fetching live models from providers..."))
	} else {
		for i := start; i < end; i++ {
			line := m.availableModels[i]

			// Prevent lipgloss word wrap which breaks the layout height
			maxLen := listWidth - 6
			if len(line) > maxLen && maxLen > 3 {
				line = line[:maxLen-3] + "..."
			}

			if i == m.modelCursor {
				leftLines = append(leftLines, " "+theme.HighlightStyle.Render("► "+line))
			} else {
				leftLines = append(leftLines, "   "+line)
			}
		}
	}
	for len(leftLines) < mainPanelHeight {
		leftLines = append(leftLines, "")
	}

	leftColStyle := lipgloss.NewStyle().Width(listWidth)
	var paddedLeftLines []string
	for _, l := range leftLines[:mainPanelHeight] {
		paddedLeftLines = append(paddedLeftLines, leftColStyle.Render(l))
	}

	rawCatLines := strings.Split(strings.Trim(catAscii, "\n"), "\n")
	catColStyle := lipgloss.NewStyle().Width(catWidth).Align(lipgloss.Center).Foreground(lipgloss.Color("240"))

	catHeight := len(rawCatLines)
	topPad := 0
	if mainPanelHeight > catHeight {
		topPad = (mainPanelHeight - catHeight) / 2
	}

	var rightLines []string
	for i := 0; i < topPad; i++ {
		rightLines = append(rightLines, catColStyle.Render(""))
	}
	for _, cl := range rawCatLines {
		if len(rightLines) < mainPanelHeight {
			rightLines = append(rightLines, catColStyle.Render(cl))
		}
	}
	for len(rightLines) < mainPanelHeight {
		rightLines = append(rightLines, catColStyle.Render(""))
	}

	var mainViewLines []string
	for i := 0; i < mainPanelHeight; i++ {
		mainViewLines = append(mainViewLines, paddedLeftLines[i]+"  "+rightLines[i])
	}
	mainView := strings.Join(mainViewLines, "\n")

	footer := footerStyle.Render("  Enter: Select • ↑/↓ or j/k: Navigate • Esc: Cancel  ")

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		header,
		mainView,
		footer,
	)
}

func (m model) View() string {
	if m.state == stateSessionSelect {
		return m.renderSessionPicker()
	}
	if m.state == stateModelSelect || m.state == stateModelLoading {
		return m.renderModelPicker()
	}

	header := headerStyle.Render(fmt.Sprintf(" 🐾 MYAAW CHAT · %s 🐾 ", agentdir.Active()))

	footerText := "  Esc/Ctrl+C: Quit • /sessions: Pick Session • ↑/↓: Scroll Chat • @: Attach File • Ctrl+T: Thinking/Tools  "
	if m.state == stateWaiting {
		footerText = "  Esc: Interrupt Response • ↑/↓: Scroll Chat • Ctrl+T: Thinking/Tools  "
	} else if m.state == stateStopping {
		footerText = "  Saving interrupted context... • Ctrl+C: Quit  "
	} else if m.isAutocompleting && len(m.suggestions) > 0 {
		footerText = "  Esc: Cancel • ↑/↓: Navigate • Tab/Enter: Select  "
	}
	leftText := footerStyle.Render(footerText)
	prov := config.CurrentProviderID
	if prov == "" {
		prov = config.LLMProviderName
	}
	mod := config.LLMDefaultModel
	if mod == "" {
		mod = "default"
	}

	tokenText := ""
	if m.totalTokens > 0 {
		tokenText = fmt.Sprintf(" • %d tokens ", m.totalTokens)
	}

	rightText := footerStyle.Render(fmt.Sprintf(" %s • %s%s ", prov, mod, tokenText))

	spaceWidth := m.width - lipgloss.Width(leftText) - lipgloss.Width(rightText)
	if spaceWidth < 0 {
		spaceWidth = 0
	}
	footer := leftText + strings.Repeat(" ", spaceWidth) + rightText

	var autocomplete string
	acLineCount := 0
	if m.isAutocompleting && len(m.suggestions) > 0 {
		var b strings.Builder

		maxItems := 8
		if m.height < 30 {
			maxItems = 5
		}
		start := 0
		if m.suggestionIndex >= maxItems {
			start = m.suggestionIndex - maxItems + 1
		}
		end := start + maxItems
		if end > len(m.suggestions) {
			end = len(m.suggestions)
		}

		b.WriteString(theme.HighlightStyle.Render("  Suggestions (Arrow Up/Down to navigate, Tab/Enter to select):") + "\n")
		acLineCount++

		if start > 0 {
			b.WriteString(theme.MutedStyle.Render("    ↑ ... more") + "\n")
			acLineCount++
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
			acLineCount++
		}

		if end < len(m.suggestions) {
			b.WriteString(theme.MutedStyle.Render("    ↓ ... more") + "\n")
			acLineCount++
		}

		autocomplete = b.String()
	}

	inputView := m.textInput.View()
	if autocomplete != "" {
		vp := m.viewport
		vp.Height = m.height - 6 - acLineCount
		if vp.Height < 3 {
			vp.Height = 3
		}
		if len(m.messages) == 0 {
			placeholder := lipgloss.NewStyle().
				Width(vp.Width).
				Height(vp.Height).
				Align(lipgloss.Center).
				AlignVertical(lipgloss.Center).
				Foreground(lipgloss.Color("240")).
				Render(catAscii)
			vp.SetContent(placeholder)
		}

		return fmt.Sprintf(
			"%s\n\n%s\n\n%s%s\n\n%s",
			header,
			vp.View(),
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

func runInteractive(botService service.BotService, convRepo repository.ConversationRepository, adapter *cliAdapter.CLIAdapter) {
	logDir, err := agentdir.ActiveLogsDir()
	if err != nil {
		log.Fatal("Error resolving agent log directory:", err)
	}

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

	p := tea.NewProgram(initialModel(botService, convRepo, adapter), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		log.Fatal("Error running TUI:", err)
	}
}

func parseFileAttachments(text string) (string, []string, error) {
	// 1. Collect potential paths from @ tags
	reAt := regexp.MustCompile(`@(?:"([^"]+)"|([^\s"]+))`)
	matchesAt := reAt.FindAllStringSubmatch(text, -1)

	// 2. Collect potential paths from absolute/home paths (common in drag-and-drop)
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
		path := strings.ReplaceAll(original, `\ `, " ")

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
