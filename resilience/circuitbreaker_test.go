package resilience

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc"
)

type eofClientStream struct {
	grpc.ClientStream
}

func (s *eofClientStream) RecvMsg(interface{}) error { return io.EOF }

type responseClientStream struct {
	grpc.ClientStream
}

func (s *responseClientStream) RecvMsg(interface{}) error { return nil }

func TestStreamCircuitBreakerRecordsSuccessWhenStreamCompletes(t *testing.T) {
	manager := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		OpenDuration:     5 * time.Millisecond,
		HalfOpenMaxReqs:  1,
	})
	method := "/example.Stream/Events"
	cb := manager.Get(method)
	openCircuit(t, cb)

	interceptor := StreamClientCircuitBreaker(manager)
	stream, err := interceptor(context.Background(), &grpc.StreamDesc{ServerStreams: true}, nil, method, func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return &eofClientStream{}, nil
	})
	if err != nil {
		t.Fatalf("StreamClientCircuitBreaker failed: %v", err)
	}
	if cb.State() != StateHalfOpen || cb.Stats().HalfOpenReqs != 1 {
		t.Fatalf("stream creation must not record success, got %#v", cb.Stats())
	}
	if err := stream.RecvMsg(nil); !errors.Is(err, io.EOF) {
		t.Fatalf("expected stream completion, got %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected completed stream to close circuit, got %s", cb.State())
	}
}

func TestStreamCircuitBreakerRecordsClientStreamResponseSuccess(t *testing.T) {
	manager := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		OpenDuration:     5 * time.Millisecond,
		HalfOpenMaxReqs:  1,
	})
	method := "/example.Stream/Upload"
	cb := manager.Get(method)
	openCircuit(t, cb)

	interceptor := StreamClientCircuitBreaker(manager)
	stream, err := interceptor(context.Background(), &grpc.StreamDesc{ClientStreams: true}, nil, method, func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return &responseClientStream{}, nil
	})
	if err != nil {
		t.Fatalf("StreamClientCircuitBreaker failed: %v", err)
	}
	if err := stream.RecvMsg(nil); err != nil {
		t.Fatalf("expected terminal client-stream response, got %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected successful client-stream response to close circuit, got %s", cb.State())
	}
}

func TestStreamCircuitBreakerRespectsFailureClassifier(t *testing.T) {
	manager := NewCircuitBreakerManager(CircuitConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		OpenDuration:     5 * time.Millisecond,
		HalfOpenMaxReqs:  1,
		IsFailure: func(err error) bool {
			return !errors.Is(err, context.Canceled)
		},
	})
	method := "/example.Stream/Events"
	cb := manager.Get(method)
	openCircuit(t, cb)

	interceptor := StreamClientCircuitBreaker(manager)
	_, err := interceptor(context.Background(), &grpc.StreamDesc{ServerStreams: true}, nil, method, func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected original stream error, got %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("non-failure stream error should close half-open circuit, got %s", cb.State())
	}
}

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
