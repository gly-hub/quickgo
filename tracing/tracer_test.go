package tracing

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
)

func TestDisableSamplingDropsAll(t *testing.T) {
	if err := Init(&Config{Enabled: true, DisableSampling: true}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Shutdown(context.Background())

	_, span := StartSpan(context.Background(), "drop")
	defer span.End()
	if span.IsRecording() {
		t.Fatal("expected disableSampling to create non-recording spans")
	}
}

func TestDefaultConfigSamplesAll(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	if err := Init(&config); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Shutdown(context.Background())

	_, span := StartSpan(context.Background(), "record")
	defer span.End()
	if !span.IsRecording() {
		t.Fatal("expected DefaultConfig to create recording spans")
	}
}

func TestParentBasedSamplingHonorsRemoteParentDecision(t *testing.T) {
	if err := Init(&Config{Enabled: true, SamplingRate: 0.01}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Shutdown(context.Background())

	newRemoteContext := func(flags trace.TraceFlags) context.Context {
		spanContext := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    trace.TraceID{1},
			SpanID:     trace.SpanID{2},
			TraceFlags: flags,
			Remote:     true,
		})
		return trace.ContextWithRemoteSpanContext(context.Background(), spanContext)
	}

	_, sampled := StartSpan(newRemoteContext(trace.FlagsSampled), "sampled-parent")
	defer sampled.End()
	if !sampled.IsRecording() {
		t.Fatal("expected sampled remote parent to remain sampled")
	}

	_, unsampled := StartSpan(newRemoteContext(0), "unsampled-parent")
	defer unsampled.End()
	if unsampled.IsRecording() {
		t.Fatal("expected unsampled remote parent to remain unsampled")
	}
}

func TestParseOTLPEndpointPreservesURLDetails(t *testing.T) {
	endpoint, err := parseOTLPEndpoint("http://collector.example/v1/traces", "4318")
	if err != nil {
		t.Fatalf("parseOTLPEndpoint failed: %v", err)
	}
	if !endpoint.isURL || endpoint.scheme != "http" || endpoint.value != "http://collector.example:4318/v1/traces" {
		t.Fatalf("unexpected endpoint: %#v", endpoint)
	}

	if _, err := parseOTLPEndpoint("ftp://collector.example", "4318"); err == nil {
		t.Fatal("expected unsupported scheme to fail")
	}
}

func TestInitRejectsConflictingOTLPURLSecurity(t *testing.T) {
	err := Init(&Config{
		Enabled: true,
		OTLP: OTLPConfig{
			Enabled:  true,
			Endpoint: "https://collector.example:4318/v1/traces",
			Insecure: true,
		},
	})
	if err == nil {
		t.Fatal("expected conflicting endpoint scheme and insecure option to fail")
	}
}

type recordingSpan struct {
	trace.Span
	ended atomic.Int32
}

func (s *recordingSpan) End(options ...trace.SpanEndOption) {
	s.ended.Add(1)
	s.Span.End(options...)
}

type eofTraceClientStream struct {
	grpc.ClientStream
}

func (s *eofTraceClientStream) CloseSend() error          { return nil }
func (s *eofTraceClientStream) RecvMsg(interface{}) error { return io.EOF }

func TestTracedServerStreamEndsAfterReceiveCompletion(t *testing.T) {
	_, noopSpan := noop.NewTracerProvider().Tracer("test").Start(context.Background(), "stream")
	span := &recordingSpan{Span: noopSpan}
	stream := &tracedClientStream{
		ClientStream:  &eofTraceClientStream{},
		span:          span,
		serverStreams: true,
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend failed: %v", err)
	}
	if got := span.ended.Load(); got != 0 {
		t.Fatalf("CloseSend ended server-stream span early: %d", got)
	}
	if err := stream.RecvMsg(nil); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
	if got := span.ended.Load(); got != 1 {
		t.Fatalf("expected span to end on stream completion once, got %d", got)
	}
}

func TestInitAndShutdownAreSafeToCallConcurrently(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true

	var wg sync.WaitGroup
	errCh := make(chan error, 16)
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := Init(&config); err != nil {
				errCh <- err
			}
		}()
		go func() {
			defer wg.Done()
			if err := Shutdown(context.Background()); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent lifecycle operation failed: %v", err)
	}
	if err := Shutdown(context.Background()); err != nil {
		t.Fatalf("final Shutdown failed: %v", err)
	}
	if IsEnabled() {
		t.Fatal("expected tracing to be disabled after final Shutdown")
	}
}
