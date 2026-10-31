package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/channel"
	"myaaw/internal/services/bot/service"

	"github.com/gofiber/fiber/v3"
)

func TestRunBrokerStopCancelsTheRun(t *testing.T) {
	run := activeRuns.start("req-1")
	defer run.finish()

	if !activeRuns.stop("req-1") {
		t.Fatal("stop reported no run")
	}

	select {
	case <-run.Context().Done():
	default:
		t.Fatal("run context was not cancelled")
	}
	if !agent.StoppedByClient(run.Context()) {
		t.Fatalf("cause = %v, want ErrRunStopped", context.Cause(run.Context()))
	}
}

func TestRunBrokerUnknownRequestID(t *testing.T) {
	if activeRuns.stop("does-not-exist") {
		t.Fatal("unknown request id reported a run")
	}
	if _, ok := activeRuns.get("does-not-exist"); ok {
		t.Fatal("unknown request id was found")
	}
}

func TestRunBrokerListsAndFiltersRuns(t *testing.T) {
	first := activeRuns.start("list-a")
	first.setConversation("conv-a")
	second := activeRuns.start("list-b")
	second.setConversation("conv-b")
	second.finish()

	everything := activeRuns.list("")
	if len(everything) < 2 {
		t.Fatalf("runs = %+v", everything)
	}

	onlyA := activeRuns.list("conv-a")
	if len(onlyA) != 1 || onlyA[0].RequestID != "list-a" || !onlyA[0].Streaming {
		t.Fatalf("conv-a runs = %+v", onlyA)
	}

	onlyB := activeRuns.list("conv-b")
	if len(onlyB) != 1 || onlyB[0].Streaming {
		t.Fatalf("conv-b runs = %+v", onlyB)
	}
}

// The whole point of the broker: events are buffered independently of any
// connection, so a client that attaches late still receives them.
func TestChatRunAttachReplaysAndFollows(t *testing.T) {
	run := newChatRun("replay")
	run.publish("chunk", map[string]any{"text": "a"})
	run.publish("chunk", map[string]any{"text": "b"})

	go func() {
		time.Sleep(30 * time.Millisecond)
		run.publish("done", map[string]any{"ok": true})
		run.finish()
	}()

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	done := make(chan struct{})
	go func() {
		run.attach(writer, 0)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("attach did not finish after the run completed")
	}

	stream := buf.String()
	for _, want := range []string{`"text":"a"`, `"text":"b"`, `"ok":true`} {
		if !strings.Contains(stream, want) {
			t.Fatalf("stream missing %q:\n%s", want, stream)
		}
	}
}

func TestChatRunAttachResumesAfterIndex(t *testing.T) {
	run := newChatRun("resume")
	run.publish("chunk", map[string]any{"text": "a"})
	run.publish("chunk", map[string]any{"text": "b"})
	run.finish()

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	run.attach(writer, 1)

	stream := buf.String()
	if strings.Contains(stream, `"text":"a"`) {
		t.Fatalf("attach replayed an event before the offset:\n%s", stream)
	}
	if !strings.Contains(stream, `"text":"b"`) {
		t.Fatalf("stream missing the later event:\n%s", stream)
	}
}

func TestAPIChatStopCancelsRegisteredRun(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	run := activeRuns.start("req-stop")
	defer run.finish()

	req := httptest.NewRequest("POST", "/api/chat/stop", strings.NewReader(`{"request_id":"req-stop"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, payload)
	}

	var got map[string]bool
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v, body = %s", err, payload)
	}
	if !got["stopped"] {
		t.Fatalf("response = %s", payload)
	}
	select {
	case <-run.Context().Done():
	default:
		t.Fatal("run was not cancelled")
	}
}

func TestAPIChatStopRequiresRequestID(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	req := httptest.NewRequest("POST", "/api/chat/stop", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAPIChatRunsListsRegisteredRuns(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	run := activeRuns.start("listed-run")
	run.setConversation("conv-listed")
	defer run.finish()

	resp, err := app.Test(httptest.NewRequest("GET", "/api/chat/runs?conversation_id=conv-listed", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Runs []RunInfo `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Runs) != 1 || payload.Runs[0].RequestID != "listed-run" || !payload.Runs[0].Streaming {
		t.Fatalf("runs = %+v", payload.Runs)
	}
}

// blockingBotService keeps a run alive until its context is cancelled, so the
// handler can be observed stopping it.
type blockingBotService struct {
	service.BotService
	started chan struct{}
	stopped chan bool
}

func (b *blockingBotService) BotStreamContext(
	ctx context.Context,
	_ *channel.IncomingMessage,
	onChunk func(channel.StreamChunk),
) (*channel.OutgoingMessage, error) {
	onChunk(channel.StreamChunk{Kind: "text", Text: "partial"})
	close(b.started)
	<-ctx.Done()
	b.stopped <- agent.StoppedByClient(ctx)
	return &channel.OutgoingMessage{Text: "partial", Stopped: true}, nil
}

func TestAPIChatStreamRegistersRunForStop(t *testing.T) {
	svc := &blockingBotService{started: make(chan struct{}), stopped: make(chan bool, 1)}
	app := newAPIAppWith(t, svc)

	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest("POST", "/api/chat/stream",
			strings.NewReader(`{"user_id":7,"text":"hi","request_id":"req-stream"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()

	<-svc.started
	if !activeRuns.stop("req-stream") {
		t.Fatal("stream run was not registered")
	}
	<-done

	if !<-svc.stopped {
		t.Fatal("stream run was not cancelled with ErrRunStopped")
	}
}
