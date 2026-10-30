package cli

import (
	"encoding/json"
	"fmt"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CLIMeta struct{}

type CLIAdapter struct{}

func NewCLIAdapter() *CLIAdapter {
	return &CLIAdapter{}
}

func (a *CLIAdapter) Name() string {
	return "cli"
}

func (a *CLIAdapter) ParseIncoming(payload json.RawMessage) (*channel.IncomingMessage, error) {
	var req struct {
		UserID         int    `json:"user_id"`
		Text           string `json:"text"`
		ConversationID string `json:"conversation_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to parse CLI payload: %w", err)
	}

	return &channel.IncomingMessage{
		UserID:         req.UserID,
		Text:           req.Text,
		ConversationID: req.ConversationID,
		Channel:        "cli",
		RawMeta:        CLIMeta{},
	}, nil
}

type queueMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text"`
	Thought   string    `json:"thought"`
}

func (a *CLIAdapter) appendQueue(out *channel.OutgoingMessage) {
	logDir, err := agentdir.ActiveLogsDir()
	if err != nil {
		return
	}

	queuePath := filepath.Join(logDir, "cli_queue.jsonl")
	f, err := os.OpenFile(queuePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	msg := queueMessage{
		Timestamp: time.Now(),
		Text:      out.Text,
		Thought:   out.Thought,
	}

	b, _ := json.Marshal(msg)
	f.Write(append(b, '\n'))
}

func (a *CLIAdapter) Send(msg *channel.IncomingMessage, out *channel.OutgoingMessage) error {
	a.appendQueue(out)

	if out.Thought != "" {
		fmt.Fprintln(os.Stderr, "\033[90m"+"💭 Reasoning:\n"+out.Thought+"\033[0m")
	}
	fmt.Fprintln(os.Stdout, out.Text)
	a.printAttachments(out.Attachments)
	return nil
}

// saveAttachments writes delivered files under the agent's logs directory so a
// terminal user can open them, and returns the paths.
func (a *CLIAdapter) saveAttachments(attachments []channel.Attachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	logDir, err := agentdir.ActiveLogsDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(logDir, "attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	paths := make([]string, 0, len(attachments))
	for _, att := range attachments {
		if len(att.Data) == 0 {
			continue
		}
		name := strings.TrimSpace(att.FileName)
		if name == "" {
			name = fmt.Sprintf("attachment-%d", time.Now().UnixNano())
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, att.Data, 0o644); err != nil {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

func (a *CLIAdapter) printAttachments(attachments []channel.Attachment) {
	for _, path := range a.saveAttachments(attachments) {
		fmt.Fprintln(os.Stdout, "📎 "+path)
	}
}

func (a *CLIAdapter) SendStream(msg *channel.IncomingMessage, streamFn func(onChunk func(chunk channel.StreamChunk)) error) (*channel.OutgoingMessage, error) {
	fullContent := ""
	printedText := false
	lastThoughtLen := 0
	lastTraceLen := 0
	var attachments []channel.Attachment
	err := streamFn(func(chunk channel.StreamChunk) {
		if chunk.Kind == "attachment" && len(chunk.Attachments) > 0 {
			attachments = append(attachments, chunk.Attachments...)
			a.printAttachments(chunk.Attachments)
		}
		if chunk.Thought != "" {
			if len(chunk.Thought) > lastThoughtLen {
				delta := chunk.Thought[lastThoughtLen:]
				fmt.Fprint(os.Stderr, "\033[90m"+delta+"\033[0m")
				lastThoughtLen = len(chunk.Thought)
			}
		}

		if len(chunk.Trace) > lastTraceLen {
			for i := lastTraceLen; i < len(chunk.Trace); i++ {
				step := chunk.Trace[i]
				if step.Observation != "" {
					fmt.Fprintf(os.Stderr, "\n\033[90m🛠️  Using %s...\033[0m\n", step.Action)
				}
			}
			lastTraceLen = len(chunk.Trace)
		}

		if chunk.Kind == "final" {
			if chunk.Text != "" && chunk.Text != fullContent {
				if printedText {
					fmt.Fprint(os.Stdout, "\n\n")
				}
				fmt.Fprint(os.Stdout, chunk.Text)
			}
			fullContent = chunk.Text
			printedText = true
		} else if chunk.Text != "" {
			fmt.Fprint(os.Stdout, chunk.Text)
			fullContent += chunk.Text
			printedText = true
		}
	})
	fmt.Println()
	return &channel.OutgoingMessage{Text: fullContent, Attachments: attachments}, err
}

func (a *CLIAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	fmt.Fprintln(os.Stderr, "❌ "+errText)
	return nil
}
