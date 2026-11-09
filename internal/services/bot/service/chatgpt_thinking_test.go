package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

func TestChatGPTThinkingStreamsAndPersists(t *testing.T) {
	const (
		credentialID = "33333333-3333-4333-8333-333333333333"
		token        = "synthetic-thinking-test-token"
		summary      = "Inspect the question carefully."
		answer       = "The answer is 42."
		ciphertext   = "encrypted-continuation-not-visible-thinking"
	)

	for _, tc := range []struct {
		name    string
		thought string
		stream  bool
	}{
		{name: "summary streaming and duplicate snapshots", thought: summary, stream: true},
		{name: "terminal-only summary", thought: summary},
		{name: "ciphertext-only with reasoning tokens"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// These globals and the default transport are process-wide: do not run
			// these cases in parallel or fall back to the real transport.
			preserveRuntimeProvider(t)
			oldObs, oldVerbose := config.ObsEnabled, config.Verbose
			config.ObsEnabled, config.Verbose = false, false
			t.Cleanup(func() { config.ObsEnabled, config.Verbose = oldObs, oldVerbose })

			svc, repo := newCheckpointService(t, nil)
			root, err := agentdir.ActiveRoot()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			seedRuntimeCredential(t, os.Getenv("HOME"), credentialID, token)
			cfg := config.ProviderConfig{
				Type: "openai_chatgpt", AuthMode: "oauth",
				DefaultModel: "gpt-5.4", CredentialID: credentialID,
			}
			setRuntimeProvider(cfg)
			config.CurrentProviderID = "chatgpt-thinking-test"

			var wire strings.Builder
			frame := func(event interface{}) {
				t.Helper()
				data, err := json.Marshal(event)
				if err != nil {
					t.Fatal(err)
				}
				fmt.Fprintf(&wire, "data: %s\n\n", data)
			}
			summaryParts := []map[string]interface{}{}
			if tc.thought != "" {
				summaryParts = append(summaryParts, map[string]interface{}{"type": "summary_text", "text": tc.thought})
			}
			reasoning := map[string]interface{}{
				"type": "reasoning", "id": "rs-thinking", "summary": summaryParts,
				"encrypted_content": ciphertext,
			}
			if tc.stream {
				frame(map[string]interface{}{
					"type": "response.output_item.added", "output_index": 0,
					"item": map[string]interface{}{
						"type": "reasoning", "id": "rs-thinking", "summary": []interface{}{},
						"encrypted_content": ciphertext,
					},
				})
				for _, delta := range []string{"Inspect the question ", "carefully."} {
					frame(map[string]interface{}{
						"type": "response.reasoning_summary_text.delta", "output_index": 0,
						"item_id": "rs-thinking", "summary_index": 0, "delta": delta,
					})
				}
				frame(map[string]interface{}{
					"type": "response.reasoning_summary_text.done", "output_index": 0,
					"item_id": "rs-thinking", "summary_index": 0, "text": summary,
				})
				frame(map[string]interface{}{
					"type": "response.reasoning_summary_part.done", "output_index": 0,
					"item_id": "rs-thinking", "summary_index": 0, "part": summaryParts[0],
				})
				frame(map[string]interface{}{
					"type": "response.output_item.done", "output_index": 0, "item": reasoning,
				})
			}
			frame(map[string]interface{}{
				"type": "response.output_item.added", "output_index": 1,
				"item": map[string]interface{}{
					"type": "message", "id": "msg-answer", "role": "assistant", "content": []interface{}{},
				},
			})
			frame(map[string]interface{}{
				"type": "response.output_text.delta", "output_index": 1,
				"content_index": 0, "item_id": "msg-answer", "delta": answer,
			})
			frame(map[string]interface{}{
				"type": "response.completed",
				"response": map[string]interface{}{
					"status": "completed",
					"output": []interface{}{reasoning, map[string]interface{}{
						"type": "message", "id": "msg-answer", "role": "assistant",
						"content": []map[string]interface{}{{"type": "output_text", "text": answer}},
					}},
					"usage": map[string]interface{}{
						"input_tokens": 10, "output_tokens": 12, "total_tokens": 22,
						"output_tokens_details": map[string]interface{}{"reasoning_tokens": 7},
					},
				},
			})

			oldTransport := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = oldTransport })
			requests := 0
			http.DefaultTransport = runtimeTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.String() != "https://api.openai.com/v1/responses" {
					t.Errorf("unexpected HTTP request: %s", r.URL)
					return nil, fmt.Errorf("unexpected HTTP request: %s", r.URL)
				}
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+token {
					t.Errorf("unexpected request method or synthetic credential: %s / %q", r.Method, r.Header.Get("Authorization"))
				}
				var body struct {
					Model     string            `json:"model"`
					Stream    bool              `json:"stream"`
					Store     bool              `json:"store"`
					Include   []string          `json:"include"`
					Reasoning map[string]string `json:"reasoning"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					return nil, fmt.Errorf("decode ChatGPT request: %w", err)
				}
				if body.Model != cfg.DefaultModel || !body.Stream || body.Store ||
					!reflect.DeepEqual(body.Include, []string{"reasoning.encrypted_content"}) ||
					!reflect.DeepEqual(body.Reasoning, map[string]string{"summary": "auto"}) {
					t.Errorf("unexpected ChatGPT reasoning request: %+v", body)
				}
				return &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader(wire.String())), Request: r,
				}, nil
			})

			p, err := provider.CreateLLMProviderFromConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := p.(*provider.ChatGPTProvider); !ok {
				t.Fatalf("provider = %T, want actual ChatGPTProvider", p)
			}
			svc.llmProvider, svc.agent = p, agent.NewAgent(p)
			svc.currentProviderID, svc.currentProviderConfig = config.CurrentProviderID, cfg
			svc.userRepo = &providerSwitchUserRepo{user: model.User{
				UserId: 1, Provider: cfg.Type, Model: cfg.DefaultModel,
			}}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var thought, text strings.Builder
			var contentKinds []string
			thoughtChunks, finals := 0, 0
			out, err := svc.BotStreamContext(ctx, &channel.IncomingMessage{
				UserID: 1, ConversationID: "conv-1", Channel: "api", Text: "What is the answer?",
			}, func(chunk channel.StreamChunk) {
				switch chunk.Kind {
				case "thought":
					thoughtChunks++
					thought.WriteString(chunk.Thought)
					contentKinds = append(contentKinds, chunk.Kind)
				case "text":
					text.WriteString(chunk.Text)
					contentKinds = append(contentKinds, chunk.Kind)
				case "final":
					finals++
					if chunk.Text != answer || chunk.Thought != tc.thought {
						t.Errorf("final chunk text/thought = %q / %q", chunk.Text, chunk.Thought)
					}
				}
			})
			if err != nil {
				t.Fatalf("BotStreamContext: %v", err)
			}
			if requests != 1 {
				t.Fatalf("HTTP requests = %d, want exactly one (no refresh, title generation, or retry)", requests)
			}
			if thought.String() != tc.thought || text.String() != answer {
				t.Errorf("streamed thought/text = %q / %q, want %q / %q", thought.String(), text.String(), tc.thought, answer)
			}
			if tc.thought == "" && thoughtChunks != 0 {
				t.Errorf("ciphertext/tokens generated %d fake thought chunks", thoughtChunks)
			}
			if tc.stream && (thoughtChunks < 2 || len(contentKinds) == 0 || contentKinds[0] != "thought") {
				t.Errorf("summary did not stream before answer: %v", contentKinds)
			}
			if finals != 1 || out.Text != answer || out.Thought != tc.thought || out.ConversationID != "conv-1" {
				t.Errorf("outgoing message = %+v, final chunks = %d", out, finals)
			}
			if out.Usage.ThoughtsTokens != 7 {
				t.Errorf("reasoning token usage = %d, want 7 even without visible thought", out.Usage.ThoughtsTokens)
			}

			if len(repo.updates) < 2 {
				t.Fatalf("repository updates = %d, want checkpoint and final persistence", len(repo.updates))
			}
			saved := repo.updates[len(repo.updates)-1]
			if saved.id != "conv-1" || len(saved.messages) != 2 || saved.messages[0].Role != "user" {
				t.Fatalf("persisted conversation = %+v", saved)
			}
			assistant := saved.messages[1]
			if assistant.Role != "assistant" || provider.MessageText(assistant) != answer || assistant.Thought != tc.thought {
				t.Errorf("persisted assistant = %+v", assistant)
			}
			var continuation struct {
				Output []map[string]interface{} `json:"output"`
			}
			if err := json.Unmarshal(assistant.ProviderMetadata[cfg.Type], &continuation); err != nil {
				t.Fatalf("persisted ChatGPT continuation: %v", err)
			}
			// Compare decoded JSON so both the opaque marker and original summary
			// survive independently of their display projection into Thought.
			expectedJSON, err := json.Marshal(reasoning)
			if err != nil {
				t.Fatal(err)
			}
			var expected map[string]interface{}
			if err := json.Unmarshal(expectedJSON, &expected); err != nil {
				t.Fatal(err)
			}
			if len(continuation.Output) != 2 || !reflect.DeepEqual(continuation.Output[0], expected) {
				t.Errorf("persisted reasoning metadata = %#v, want %#v", continuation.Output, expected)
			}
		})
	}
}
