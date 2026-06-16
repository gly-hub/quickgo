package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHealthCheckerCheckHandlesHealthyTimeoutAndPanic(t *testing.T) {
	checker := NewHealthChecker(HealthCheckerConfig{Timeout: 20 * time.Millisecond})
	checker.RegisterFunc("ok", func(ctx context.Context) HealthResult {
		return HealthResult{Status: StatusHealthy, Message: "ok"}
	})
	checker.RegisterFunc("slow", func(ctx context.Context) HealthResult {
		time.Sleep(200 * time.Millisecond)
		return HealthResult{Status: StatusHealthy}
	})
	checker.RegisterFunc("panic", func(ctx context.Context) HealthResult {
		panic("boom")
	})

	start := time.Now()
	results := checker.Check(context.Background())
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("Check took too long: %v", elapsed)
	}

	if results["ok"].Status != StatusHealthy {
		t.Fatalf("expected ok check to be healthy, got %s", results["ok"].Status)
	}
	if results["slow"].Status != StatusUnhealthy {
		t.Fatalf("expected slow check to time out as unhealthy, got %s", results["slow"].Status)
	}
	if !strings.Contains(results["slow"].Message, "timed out") {
		t.Fatalf("expected timeout message, got %q", results["slow"].Message)
	}
	if results["panic"].Status != StatusUnhealthy {
		t.Fatalf("expected panic check to be unhealthy, got %s", results["panic"].Status)
	}
	if !strings.Contains(results["panic"].Message, "panic") {
		t.Fatalf("expected panic message, got %q", results["panic"].Message)
	}
}

func TestHealthCheckerCheckOneTimeoutUpdatesLastResult(t *testing.T) {
	checker := NewHealthChecker(HealthCheckerConfig{Timeout: 10 * time.Millisecond})
	checker.RegisterFunc("slow", func(ctx context.Context) HealthResult {
		time.Sleep(100 * time.Millisecond)
		return HealthResult{Status: StatusHealthy}
	})

	result, ok := checker.CheckOne(context.Background(), "slow")
	if !ok {
		t.Fatal("expected registered check to be found")
	}
	if result.Status != StatusUnhealthy {
		t.Fatalf("expected timeout to be unhealthy, got %s", result.Status)
	}
	if checker.LastResult()["slow"].Status != StatusUnhealthy {
		t.Fatal("expected last result to contain timeout result")
	}
}

func TestHealthCheckerOverallAndReadinessRespectTimeouts(t *testing.T) {
	checker := NewHealthChecker(HealthCheckerConfig{Timeout: 10 * time.Millisecond})
	checker.RegisterFunc("slow", func(ctx context.Context) HealthResult {
		time.Sleep(100 * time.Millisecond)
		return HealthResult{Status: StatusHealthy}
	})

	if got := checker.OverallStatus(context.Background()); got != StatusUnhealthy {
		t.Fatalf("expected overall status unhealthy, got %s", got)
	}
	if readiness := checker.IsReady(context.Background()); readiness.Ready {
		t.Fatal("expected readiness to be false when a check times out")
	}
}
