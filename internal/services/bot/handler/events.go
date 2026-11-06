package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"time"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/agentdir"
	"myaaw/internal/services/bot/events"

	"github.com/gofiber/fiber/v3"
)

// eventKeepAlive keeps an idle stream from being reaped by an idle proxy.
const eventKeepAlive = 20 * time.Second

// APIEvents streams conversation updates as Server-Sent Events. A client
// subscribes once per agent and re-reads the transcript when an event arrives;
// the payload only says which conversation changed, so no transcript is carried
// over the wire.
//
// SendStreamWriter runs its callback in a goroutine and returns immediately, so
// the subscription lives inside the callback: it is created when streaming
// starts and cancelled when the stream ends, never before it.
//
// APIEvents
// @Summary		Conversation events
// @Description	A `conversation.updated` event is emitted when a cron, heartbeat, or sub-agent turn lands. The payload names the conversation; read it for the transcript. A `HEARTBEAT_OK` turn is not stored, so it emits nothing.
// @Tags			API
// @Produce		text/event-stream
// @Success		200
// @Router		/api/events [get]
func (s *BotHandlerImpl) APIEvents(c fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	agentName := agentdir.Active()

	c.SendStreamWriter(func(w *bufio.Writer) {
		ch, cancel := events.Default().Subscribe()
		defer cancel()
		approvalsCh, cancelApprovals := approvals.DefaultBroker().Subscribe()
		defer cancelApprovals()

		// An opening comment flushes the headers immediately.
		if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}

		keepAlive := time.NewTicker(eventKeepAlive)
		defer keepAlive.Stop()

		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}
				payload := struct {
					Agent string `json:"agent"`
					events.TurnEvent
				}{Agent: agentName, TurnEvent: event}

				data, err := json.Marshal(payload)
				if err != nil {
					continue
				}
				if _, err := fmt.Fprintf(w, "event: conversation.updated\ndata: %s\n\n", data); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			case approval, ok := <-approvalsCh:
				if !ok {
					return
				}
				kind := "approval.required"
				if approval.Resolved {
					kind = "approval.resolved"
				}
				payload := struct {
					Agent string `json:"agent"`
					approvals.Event
				}{Agent: agentName, Event: approval}

				data, err := json.Marshal(payload)
				if err != nil {
					continue
				}
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			case <-keepAlive.C:
				if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
					return
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})
	return nil
}
