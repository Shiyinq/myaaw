package discord

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

type scriptedTransport struct {
	sendErrs  []error
	replyErrs []error
	editErrs  []error

	sends   int
	replies int
	edits   int
}

func errAt(list []error, n int) error {
	if n < len(list) {
		return list[n]
	}
	return nil
}

func (s *scriptedTransport) Send(channelID, content string) (string, error) {
	err := errAt(s.sendErrs, s.sends)
	s.sends++
	if err != nil {
		return "", err
	}
	return "m1", nil
}

func (s *scriptedTransport) SendReply(channelID, replyTo, content string) (string, error) {
	err := errAt(s.replyErrs, s.replies)
	s.replies++
	if err != nil {
		return "", err
	}
	return "m2", nil
}

func (s *scriptedTransport) Edit(channelID, messageID, content string) error {
	err := errAt(s.editErrs, s.edits)
	s.edits++
	return err
}

func restStatus(status int) error {
	return &discordgo.RESTError{Response: &http.Response{StatusCode: status}}
}

func transportFailure() error { return errors.New("connection reset by peer") }

func newTestRetryTransport(inner discordTransport) *retryingTransport {
	return &retryingTransport{inner: inner, sleep: func(time.Duration) {}}
}

func TestRetryingTransportRetriesSendServerErrors(t *testing.T) {
	inner := &scriptedTransport{sendErrs: []error{restStatus(500), restStatus(503)}}
	transport := newTestRetryTransport(inner)

	id, err := transport.Send("chan", "hi")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id != "m1" {
		t.Fatalf("id = %q", id)
	}
	if inner.sends != 3 {
		t.Fatalf("send attempts = %d, want 3", inner.sends)
	}
}

// A dropped connection may mean the message was delivered, so sending is not
// retried on transport errors.
func TestRetryingTransportDoesNotRetrySendTransportErrors(t *testing.T) {
	inner := &scriptedTransport{sendErrs: []error{transportFailure()}}
	transport := newTestRetryTransport(inner)

	if _, err := transport.Send("chan", "hi"); err == nil {
		t.Fatal("expected the send to fail")
	}
	if inner.sends != 1 {
		t.Fatalf("send attempts = %d, want 1", inner.sends)
	}
}

func TestRetryingTransportRetriesEditTransportErrors(t *testing.T) {
	inner := &scriptedTransport{editErrs: []error{transportFailure()}}
	transport := newTestRetryTransport(inner)

	if err := transport.Edit("chan", "m1", "hi"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if inner.edits != 2 {
		t.Fatalf("edit attempts = %d, want 2", inner.edits)
	}
}

func TestRetryingTransportDoesNotRetryEditClientErrors(t *testing.T) {
	inner := &scriptedTransport{editErrs: []error{restStatus(http.StatusForbidden)}}
	transport := newTestRetryTransport(inner)

	if err := transport.Edit("chan", "m1", "hi"); err == nil {
		t.Fatal("expected the edit to fail")
	}
	if inner.edits != 1 {
		t.Fatalf("edit attempts = %d, want 1", inner.edits)
	}
}

func TestDiscordServerErrorUnwrapsWrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("send failed: %w", restStatus(502))
	if !discordServerError(wrapped) {
		t.Fatal("wrapped 5xx should be retryable")
	}
	if discordServerError(transportFailure()) {
		t.Fatal("transport errors are not server errors")
	}
}
