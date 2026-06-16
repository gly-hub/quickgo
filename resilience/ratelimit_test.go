package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTokenBucketRejectsInvalidTokenCounts(t *testing.T) {
	limiter := NewTokenBucketLimiter(TokenBucketConfig{MaxTokens: 2, RefillRate: 1})

	if limiter.AllowN(0) {
		t.Fatal("AllowN(0) should be rejected")
	}
	if limiter.AllowN(-1) {
		t.Fatal("AllowN(-1) should be rejected")
	}
	if limiter.AllowN(3) {
		t.Fatal("AllowN above capacity should be rejected")
	}
	if !limiter.AllowN(2) {
		t.Fatal("AllowN at capacity should be allowed")
	}

	if err := limiter.WaitN(context.Background(), 0); !errors.Is(err, ErrInvalidTokenCount) {
		t.Fatalf("WaitN(0) should return ErrInvalidTokenCount, got %v", err)
	}
	if err := limiter.WaitN(context.Background(), 3); !errors.Is(err, ErrInvalidTokenCount) {
		t.Fatalf("WaitN above capacity should return ErrInvalidTokenCount, got %v", err)
	}
}

func TestSlidingWindowRejectsInvalidTokenCounts(t *testing.T) {
	limiter := NewSlidingWindowLimiter(SlidingWindowConfig{WindowSize: time.Second, MaxReqs: 2})

	if limiter.AllowN(0) {
		t.Fatal("AllowN(0) should be rejected")
	}
	if limiter.AllowN(-1) {
		t.Fatal("AllowN(-1) should be rejected")
	}
	if limiter.AllowN(3) {
		t.Fatal("AllowN above capacity should be rejected")
	}
	if !limiter.AllowN(2) {
		t.Fatal("AllowN at capacity should be allowed")
	}

	if err := limiter.WaitN(context.Background(), 0); !errors.Is(err, ErrInvalidTokenCount) {
		t.Fatalf("WaitN(0) should return ErrInvalidTokenCount, got %v", err)
	}
	if err := limiter.WaitN(context.Background(), 3); !errors.Is(err, ErrInvalidTokenCount) {
		t.Fatalf("WaitN above capacity should return ErrInvalidTokenCount, got %v", err)
	}
}

func TestRateLimitMiddlewareNilLimiterRejects(t *testing.T) {
	var middleware *RateLimitMiddleware
	if err := middleware.Check(context.Background()); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("nil middleware should return ErrRateLimited, got %v", err)
	}

	middleware = NewRateLimitMiddleware(nil, false)
	if err := middleware.Check(context.Background()); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("nil limiter should return ErrRateLimited, got %v", err)
	}
}
