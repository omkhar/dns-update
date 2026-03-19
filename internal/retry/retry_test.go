package retry

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestMarkAndAfter(t *testing.T) {
	t.Parallel()

	if got := Mark(nil, time.Second); got != nil {
		t.Fatalf("Mark(nil) = %v, want nil", got)
	}

	baseErr := errors.New("boom")
	err := Mark(baseErr, time.Second)
	if !errors.Is(err, baseErr) {
		t.Fatal("errors.Is(retryable, boom) = false, want true")
	}
	delay, ok := After(err)
	if !ok {
		t.Fatal("After() ok = false, want true")
	}
	if got, want := delay, time.Second; got != want {
		t.Fatalf("After() = %v, want %v", got, want)
	}
	if _, ok := After(errors.New("boom")); ok {
		t.Fatal("After(non-retryable) ok = true, want false")
	}

	negativeDelay, ok := After(Mark(errors.New("boom"), -time.Second))
	if !ok {
		t.Fatal("After(negative delay) ok = false, want true")
	}
	if negativeDelay != 0 {
		t.Fatalf("After(negative delay) = %v, want 0", negativeDelay)
	}
}

func TestShouldRetryHTTPStatus(t *testing.T) {
	t.Parallel()

	for _, statusCode := range []int{
		http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		if !ShouldRetryHTTPStatus(statusCode) {
			t.Fatalf("ShouldRetryHTTPStatus(%d) = false, want true", statusCode)
		}
	}
	if ShouldRetryHTTPStatus(http.StatusBadRequest) {
		t.Fatal("ShouldRetryHTTPStatus(400) = true, want false")
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 17, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{name: "empty", value: "", want: 0, ok: false},
		{name: "seconds", value: "3", want: 3 * time.Second, ok: true},
		{name: "negative", value: "-1", want: 0, ok: false},
		{name: "date future", value: now.Add(2 * time.Second).Format(http.TimeFormat), want: 2 * time.Second, ok: true},
		{name: "date past", value: now.Add(-2 * time.Second).Format(http.TimeFormat), want: 0, ok: true},
		{name: "invalid", value: "not-a-date", want: 0, ok: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseRetryAfter(test.value, now)
			if ok != test.ok {
				t.Fatalf("ParseRetryAfter() ok = %t, want %t", ok, test.ok)
			}
			if got != test.want {
				t.Fatalf("ParseRetryAfter() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPolicyDelay(t *testing.T) {
	t.Parallel()

	policy := Policy{
		MaxAttempts:   4,
		InitialDelay:  time.Second,
		MaxDelay:      5 * time.Second,
		RandomFloat64: func() float64 { return 0 },
	}

	if got, want := policy.Delay(1, 0), 500*time.Millisecond; got != want {
		t.Fatalf("Delay(1) = %v, want %v", got, want)
	}
	if got, want := policy.Delay(3, 0), 2*time.Second; got != want {
		t.Fatalf("Delay(3) = %v, want %v", got, want)
	}
	if got, want := policy.Delay(10, 0), 2500*time.Millisecond; got != want {
		t.Fatalf("Delay(10) = %v, want %v", got, want)
	}
	if got, want := policy.Delay(1, 4*time.Second), 4*time.Second; got != want {
		t.Fatalf("Delay(1, retryAfter) = %v, want %v", got, want)
	}
	if got, want := policy.Delay(1, 10*time.Second), 5*time.Second; got != want {
		t.Fatalf("Delay(1, large retryAfter) = %v, want %v", got, want)
	}
}

func TestPolicyCanRetry(t *testing.T) {
	t.Parallel()

	policy := Policy{MaxAttempts: 2}
	if _, ok := policy.CanRetry(1, errors.New("boom")); ok {
		t.Fatal("CanRetry(non-retryable) ok = true, want false")
	}

	retryable := Mark(errors.New("boom"), time.Second)
	if delay, ok := policy.CanRetry(1, retryable); !ok || delay != time.Second {
		t.Fatalf("CanRetry() = %v, %t, want %v, true", delay, ok, time.Second)
	}
	if _, ok := policy.CanRetry(2, retryable); ok {
		t.Fatal("CanRetry(max attempts) ok = true, want false")
	}
}

func TestPolicyWait(t *testing.T) {
	t.Parallel()

	policy := Policy{
		Sleep: func(context.Context, time.Duration) error {
			return errors.New("boom")
		},
	}
	if err := policy.Wait(context.Background(), time.Second); err == nil {
		t.Fatal("Wait() error = nil, want custom sleep error")
	}

	if err := (Policy{Sleep: func(context.Context, time.Duration) error { return nil }}).Wait(context.Background(), 0); err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
}

func TestPolicyNormalizedBranches(t *testing.T) {
	t.Parallel()

	policy := Policy{
		InitialDelay: time.Second,
		MaxDelay:     time.Millisecond,
	}
	if got := policy.Delay(1, 0); got < 500*time.Millisecond || got > time.Second {
		t.Fatalf("Delay() = %v, want value in [500ms, 1s]", got)
	}

	if delay, ok := (Policy{}).CanRetry(1, Mark(errors.New("boom"), 0)); !ok || delay != 0 {
		t.Fatalf("CanRetry() = %v, %t, want 0, true", delay, ok)
	}
}

func TestSleep(t *testing.T) {
	t.Parallel()

	if err := sleep(context.Background(), 0); err != nil {
		t.Fatalf("sleep(0) error = %v, want nil", err)
	}

	if err := sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("sleep(success) error = %v, want nil", err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("boom"))
	if err := sleep(ctx, time.Second); err == nil {
		t.Fatal("sleep(canceled) error = nil, want non-nil")
	}
}
