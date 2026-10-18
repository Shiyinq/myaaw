package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/go-resty/resty/v2"

	"myaaw/internal/config"
)

// doWithRetry retries a provider request when the transport fails or the
// provider returns a transient status. Streaming requests are safe to retry
// here because this wrapper only covers the initial connection: once the
// caller starts reading a successful response, retries are no longer attempted.
func doWithRetry(request func() (*resty.Response, error)) (*resty.Response, error) {
	attempts := config.LLMRequestRetries
	if attempts < 0 {
		attempts = 0
	}

	var (
		lastResponse *resty.Response
		lastErr      error
	)

	for attempt := 0; attempt <= attempts; attempt++ {
		response, err := request()
		lastResponse, lastErr = response, err

		if err == nil && response != nil && !isRetryableStatus(response.StatusCode()) {
			return response, nil
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return response, err
			}
		} else if response != nil {
			lastErr = fmt.Errorf("provider returned status %d", response.StatusCode())
			drainAndClose(response)
		}

		if attempt < attempts {
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
		}
	}

	return lastResponse, lastErr
}

func isRetryableStatus(status int) bool {
	switch status {
	case 408, 425, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func drainAndClose(response *resty.Response) {
	if response == nil || response.RawBody() == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.RawBody(), 32*1024))
	_ = response.RawBody().Close()
}
