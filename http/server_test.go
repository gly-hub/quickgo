package http

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gly-hub/quickgo/logger"
	"github.com/gofiber/fiber/v2"
)

func TestServerStopAfterListenClosesListener(t *testing.T) {
	port := reserveTCPPort(t)
	server, err := NewServer(Config{
		Address: "127.0.0.1",
		Port:    port,
		FiberConfig: fiber.Config{
			DisableStartupMessage: true,
		},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if err := server.Listen(); err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	addr := server.getListener().Addr().String()

	if err := server.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("expected listener address to be released after Stop, got %v", err)
	}
	_ = listener.Close()
}

func TestServerDoesNotSetDefaultReadWriteTimeouts(t *testing.T) {
	server, err := NewServer(Config{
		FiberConfig: fiber.Config{
			DisableStartupMessage: true,
		},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if got := server.GetApp().Server().ReadTimeout; got != 0 {
		t.Fatalf("expected default read timeout to remain unset, got %s", got)
	}
	if got := server.GetApp().Server().WriteTimeout; got != 0 {
		t.Fatalf("expected default write timeout to remain unset, got %s", got)
	}
}

func TestServerPreservesConfiguredReadWriteTimeouts(t *testing.T) {
	server, err := NewServer(Config{
		FiberConfig: fiber.Config{
			DisableStartupMessage: true,
			ReadTimeout:           3 * time.Second,
			WriteTimeout:          4 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if got := server.GetApp().Server().ReadTimeout; got != 3*time.Second {
		t.Fatalf("expected configured read timeout, got %s", got)
	}
	if got := server.GetApp().Server().WriteTimeout; got != 4*time.Second {
		t.Fatalf("expected configured write timeout, got %s", got)
	}
}

func TestServerDefaultMiddlewaresCanBeDisabledIndividually(t *testing.T) {
	server, err := NewServer(Config{
		DisableCORS:  true,
		DisableTrace: true,
		FiberConfig: fiber.Config{
			DisableStartupMessage: true,
		},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	server.GetApp().Get("/ok", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := server.GetApp().Test(httptest.NewRequest("GET", "/ok", nil))
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if got := resp.Header.Get(TraceIDHeader); got != "" {
		t.Fatalf("expected trace middleware to be disabled, got trace header %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected cors middleware to be disabled, got allow-origin %q", got)
	}
}

func TestServerZeroConfigEnablesDefaultMiddlewares(t *testing.T) {
	server, err := NewServer(Config{
		FiberConfig: fiber.Config{
			DisableStartupMessage: true,
		},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	server.GetApp().Get("/ok", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := server.GetApp().Test(httptest.NewRequest("GET", "/ok", nil))
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if got := resp.Header.Get(TraceIDHeader); got == "" {
		t.Fatal("expected trace middleware to set trace header")
	}
}

func TestTraceMiddlewareStoresCorrelationInUserContext(t *testing.T) {
	server, err := NewServer(Config{
		FiberConfig: fiber.Config{DisableStartupMessage: true},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	server.GetApp().Get("/trace", func(c *fiber.Ctx) error {
		if got := logger.GetTraceID(c.UserContext()); got != "request-trace" {
			t.Fatalf("trace ID = %q, want request-trace", got)
		}
		if logger.GetSpanID(c.UserContext()) == "" {
			t.Fatal("expected span ID in user context")
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest("GET", "/trace", nil)
	req.Header.Set(TraceIDHeader, "request-trace")
	if _, err := server.GetApp().Test(req); err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
}

func TestLoggingMiddlewareGeneratesCorrelationWhenTraceIsDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	if err := logger.Init(logger.Config{Level: logger.LevelInfo, Output: path}); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	defer logger.Close()

	server, err := NewServer(Config{
		DisableTrace: true,
		DisableCORS:  true,
		FiberConfig:  fiber.Config{DisableStartupMessage: true},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	server.GetApp().Get("/ok", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	if _, err := server.GetApp().Test(httptest.NewRequest("GET", "/ok", nil)); err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read access log: %v", err)
	}
	var entry logger.LogEntry
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("decode access log: %v; content=%s", err, content)
	}
	if entry.TraceID == "" || entry.SpanID == "" {
		t.Fatalf("expected generated correlation IDs, got trace=%q span=%q", entry.TraceID, entry.SpanID)
	}
}

func TestServerExplicitEnableDoesNotDisableOtherDefaultMiddlewares(t *testing.T) {
	server, err := NewServer(Config{
		EnableCORS: true,
		FiberConfig: fiber.Config{
			DisableStartupMessage: true,
		},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	server.GetApp().Get("/ok", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := server.GetApp().Test(httptest.NewRequest("GET", "/ok", nil))
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if got := resp.Header.Get(TraceIDHeader); got == "" {
		t.Fatal("expected trace middleware to remain enabled")
	}
}

func TestServerStartAsyncRejectsDuplicateStart(t *testing.T) {
	port := reserveTCPPort(t)
	server, err := NewServer(Config{
		Address: "127.0.0.1",
		Port:    port,
		FiberConfig: fiber.Config{
			DisableStartupMessage: true,
		},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer func() {
		if err := server.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	}()

	if err := server.StartAsync(); err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	if err := server.StartAsync(); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected duplicate StartAsync to be rejected, got %v", err)
	}
}

func TestServerStopIsIdempotent(t *testing.T) {
	port := reserveTCPPort(t)
	server, err := NewServer(Config{
		Address: "127.0.0.1",
		Port:    port,
		FiberConfig: fiber.Config{
			DisableStartupMessage: true,
		},
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if err := server.Stop(); err != nil {
		t.Fatalf("first Stop failed: %v", err)
	}
	if err := server.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve tcp port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}
