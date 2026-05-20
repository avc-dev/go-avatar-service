package avatar

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// withRetry runs fn under an exponential backoff schedule. On the first
// success it returns nil. After exhausting all delays it returns the last
// error wrapped with the op label and attempt count. ctx cancellation aborts
// immediately, even mid-sleep.
//
// The retry policy is intentionally simple: every error is treated as
// transient. Truly permanent errors (e.g. image decode failure on corrupt
// input) still get retried — the cost is bounded (at most sum(delays) of
// waste) and avoids the maintenance burden of a transient/permanent error
// taxonomy that won't catch every case anyway. Persistent failures end up at
// the DLX through the consumer.
func withRetry(ctx context.Context, log *slog.Logger, delays []time.Duration, op string, fn func(context.Context) error) error {
	maxAttempts := len(delays) + 1

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		if err := fn(ctx); err == nil {
			if attempt > 1 {
				log.Info(op+": succeeded after retry", "attempts", attempt)
			}
			return nil
		} else {
			lastErr = err
		}

		if attempt == maxAttempts {
			break
		}

		delay := delays[attempt-1]
		log.Warn(op+": attempt failed, retrying",
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"next_delay", delay,
			"err", lastErr,
		)
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", op, ctx.Err())
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("%s: exhausted %d attempts: %w", op, maxAttempts, lastErr)
}
