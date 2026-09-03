package resilience

import (
	"context"
	"testing"
)

func BenchmarkTokenBucketAllow(b *testing.B) {
	limiter := NewTokenBucketLimiter(TokenBucketConfig{MaxTokens: 1 << 30, RefillRate: 1e12})
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if !limiter.Allow() {
				b.Fatal("token bucket unexpectedly rejected request")
			}
		}
	})
}

func BenchmarkSlidingWindowRejected(b *testing.B) {
	limiter := NewSlidingWindowLimiter(SlidingWindowConfig{MaxReqs: 1})
	if !limiter.Allow() {
		b.Fatal("failed to seed sliding window")
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = limiter.Allow()
		}
	})
}

func BenchmarkCircuitBreakerExecute(b *testing.B) {
	cb := NewCircuitBreaker("benchmark", DefaultCircuitConfig())
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := cb.Execute(ctx, func(context.Context) error { return nil }); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCircuitBreakerManagerGet(b *testing.B) {
	manager := NewCircuitBreakerManager(DefaultCircuitConfig())
	_ = manager.Get("benchmark")
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = manager.Get("benchmark")
		}
	})
}

func BenchmarkBackoff(b *testing.B) {
	config := BackoffConfig{Policy: BackoffExponential, InitialDelay: 10, MaxDelay: 100000, Multiplier: 2}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Backoff(config, i%8+1)
	}
}
