package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"myaaw/internal/config"

	"github.com/go-resty/resty/v2"
)

// streamIdleTimeout bounds the silence between stream chunks. A dropped
// connection is detected in this window instead of waiting for the whole
// stream budget (streamRequestTimeout), which defaults to 180s.
var streamIdleTimeout = 60 * time.Second

// streamAttempt is what one streaming request produced: whether output already
// reached the caller and whether repeating the request is safe.
type streamAttempt struct {
	delivered bool
	retryable bool
	err       error
}

// streamWithRetry runs a streaming request and retries it while nothing has been
// delivered to onEvent. Once a text or thinking delta reached the caller a retry
// would duplicate output, so the error is returned instead.
func streamWithRetry(
	ctx context.Context,
	do func(ctx context.Context) (*resty.Response, error),
	consume func(io.Reader, func(StreamEvent) error) error,
	onEvent func(StreamEvent) error,
) error {
	attempts := config.LLMRequestRetries
	if attempts < 0 {
		attempts = 0
	}

	var lastErr error
	for attempt := 0; attempt <= attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
			// Tell observers that this is a second (or later) request. The
			// engine turns it into a visible trace span; a failed attempt is
			// otherwise indistinguishable from a slow model.
			if err := onEvent(StreamEvent{Type: StreamAttempt, Attempt: attempt + 1, Err: lastErr}); err != nil {
				return err
			}
		}

		result := runStreamAttempt(ctx, do, consume, onEvent)
		if result.err == nil {
			return nil
		}
		lastErr = result.err
		if result.delivered || !result.retryable || ctx.Err() != nil {
			return result.err
		}
	}
	return lastErr
}

// runStreamAttempt sends one streaming request and consumes it. A retry is
// offered for transport failures, retryable HTTP statuses, empty streams, and
// idle stalls, but never after content was delivered or when the consumer
// itself failed.
func runStreamAttempt(
	ctx context.Context,
	do func(ctx context.Context) (*resty.Response, error),
	consume func(io.Reader, func(StreamEvent) error) error,
	onEvent func(StreamEvent) error,
) streamAttempt {
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	res, err := do(reqCtx)
	if err != nil {
		return streamAttempt{retryable: true, err: fmt.Errorf("stream request failed: %w", err)}
	}
	if res == nil {
		return streamAttempt{err: errors.New("stream request returned a nil response")}
	}
	if res.RawBody() == nil {
		return streamAttempt{err: errors.New("stream response has no body")}
	}
	defer res.RawBody().Close()

	if res.StatusCode() != 200 {
		body, _ := io.ReadAll(io.LimitReader(res.RawBody(), 64*1024))
		return streamAttempt{
			retryable: isRetryableStatus(res.StatusCode()),
			err: fmt.Errorf("stream request failed (%d): %s",
				res.StatusCode(), strings.TrimSpace(string(body))),
		}
	}

	idle := newIdleTimeoutReader(res.RawBody(), streamIdleTimeout, cancel)
	defer idle.Stop()

	delivered := false
	var consumerErr error
	streamErr := consume(idle, func(event StreamEvent) error {
		if event.Type == StreamTextDelta || event.Type == StreamThinkingDelta {
			delivered = true
		}
		if err := onEvent(event); err != nil {
			consumerErr = err
			return err
		}
		return nil
	})
	if consumerErr != nil {
		return streamAttempt{delivered: delivered, err: consumerErr}
	}
	if streamErr != nil {
		if idle.Stalled() {
			return streamAttempt{
				delivered: delivered,
				retryable: true,
				err:       fmt.Errorf("stream stalled: no data for %s: %w", streamIdleTimeout, streamErr),
			}
		}
		return streamAttempt{delivered: delivered, retryable: true, err: streamErr}
	}
	return streamAttempt{delivered: delivered}
}

// idleTimeoutReader cancels the request when no bytes arrive for timeout, so a
// silently dead connection fails fast instead of hanging until the stream
// budget runs out.
type idleTimeoutReader struct {
	r       io.Reader
	timeout time.Duration
	cancel  context.CancelFunc

	lastRead atomic.Int64
	stalled  atomic.Bool

	done     chan struct{}
	stopOnce sync.Once
}

func newIdleTimeoutReader(r io.Reader, timeout time.Duration, cancel context.CancelFunc) *idleTimeoutReader {
	ir := &idleTimeoutReader{r: r, timeout: timeout, cancel: cancel, done: make(chan struct{})}
	ir.lastRead.Store(time.Now().UnixNano())
	if timeout > 0 {
		go ir.watch()
	}
	return ir
}

func (ir *idleTimeoutReader) Read(p []byte) (int, error) {
	n, err := ir.r.Read(p)
	if n > 0 {
		ir.lastRead.Store(time.Now().UnixNano())
	}
	if err != nil {
		ir.Stop()
	}
	return n, err
}

// Stalled reports whether the watchdog gave up on a silent stream.
func (ir *idleTimeoutReader) Stalled() bool { return ir.stalled.Load() }

// Stop releases the watchdog. It is safe to call more than once.
func (ir *idleTimeoutReader) Stop() {
	ir.stopOnce.Do(func() { close(ir.done) })
}

func (ir *idleTimeoutReader) watch() {
	interval := ir.timeout / 4
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ir.done:
			return
		case <-ticker.C:
			last := time.Unix(0, ir.lastRead.Load())
			if time.Since(last) < ir.timeout {
				continue
			}
			ir.stalled.Store(true)
			if ir.cancel != nil {
				ir.cancel()
			}
			return
		}
	}
}
