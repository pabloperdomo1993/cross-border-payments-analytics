package processing

import (
	"context"
	"time"
)

// Retry runs fn up to maxAttempts times total (the first attempt plus
// up to maxAttempts-1 retries), waiting backoff between attempts. It
// stops early if fn succeeds, if fn returns a non-retryable error, or
// if ctx is done.
//
// This is intentionally minimal: no retry topic, no external scheduler,
// no exponential backoff curve — just bounded in-process attempts. The
// caller is expected to derive ctx with an overall per-job timeout, so
// retrying here can never hold a worker beyond that bound regardless of
// maxAttempts/backoff.
func Retry(ctx context.Context, maxAttempts int, backoff time.Duration, fn func() error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !IsRetryable(err) || attempt == maxAttempts {
			return err
		}

		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return err
		}
	}
	return err
}
