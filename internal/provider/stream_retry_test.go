package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"myaaw/internal/config"
)

// withStreamRetries pins the retry budget and idle timeout for a test.
func withStreamRetries(t *testing.T, retries int, idle time.Duration) {
	t.Helper()

	oldRetries := config.LLMRequestRetries
	oldIdle := streamIdleTimeout
	config.LLMRequestRetries = retries
	streamIdleTimeout = idle
	t.Cleanup(func() {
		config.LLMRequestRetries = oldRetries
		streamIdleTimeout = oldIdle
	})
}

func writeSSEText(w http.ResponseWriter, text string) {
	fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", text)
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func collectStreamText(t *testing.T, p LLMProvider) (string, error) {
	t.Helper()

	var text strings.Builder
	err := p.ChatStreamEvents(context.Background(), "test-model", []Message{{Role: "user", Content: "hi"}}, nil,
		func(event StreamEvent) error {
			if event.Type == StreamTextDelta {
				text.WriteString(event.Text)
			}
			return nil
		})
	return text.String(), err
}

// A retried request is announced so observers can record each attempt.
func TestStreamReportsTheRetriedAttempt(t *testing.T) {
	withStreamRetries(t, 1, 5*time.Second)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			return
		}
		writeSSEText(w, "ok")
	}))
	defer server.Close()

	var attempts []int
	err := NewOpenAIProvider(server.URL, "sk-test", "test-model").ChatStreamEvents(
		context.Background(), "test-model", []Message{{Role: "user", Content: "hi"}}, nil,
		func(event StreamEvent) error {
			if event.Type != StreamAttempt {
				return nil
			}
			if event.Err == nil {
				t.Error("attempt event did not carry the previous error")
			}
			attempts = append(attempts, event.Attempt)
			return nil
		})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if len(attempts) != 1 || attempts[0] != 2 {
		t.Fatalf("attempts = %+v, want one retry numbered 2", attempts)
	}
}

// A connection that dies before any token reached the user is retried.
func TestStreamRetriesWhenNothingDelivered(t *testing.T) {
	withStreamRetries(t, 1, 5*time.Second)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			// 200 with an empty body: the stream ends before any data.
			return
		}
		writeSSEText(w, "ok")
	}))
	defer server.Close()

	text, err := collectStreamText(t, NewOpenAIProvider(server.URL, "sk-test", "test-model"))
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if text != "ok" {
		t.Fatalf("text = %q, want ok", text)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

// Once text was streamed, retrying would duplicate it, so the error surfaces.
func TestStreamDoesNotRetryAfterDelta(t *testing.T) {
	withStreamRetries(t, 1, 5*time.Second)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		// Announce more bytes than are sent so the connection drops mid-stream.
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer server.Close()

	text, err := collectStreamText(t, NewOpenAIProvider(server.URL, "sk-test", "test-model"))
	if err == nil {
		t.Fatal("expected the truncated stream to fail")
	}
	if text != "partial" {
		t.Fatalf("text = %q, want the partial delta", text)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1 (no retry after output)", got)
	}
}

// A silent connection is detected by the idle watchdog and retried.
func TestStreamRetriesWhenStalled(t *testing.T) {
	withStreamRetries(t, 1, 150*time.Millisecond)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if n == 1 {
			<-r.Context().Done() // go silent until the client gives up
			return
		}
		writeSSEText(w, "ok")
	}))
	defer server.Close()

	start := time.Now()
	text, err := collectStreamText(t, NewOpenAIProvider(server.URL, "sk-test", "test-model"))
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if text != "ok" {
		t.Fatalf("text = %q, want ok", text)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("stall detection took %s, want the idle timeout to fire", elapsed)
	}
}

// The retry budget is bounded: a permanently failing stream ends in an error.
func TestStreamStopsAfterRetryBudget(t *testing.T) {
	withStreamRetries(t, 1, 5*time.Second)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if _, err := collectStreamText(t, NewOpenAIProvider(server.URL, "sk-test", "test-model")); err == nil {
		t.Fatal("expected the stream to fail")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2 (initial + one retry)", got)
	}
}

// A consumer error must never be retried: it would re-run the turn for nothing.
func TestStreamDoesNotRetryConsumerError(t *testing.T) {
	withStreamRetries(t, 1, 5*time.Second)

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSEText(w, "ok")
	}))
	defer server.Close()

	sentinel := errors.New("consumer stopped")
	err := NewOpenAIProvider(server.URL, "sk-test", "test-model").ChatStreamEvents(
		context.Background(), "test-model", []Message{{Role: "user", Content: "hi"}}, nil,
		func(event StreamEvent) error {
			if event.Type == StreamTextDelta {
				return sentinel
			}
			return nil
		})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the consumer error", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

func TestIdleTimeoutReaderReportsStall(t *testing.T) {
	reader, writer := io.Pipe()

	idle := newIdleTimeoutReader(reader, 50*time.Millisecond, func() {
		_ = writer.CloseWithError(errors.New("stalled"))
	})
	defer idle.Stop()

	buf := make([]byte, 16)
	if _, err := idle.Read(buf); err == nil {
		t.Fatal("expected the stalled read to fail")
	}
	if !idle.Stalled() {
		t.Fatal("stall was not reported")
	}
}

func TestIdleTimeoutReaderStaysQuietWhileDataFlows(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close() })

	idle := newIdleTimeoutReader(reader, 80*time.Millisecond, func() {})
	defer idle.Stop()

	go func() {
		for range 5 {
			if _, err := writer.Write([]byte("x")); err != nil {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		_ = writer.Close()
	}()

	for range 5 {
		buf := make([]byte, 16)
		if _, err := idle.Read(buf); err != nil {
			break
		}
	}
	if idle.Stalled() {
		t.Fatal("a stream with steady data must not be marked stalled")
	}
}
