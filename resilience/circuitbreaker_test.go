package resilience

import (
	"errors"
	"testing"
	"time"
)

func openCircuit(t *testing.T, cb *CircuitBreaker) {
	t.Helper()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("expected circuit to be open, got %s", cb.State())
	}
	time.Sleep(cb.config.OpenDuration + time.Millisecond)
}

func TestCircuitBreakerHalfOpenMaxRequests(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		OpenDuration:     5 * time.Millisecond,
		HalfOpenMaxReqs:  2,
	})
	openCircuit(t, cb)

	if err := cb.Allow(); err != nil {
		t.Fatalf("first half-open probe should be allowed: %v", err)
	}
	if err := cb.Allow(); err != nil {
		t.Fatalf("second half-open probe should be allowed: %v", err)
	}
	if err := cb.Allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("third half-open probe should be rejected, got %v", err)
	}
	if stats := cb.Stats(); stats.HalfOpenReqs != 2 {
		t.Fatalf("expected two half-open probes in flight, got %d", stats.HalfOpenReqs)
	}
}

func TestCircuitBreakerHalfOpenSuccessClosesCircuit(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		OpenDuration:     5 * time.Millisecond,
		HalfOpenMaxReqs:  1,
	})
	openCircuit(t, cb)

	if err := cb.Allow(); err != nil {
		t.Fatalf("first half-open probe should be allowed: %v", err)
	}
	cb.RecordSuccess()
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected circuit to remain half-open before success threshold, got %s", cb.State())
	}
	if stats := cb.Stats(); stats.HalfOpenReqs != 0 {
		t.Fatalf("expected half-open request count to be released, got %d", stats.HalfOpenReqs)
	}

	if err := cb.Allow(); err != nil {
		t.Fatalf("second half-open probe should be allowed: %v", err)
	}
	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Fatalf("expected circuit to close after success threshold, got %s", cb.State())
	}
}

func TestCircuitBreakerHalfOpenFailureReopensCircuit(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		OpenDuration:     5 * time.Millisecond,
		HalfOpenMaxReqs:  1,
	})
	openCircuit(t, cb)

	if err := cb.Allow(); err != nil {
		t.Fatalf("half-open probe should be allowed: %v", err)
	}
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("expected failure in half-open state to reopen circuit, got %s", cb.State())
	}
	if stats := cb.Stats(); stats.HalfOpenReqs != 0 {
		t.Fatalf("expected half-open request count to reset, got %d", stats.HalfOpenReqs)
	}
}
