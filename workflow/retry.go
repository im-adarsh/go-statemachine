package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// RetryPolicy
// ─────────────────────────────────────────────────────────────────────────────

// RetryPolicy configures the retry behaviour for an Activity wrapped with Retry().
// Zero values produce sensible defaults (see withDefaults).
type RetryPolicy struct {
	// MaxAttempts is the maximum number of times the Activity is called.
	// 0 means unlimited; the Activity is retried until it succeeds or
	// the context is cancelled.
	MaxAttempts int

	// InitialInterval is the wait time before the second attempt.
	// Defaults to 1 second.
	InitialInterval time.Duration

	// BackoffCoefficient multiplies the interval after each failure.
	// 1.0 = constant interval, 2.0 = exponential backoff. Defaults to 2.0.
	BackoffCoefficient float64

	// MaxInterval caps the wait time between attempts. 0 means uncapped.
	MaxInterval time.Duration

	// NonRetryableErrors lists errors that abort the retry loop immediately,
	// even when MaxAttempts has not been reached.
	NonRetryableErrors []error
}

func (p RetryPolicy) withDefaults() RetryPolicy {
	if p.InitialInterval <= 0 {
		p.InitialInterval = time.Second
	}
	if p.BackoffCoefficient <= 0 {
		p.BackoffCoefficient = 2.0
	}
	return p
}

// ─────────────────────────────────────────────────────────────────────────────
// Activity wrappers
// ─────────────────────────────────────────────────────────────────────────────

// Retry returns an Activity that transparently retries fn according to policy.
// The returned Activity satisfies ErrRetryExhausted (wrapped) when all attempts fail.
//
//	var policy = workflow.RetryPolicy{MaxAttempts: 3, InitialInterval: 500 * time.Millisecond}
//	.Activity(workflow.Retry(callExternalAPI, policy))
func Retry[T any](fn Activity[T], policy RetryPolicy) Activity[T] {
	policy = policy.withDefaults()
	return func(ctx context.Context, payload T) error {
		interval := policy.InitialInterval
		for attempt := 1; ; attempt++ {
			err := fn(ctx, payload)
			if err == nil {
				return nil
			}

			// Abort immediately on non-retryable errors.
			for _, nr := range policy.NonRetryableErrors {
				if errors.Is(err, nr) {
					return err
				}
			}

			// Abort when max attempts reached.
			if policy.MaxAttempts > 0 && attempt >= policy.MaxAttempts {
				return fmt.Errorf("%w (attempts=%d): %w", ErrRetryExhausted, attempt, err)
			}

			// Wait before next attempt, but respect context cancellation.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}

			// Exponential backoff with optional cap.
			next := time.Duration(float64(interval) * policy.BackoffCoefficient)
			if policy.MaxInterval > 0 && next > policy.MaxInterval {
				next = policy.MaxInterval
			}
			interval = next
		}
	}
}

// Timeout returns an Activity that fails with context.DeadlineExceeded if fn
// does not complete within d.
//
//	.Activity(workflow.Timeout(callThirdParty, 5*time.Second))
func Timeout[T any](fn Activity[T], d time.Duration) Activity[T] {
	return func(ctx context.Context, payload T) error {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return fn(ctx, payload)
	}
}

// WithMiddleware composes mw around fn, with the first element being the
// outermost wrapper. It is a convenience for per-activity middleware;
// for workflow-wide middleware prefer Builder.WithMiddleware.
//
//	.Activity(workflow.WithMiddleware(chargeCard, otelMiddleware, metricsMiddleware))
func WithMiddleware[T any](fn Activity[T], mw ...Middleware[T]) Activity[T] {
	result := fn
	// Apply in reverse so that mw[0] is the outermost (first to execute).
	for i := len(mw) - 1; i >= 0; i-- {
		result = mw[i](result)
	}
	return result
}
