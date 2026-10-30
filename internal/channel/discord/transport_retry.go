package discord

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	discordMaxRetries    = 2
	discordRetryInterval = 500 * time.Millisecond
)

// retryingTransport adds retries for transient REST failures on top of
// discordgo, which only retries rate limits (429) itself.
//
// Editing is idempotent, so both 5xx and transport errors are retried. Sending
// is not: a dropped connection may mean the message was already delivered, so
// only error responses (which mean Discord rejected the request) are retried.
type retryingTransport struct {
	inner discordTransport
	sleep func(time.Duration)
}

func newRetryingTransport(inner discordTransport) discordTransport {
	if inner == nil {
		return nil
	}
	return &retryingTransport{inner: inner, sleep: time.Sleep}
}

func (t *retryingTransport) Send(channelID, content string) (string, error) {
	return t.retrySend(func() (string, error) {
		return t.inner.Send(channelID, content)
	})
}

func (t *retryingTransport) SendReply(channelID, replyTo, content string) (string, error) {
	return t.retrySend(func() (string, error) {
		return t.inner.SendReply(channelID, replyTo, content)
	})
}

func (t *retryingTransport) Edit(channelID, messageID, content string) error {
	var lastErr error
	for attempt := 0; attempt <= discordMaxRetries; attempt++ {
		if attempt > 0 {
			t.sleep(discordRetryDelay(attempt))
		}
		err := t.inner.Edit(channelID, messageID, content)
		if err == nil {
			return nil
		}
		lastErr = err
		if !discordRetryableEditError(err) {
			log.Printf("discord: edit failed permanently: %v", err)
			return err
		}
	}
	return lastErr
}

func (t *retryingTransport) retrySend(call func() (string, error)) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= discordMaxRetries; attempt++ {
		if attempt > 0 {
			t.sleep(discordRetryDelay(attempt))
		}
		id, err := call()
		if err == nil {
			return id, nil
		}
		lastErr = err
		// A transport error means the request may have reached Discord; do not
		// risk a duplicate message.
		if !discordServerError(err) {
			return "", err
		}
	}
	return "", lastErr
}

func discordRetryDelay(attempt int) time.Duration {
	return time.Duration(attempt) * discordRetryInterval
}

// discordServerError reports whether Discord answered with a 5xx status.
func discordServerError(err error) bool {
	var restErr *discordgo.RESTError
	if !errors.As(err, &restErr) || restErr.Response == nil {
		return false
	}
	status := restErr.Response.StatusCode
	return status >= http.StatusInternalServerError && status <= 599
}

// discordRetryableEditError also accepts transport failures, because an edit can
// be repeated safely.
func discordRetryableEditError(err error) bool {
	if discordServerError(err) {
		return true
	}
	var restErr *discordgo.RESTError
	// Discord responded (for example a 403), so retrying will not help.
	return !errors.As(err, &restErr)
}
