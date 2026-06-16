package http

import (
	"net"
	"net/http/httptest"
	"strings"
	"testing"

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
