package tracing

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func BenchmarkStartSpanNoop(b *testing.B) {
	if err := Shutdown(context.Background()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := StartSpan(context.Background(), "benchmark")
		span.End()
	}
}

func BenchmarkStartSpanUnsampled(b *testing.B) {
	if err := Init(&Config{Enabled: true, DisableSampling: true}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = Shutdown(context.Background()) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := StartSpan(context.Background(), "benchmark")
		span.End()
	}
}

func BenchmarkInjectTraceContext(b *testing.B) {
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-request-id", "bench"))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = InjectTraceContext(ctx)
	}
}
