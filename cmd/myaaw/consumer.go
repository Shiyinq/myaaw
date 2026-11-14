package main

import (
	"encoding/json"
	"fmt"
	"log"
	"myaaw/internal/channel"
	"myaaw/internal/config"
	"myaaw/internal/cron"
	"myaaw/internal/heartbeat"
	"myaaw/internal/services/bot/service"
	"myaaw/internal/services/queue/repository"
	"myaaw/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

func startBackgroundWorkers() {
	config.LoadConfig()

	var hb *heartbeat.HeartbeatService
	if config.Heartbeat.Active {
		hb = heartbeat.NewHeartbeatService()
		go hb.Start()
	}

	// Start Cron Scheduler & Watcher
	scheduler, err := cron.NewDefaultScheduler()
	if err != nil {
		log.Printf("Failed to initialize cron scheduler: %v", err)
	} else {
		if err := scheduler.Start(); err != nil {
			log.Printf("Failed to start cron scheduler: %v", err)
		}
		defer scheduler.Stop()
		// Start Hot-Reload Watcher for jobs.json
		go scheduler.Watch()
	}

	// Start Global Config Watcher for config.json
	config.WatchConfig(func() {
		log.Println("Global config changed. Reloading components...")

		// Reload Cron Scheduler config (Active/Inactive)
		if scheduler != nil {
			scheduler.ReloadConfig()
		}

		// Reload Heartbeat config (Interval/Active)
		if hb != nil {
			hb.ReloadConfig()
		}
	})

	consumeMessages()
}

func sendToWebhookBot(jsonBody []byte) error {
	client := resty.New()
	// The webhook handles a whole agent run before responding, so the request
	// timeout must outlive the run timeout; otherwise a healthy run looks like a
	// failed delivery and frees its worker slot early.
	timeout := time.Duration(config.AgentRunTimeoutSeconds+60) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	client.SetTimeout(timeout)

	baseURL := config.MYAAWBaseURL
	if baseURL == "" {
		baseURL = "http://localhost" + config.PORT
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(jsonBody).
		Post(fmt.Sprintf("%s/webhook/bot", baseURL))

	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if resp.IsError() {
		return fmt.Errorf("received error response from webhook: %s %v", resp.Status(), resp.String())
	}

	log.Println("Message forwarded to webhook bot successfully")
	return nil
}

// messageBatchWindow is the quiet period that turns a spam burst into one turn.
const messageBatchWindow = time.Second

func consumeMessages() {
	workers := config.QueueWorkers
	if workers < 1 {
		workers = 1
	}
	if workers > config.MaxQueueWorkers {
		workers = config.MaxQueueWorkers
	}

	dispatch := newKeyedDispatcher(workers, func(msg []byte) {
		if err := sendToWebhookBot(msg); err != nil {
			log.Printf("Failed to send message to webhook: %s", err)
		}
	})

	// Plain chat messages are collected for one quiet second so a spam burst
	// becomes one turn that still contains every message.
	router := newMessageRouter(dispatch, messageBatchWindow)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			router.flush()
		}
	}()

	log.Printf("Waiting for messages from in-memory queue (%d worker(s), per-user order kept). To exit press CTRL+C", workers)

	for msg := range repository.WebhookQueue {
		log.Printf("Received message: %s", msg)
		router.route(msg)
	}
}

// messageRouter turns queue envelopes into dispatcher jobs. It owns the only
// three-way split of inbound traffic: a stop command runs immediately, a plain
// chat message is collected for the quiet window, and everything else keeps its
// place in the conversation queue.
type messageRouter struct {
	dispatch *keyedDispatcher
	batcher  *messageBatcher
	// cancel stops the turn answering a conversation key. It is a field so tests
	// can run without the process-wide run registry.
	cancel func(key string) bool
}

func newMessageRouter(dispatch *keyedDispatcher, window time.Duration) *messageRouter {
	return &messageRouter{
		dispatch: dispatch,
		batcher:  newMessageBatcher(window),
		cancel:   service.CancelTurn,
	}
}

// flush starts every batch whose quiet window elapsed. It is called repeatedly so
// a conversation that is still streaming holds its batch until it is idle.
func (r *messageRouter) flush() {
	for _, batch := range r.batcher.due(r.dispatch.Busy) {
		r.dispatch.Submit(batch.key, batch.envelope)
	}
}

// route dispatches one envelope from the queue.
func (r *messageRouter) route(msg []byte) {
	facts := parseEnvelope(msg)
	key := turnKeyFromBody(msg)
	dispatchKey := key
	if dispatchKey == "" {
		dispatchKey = messageQueueKey(msg)
	}

	switch {
	case facts.stop:
		// A stop command must reach the agent while the turn it cancels is still
		// streaming. It skips the conversation queue and also drops messages that
		// have not started yet.
		if key != "" {
			r.batcher.clear(key)
		}
		r.dispatch.SubmitControl(msg)
	case facts.interrupts:
		// Every plain message is kept, but the conversation gets one answer: cancel
		// the response in progress, collect the message, and let the quiet window
		// decide when the batch starts.
		if key == "" {
			r.dispatch.Submit(dispatchKey, msg)
			return
		}
		r.cancel(key)
		r.batcher.add(key, facts.channel, facts.payload)
	default:
		r.dispatch.Submit(dispatchKey, msg)
	}
}

