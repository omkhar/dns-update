package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand" // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- retry jitter is not security-sensitive randomness.
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultMaxAttempts is the default maximum number of retry attempts.
	DefaultMaxAttempts  = 5
	defaultInitialDelay = 500 * time.Millisecond
	defaultMaxDelay     = 30 * time.Second
)

type retryableError struct {
	err        error
	retryAfter time.Duration
}

func (e *retryableError) Error() string {
	return e.err.Error()
}

func (e *retryableError) Unwrap() error {
	return e.err
}

// Policy defines bounded exponential backoff with jitter.
type Policy struct {
	MaxAttempts   int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	RandomFloat64 func() float64
	Sleep         func(context.Context, time.Duration) error
}

// DefaultPolicy returns the production retry policy.
func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts:   DefaultMaxAttempts,
		InitialDelay:  defaultInitialDelay,
		MaxDelay:      defaultMaxDelay,
		RandomFloat64: rand.Float64,
		Sleep:         sleep,
	}
}

// Mark wraps err as retryable and optionally attaches a Retry-After delay.
func Mark(err error, retryAfter time.Duration) error {
	if err == nil {
		return nil
	}
	if retryAfter < 0 {
		retryAfter = 0
	}
	return &retryableError{
		err:        err,
		retryAfter: retryAfter,
	}
}

// After returns the retry delay for err and whether the error is retryable.
func After(err error) (time.Duration, bool) {
	var retryable *retryableError
	if !errors.As(err, &retryable) {
		return 0, false
	}
	return retryable.retryAfter, true
}

// ShouldRetryHTTPStatus reports whether statusCode is usually safe to retry.
func ShouldRetryHTTPStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// ParseRetryAfter parses an HTTP Retry-After value.
func ParseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}

	seconds, err := strconv.Atoi(trimmed)
	if err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}

	timestamp, err := http.ParseTime(trimmed)
	if err != nil {
		return 0, false
	}
	if timestamp.After(now) {
		return timestamp.Sub(now), true
	}
	return 0, true
}

// Delay returns the bounded exponential backoff delay for attempt.
func (p Policy) Delay(attempt int, retryAfter time.Duration) time.Duration {
	normalized := p.normalized()
	delay := normalized.InitialDelay
	for currentAttempt := 1; currentAttempt < attempt; currentAttempt++ {
		if delay >= normalized.MaxDelay/2 {
			delay = normalized.MaxDelay
			break
		}
		delay *= 2
	}

	randomFloat64 := normalized.RandomFloat64
	jittered := delay
	if delay > 0 {
		jitterMultiplier := 0.5 + (randomFloat64() * 0.5)
		jittered = time.Duration(float64(delay) * jitterMultiplier)
	}
	if retryAfter > jittered {
		jittered = retryAfter
	}
	if jittered > normalized.MaxDelay {
		return normalized.MaxDelay
	}
	return jittered
}

// Wait sleeps for the calculated delay or until ctx is canceled.
func (p Policy) Wait(ctx context.Context, delay time.Duration) error {
	return p.normalized().Sleep(ctx, delay)
}

// CanRetry reports whether another retry attempt is allowed.
func (p Policy) CanRetry(attempt int, err error) (time.Duration, bool) {
	retryAfter, ok := After(err)
	if !ok {
		return 0, false
	}
	if attempt >= p.normalized().MaxAttempts {
		return 0, false
	}
	return retryAfter, true
}

func (p Policy) normalized() Policy {
	normalized := p
	if normalized.MaxAttempts <= 0 {
		normalized.MaxAttempts = DefaultMaxAttempts
	}
	if normalized.InitialDelay <= 0 {
		normalized.InitialDelay = defaultInitialDelay
	}
	if normalized.MaxDelay <= 0 {
		normalized.MaxDelay = defaultMaxDelay
	}
	if normalized.MaxDelay < normalized.InitialDelay {
		normalized.MaxDelay = normalized.InitialDelay
	}
	if normalized.RandomFloat64 == nil {
		normalized.RandomFloat64 = rand.Float64
	}
	if normalized.Sleep == nil {
		normalized.Sleep = sleep
	}
	return normalized
}

func sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("wait before retry: %w", context.Cause(ctx))
	case <-timer.C:
		return nil
	}
}
