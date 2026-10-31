package handler

import (
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

func TestRunRegistryStopCancelsTheRun(t *testing.T) {
	ctx, _, release := runContext(context.Background(), "req-1")
	defer release()

	if !activeRuns.stop("req-1") {
		t.Fatal("stop reported no run")
	}

	select {
	case <-ctx.Done():
	default:
		t.Fatal("run context was not cancelled")
	}
	if !agent.StoppedByClient(ctx) {
		t.Fatalf("cause = %v, want ErrRunStopped", context.Cause(ctx))
	}
}

func TestRunRegistryReleaseForgetsTheRun(t *testing.T) {
	_, _, release := runContext(context.Background(), "req-2")
	release()

	if activeRuns.stop("req-2") {
		t.Fatal("a finished run is still stoppable")
	}
}

func TestRunRegistryUnknownRequestID(t *testing.T) {
	if activeRuns.stop("does-not-exist") {
		t.Fatal("unknown request id reported a run")
	}
}

func TestRunContextWithoutRequestIDIsNotRegistered(t *testing.T) {
	ctx, _, release := runContext(context.Background(), "")
	defer release()

	if ctx.Done() != nil {
		t.Fatal("a run without a request id should not be cancelable")
	}
	if activeRuns.stop("") {
		t.Fatal("empty request id was registered")
	}
}

func TestAPIChatStopCancelsRegisteredRun(t *testing.T) {
	app := newAPIApp(t, newFakeBotService())

	ctx, _, release := runContext(context.Background(), "req-stop")
	defer release()

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
	case <-ctx.Done():
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

// blockingBotService keeps a run alive until its context is cancelled, so the
// handler can be observed stopping it mid-stream.
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