// envelopeFacts are the routing facts of one queue message: how it is dispatched
// and, for a batched message, the channel payload that must be replayed.
type envelopeFacts struct {
	channel    string
	payload    json.RawMessage
	stop       bool
	interrupts bool
}

// parseEnvelope reads the message text to tell a stop command and a new chat
// message apart from ordinary ordered work.
func parseEnvelope(body []byte) envelopeFacts {
	var envelope struct {
		Channel string          `json:"channel"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return envelopeFacts{}
	}

	var inner struct {
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
	}
	// A malformed inner payload still routes as ordinary work; the handler reports
	// the parse error for it.
	_ = json.Unmarshal(envelope.Payload, &inner)

	text := strings.TrimSpace(inner.Message.Text)
	isCommand, name, _ := utils.ParseCommand(text)
	stop := isCommand && strings.EqualFold(name, "stop")
	channelName := strings.ToLower(strings.TrimSpace(envelope.Channel))
	// Slash commands keep their order behind running work: they are cheap and
	// some of them (for example /new) depend on the current conversation. Plain
	// chat text is what should interrupt the answer being written, and "!" text
	// that is not a known command reaches the model as text too.
	return envelopeFacts{
		channel:    channelName,
		payload:    envelope.Payload,
		stop:       stop,
		interrupts: !stop && channel.Interactive(channelName) && !strings.HasPrefix(text, "/"),
	}
}

// turnKeyFromBody builds the same conversation identity the service uses for
// its lock and run registry, so an interrupting message cancels the turn that
// is answering the same chat, topic, or channel.
func turnKeyFromBody(body []byte) string {
	var envelope struct {
		Channel string `json:"channel"`
		Payload struct {
			Message struct {
				MessageThreadID json.RawMessage `json:"message_thread_id"`
				Chat            struct {
					ID json.RawMessage `json:"id"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}

	chatID := rawIDString(envelope.Payload.Message.Chat.ID)
	if chatID == "" {
		return ""
	}

	// The adapters only set TopicID for a real topic (thread >= 2), so the key
	// uses the same rule for Telegram groups and forum topics.
	topicID := ""
	if thread, err := strconv.Atoi(rawIDString(envelope.Payload.Message.MessageThreadID)); err == nil && thread >= 2 {
		topicID = strconv.Itoa(thread)
	}
	return service.TurnKey(envelope.Channel, "", chatID, topicID)
}

// messageQueueKey groups work by conversation target so messages that share a
// session are handled in order. Direct chats keep the per-user key; group chats
// (including forum topics) use the same scope as the conversation lookup, so a
// user's DM and a group topic never serialize each other.
func messageQueueKey(body []byte) string {
	var envelope struct {
		Channel string `json:"channel"`
		Payload struct {
			Message struct {
				MessageThreadID json.RawMessage `json:"message_thread_id"`
				Chat            struct {
					ID   json.RawMessage `json:"id"`
					Type string          `json:"type"`
				} `json:"chat"`
				From struct {
					ID json.Number `json:"id"`
				} `json:"from"`
			} `json:"message"`
			DiscordAuthor struct {
				ID string `json:"id"`
			} `json:"discord_author"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "unparsed"
	}

	if envelope.Channel == "telegram" {
		chatType := strings.ToLower(strings.TrimSpace(envelope.Payload.Message.Chat.Type))
		if chatType != "" && chatType != "private" {
			if key := telegramScopeKey(envelope.Payload.Message.Chat.ID, envelope.Payload.Message.MessageThreadID); key != "" {
				return "scope:" + key
			}
		}
	}

	if envelope.Channel == "discord" {
		chatType := strings.ToLower(strings.TrimSpace(envelope.Payload.Message.Chat.Type))
		if chatType != "" && chatType != "dm" {
			if channelID := rawIDString(envelope.Payload.Message.Chat.ID); channelID != "" {
				return "scope:dc:" + channelID
			}
		}
	}

	userID := strings.TrimSpace(envelope.Payload.Message.From.ID.String())
	if userID == "" || userID == "0" {
		userID = strings.TrimSpace(envelope.Payload.DiscordAuthor.ID)
	}
	if userID == "" {
		// Unknown sender: keep it serialized instead of losing ordering.
		return "channel:" + envelope.Channel
	}
	return "user:" + userID
}

// telegramScopeKey mirrors the conversation scope: threads 0 and 1 are the
// chat-level bucket (non-forum groups and the General topic).
func telegramScopeKey(chatID, threadID json.RawMessage) string {
	chat := rawIDString(chatID)
	if chat == "" || chat == "0" {
		return ""
	}
	thread, err := strconv.Atoi(rawIDString(threadID))
	if err != nil || thread <= 1 {
		return "tg:" + chat
	}
	return fmt.Sprintf("tg:%s:%d", chat, thread)
}

// rawIDString reads an ID that may be encoded as a JSON number or a string.
// Discord uses string channel IDs, Telegram uses numbers.
func rawIDString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if strings.HasPrefix(s, `"`) {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return ""
		}
		return value
	}
	return s
}
