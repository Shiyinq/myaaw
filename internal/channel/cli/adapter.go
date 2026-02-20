package cli

import (
	"encoding/json"
	"fmt"
	"myaaw/internal/channel"
	"os"
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
		UserID int    `json:"user_id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("failed to parse CLI payload: %w", err)
	}

	return &channel.IncomingMessage{
		UserID:  req.UserID,
		Text:    req.Text,
		Channel: "cli",
		RawMeta: CLIMeta{},
	}, nil
}

func (a *CLIAdapter) Send(msg *channel.IncomingMessage, out *channel.OutgoingMessage) error {
	if out.Thought != "" {
		fmt.Fprintln(os.Stderr, "\033[90m"+"💭 Reasoning:\n"+out.Thought+"\033[0m")
	}
	fmt.Fprintln(os.Stdout, out.Text)
	return nil
}

func (a *CLIAdapter) SendStream(msg *channel.IncomingMessage, streamFn func(onChunk func(chunk channel.StreamChunk)) error) (*channel.OutgoingMessage, error) {
	fullContent := ""
	lastThoughtLen := 0
	lastTraceLen := 0
	err := streamFn(func(chunk channel.StreamChunk) {
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

		if chunk.Text != "" {
			fmt.Fprint(os.Stdout, chunk.Text)
			fullContent += chunk.Text
		}
	})
	fmt.Println()
	return &channel.OutgoingMessage{Text: fullContent}, err
}

func (a *CLIAdapter) SendError(msg *channel.IncomingMessage, errText string) error {
	fmt.Fprintln(os.Stderr, "❌ "+errText)
	return nil
}
