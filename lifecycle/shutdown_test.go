package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestShutdownEnforcesHookTimeoutAndRetainsError(t *testing.T) {
	manager := NewShutdownManager(ShutdownConfig{GlobalTimeout: 20 * time.Millisecond})
	release := make(chan struct{})
	manager.RegisterFunc("blocked", PriorityFirst, func(context.Context) error {
		<-release
		return nil
	})

	started := time.Now()
	err := manager.Shutdown(context.Background())
	close(release)
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("shutdown hook timeout was not enforced")
	}

	secondErr := manager.Shutdown(context.Background())
	if secondErr == nil || secondErr.Error() != err.Error() {
		t.Fatalf("expected retained shutdown error, got %v", secondErr)
	}
}

func TestShutdownConvertsHookPanicToError(t *testing.T) {
	manager := NewShutdownManager(DefaultShutdownConfig())
	manager.RegisterFunc("panic", PriorityFirst, func(context.Context) error {
		panic("boom")
	})

	err := manager.Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "panic: boom") {
		t.Fatalf("expected panic error, got %v", err)
	}
}

func TestShutdownIgnoresHooksRegisteredAfterShutdownStarts(t *testing.T) {
	manager := NewShutdownManager(DefaultShutdownConfig())
	var ran []string
	manager.RegisterFunc("first", PriorityFirst, func(context.Context) error {
		ran = append(ran, "first")
		return nil
	})
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	manager.RegisterFunc("late", PriorityLast, func(context.Context) error {
		ran = append(ran, "late")
		return nil
	})
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown failed: %v", err)
	}
	if strings.Join(ran, ",") != "first" {
		t.Fatalf("late shutdown hook must not run, got %v", ran)
	}
}
