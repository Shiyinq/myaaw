package telegram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"myaaw/internal/channel"
	"myaaw/internal/config"

	"github.com/go-resty/resty/v2"
)

// withTelegramTestServer points the package at a local server for the duration
// of the test.
func withTelegramTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	oldBase := telegramAPIBaseURL
	oldToken := config.TelegramBotToken
	telegramAPIBaseURL = server.URL
	config.TelegramBotToken = "test-token"
	t.Cleanup(func() {
		telegramAPIBaseURL = oldBase
		config.TelegramBotToken = oldToken
	})
	return server
}

func okTyping(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// The typing indicator is cosmetic: a transport failure used to call
// log.Fatalf and take the whole agent down.
func TestTypingActionFailureIsNotFatal(t *testing.T) {
	oldBase := telegramAPIBaseURL
	telegramAPIBaseURL = "http://127.0.0.1:1"
	t.Cleanup(func() { telegramAPIBaseURL = oldBase })

	done := make(chan error, 1)
	go func() { done <- sendTelegramTypingAction(1) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected the typing request to fail")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("typing indicator hung instead of failing fast")
	}
}

func TestSendRetriesServerErrors(t *testing.T) {
	var sends atomic.Int32
	withTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendChatAction") {
			okTyping(w)
			return
		}
		if sends.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"ok":false,"description":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
	})

	res, err := SendTelegramMessage(1, 0, "hi", "")
	if err != nil {
		t.Fatalf("SendTelegramMessage: %v", err)
	}
	if res.Result.MessageId != 7 {
		t.Fatalf("result = %+v", res)
	}
	if got := sends.Load(); got != 3 {
		t.Fatalf("send attempts = %d, want 3", got)
	}
}

func TestSendRetriesRateLimit(t *testing.T) {
	var sends atomic.Int32
	withTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendChatAction") {
			okTyping(w)
			return
		}
		if sends.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"parameters":{"retry_after":0}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":9}}`))
	})

	if _, err := SendTelegramMessage(1, 0, "hi", ""); err != nil {
		t.Fatalf("SendTelegramMessage: %v", err)
	}
	if got := sends.Load(); got != 2 {
		t.Fatalf("send attempts = %d, want 2", got)
	}
}

func TestSendDoesNotRetryBadRequest(t *testing.T) {
	var sends atomic.Int32
	withTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendChatAction") {
			okTyping(w)
			return
		}
		sends.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"bad"}`))
	})

	if _, err := SendTelegramMessage(1, 0, "hi", ""); err == nil {
		t.Fatal("expected a 400 to fail")
	}
	if got := sends.Load(); got != 1 {
		t.Fatalf("send attempts = %d, want 1", got)
	}
}

// A dropped connection after sendMessage may mean the message was already
// delivered, so retrying could duplicate it.
func TestSendDoesNotRetryTransportErrors(t *testing.T) {
	var sends atomic.Int32
	withTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendChatAction") {
			okTyping(w)
			return
		}
		sends.Add(1)
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Skip("hijacking is not supported")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		_ = conn.Close()
	})

	if _, err := SendTelegramMessage(1, 0, "hi", ""); err == nil {
		t.Fatal("expected the dropped connection to fail")
	}
	if got := sends.Load(); got != 1 {
		t.Fatalf("send attempts = %d, want 1", got)
	}
}

// Editing is idempotent, so a transport failure is safe to retry.
func TestEditRetriesTransportErrors(t *testing.T) {
	var edits atomic.Int32
	withTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/editMessageText") {
			okTyping(w)
			return
		}
		if edits.Add(1) == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Skip("hijacking is not supported")
			}
			conn, _, err := hijacker.Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":9}}`))
	})

	if _, err := EditTelegramMessage(1, 0, 9, "hi", ""); err != nil {
		t.Fatalf("EditTelegramMessage: %v", err)
	}
	if got := edits.Load(); got != 2 {
		t.Fatalf("edit attempts = %d, want 2", got)
	}
}

func TestOffsetStoreRoundTrip(t *testing.T) {
	store := &offsetStore{path: filepath.Join(t.TempDir(), "state", "telegram_offset")}

	if got := store.Load(); got != 0 {
		t.Fatalf("fresh store = %d, want 0", got)
	}
	store.Save(42)
	if got := store.Load(); got != 42 {
		t.Fatalf("loaded offset = %d, want 42", got)
	}
}

type fakeQueue struct {
	envelopes []channel.QueueEnvelope
}

func (q *fakeQueue) ProcessAndPublishMessage(envelope *channel.QueueEnvelope) error {
	q.envelopes = append(q.envelopes, *envelope)
	return nil
}

func TestPollUpdatesResumesFromSavedOffset(t *testing.T) {
	store := &offsetStore{path: filepath.Join(t.TempDir(), "state", "telegram_offset")}
	store.Save(100)

	var sentOffset string
	withTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		sentOffset = r.URL.Query().Get("offset")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[
			{"update_id":98,"message":{"message_id":1,"text":"old"}},
			{"update_id":100,"message":{"message_id":2,"text":"new"}}
		]}`))
	})

	queue := &fakeQueue{}
	next := pollUpdates(resty.New().SetTimeout(5*time.Second), store.Load(), store, queue)

	if sentOffset != "100" {
		t.Fatalf("getUpdates offset = %q, want 100", sentOffset)
	}
	if len(queue.envelopes) != 1 {
		t.Fatalf("processed %d updates, want 1 (the stale one must be skipped)", len(queue.envelopes))
	}
	var payload struct {
		UpdateID int64 `json:"update_id"`
	}
	if err := json.Unmarshal(queue.envelopes[0].Payload, &payload); err != nil {
		t.Fatalf("persisted payload: %v", err)
	}
	if payload.UpdateID != 100 {
		t.Fatalf("processed update %d, want 100", payload.UpdateID)
	}
	if next != 101 {
		t.Fatalf("next offset = %d, want 101", next)
	}
	if got := store.Load(); got != 101 {
		t.Fatalf("persisted offset = %d, want 101", got)
	}
}
