package handler

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"myaaw/internal/services/bot/events"

	"github.com/gofiber/fiber/v3"
)

// The whole feature depends on one fact: a turn published by the bot service
// reaches an SSE client subscribed to the agent. Pin that here, without a
// provider or a database.
func TestAPIEventsStreamsPublishedTurns(t *testing.T) {
	app := fiber.New()
	h := &BotHandlerImpl{}
	app.Get("/api/events", h.APIEvents)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Listener(ln) }()
	// The stream stays open until the client disconnects; a blocking Shutdown
	// would wait for it, so release the listener out of band.
	t.Cleanup(func() { go func() { _ = app.Shutdown() }() })

	resp, err := http.Get("http://" + ln.Addr().String() + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	reader := bufio.NewReader(resp.Body)
	if err := readUntil(reader, ": connected", time.Second); err != nil {
		t.Fatalf("opening comment: %v", err)
	}

	events.Default().Publish(events.TurnEvent{
		ConversationID: "conv-1",
		Trigger:        "cron",
		At:             "2026-11-06T18:00:00Z",
	})

	if err := readUntil(reader, "event: conversation.updated", 2*time.Second); err != nil {
		t.Fatalf("event: %v", err)
	}
	dataLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("data line: %v", err)
	}
	for _, want := range []string{`"conversation_id":"conv-1"`, `"trigger":"cron"`} {
		if !strings.Contains(dataLine, want) {
			t.Fatalf("data line %q missing %q", dataLine, want)
		}
	}
}

func readUntil(reader *bufio.Reader, marker string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if strings.Contains(line, marker) {
			return nil
		}
	}
	return &timeoutError{marker: marker}
}

type timeoutError struct{ marker string }

func (e *timeoutError) Error() string { return "timed out waiting for " + e.marker }
