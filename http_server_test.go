package quickgo

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/team-dandelion/quickgo/metrics"
)

func TestNewHTTPServerAppliesDefaultsWithoutMutatingInput(t *testing.T) {
	config := &HTTPServerConfig{}

	server, err := NewHTTPServer(config)
	if err != nil {
		t.Fatalf("NewHTTPServer should accept minimal config: %v", err)
	}
	if server == nil {
		t.Fatal("NewHTTPServer returned nil server")
	}

	if config.Address != "" {
		t.Fatalf("expected input address to remain empty, got %q", config.Address)
	}
	if config.Port != 0 {
		t.Fatalf("expected input port to remain 0, got %d", config.Port)
	}
	if server.config.Address != "0.0.0.0" {
		t.Fatalf("expected server default address %q, got %q", "0.0.0.0", server.config.Address)
	}
	if server.config.Port != 8080 {
		t.Fatalf("expected server default port %d, got %d", 8080, server.config.Port)
	}
}

func TestNewHTTPServerClonesMetricsConfig(t *testing.T) {
	config := &HTTPServerConfig{
		Metrics: &metrics.Config{Namespace: "http", Buckets: []float64{0.1, 0.2}},
	}

	server, err := NewHTTPServer(config)
	if err != nil {
		t.Fatalf("NewHTTPServer failed: %v", err)
	}
	if server.config.Metrics == config.Metrics {
		t.Fatal("expected metrics config to be cloned")
	}
	config.Metrics.Buckets[0] = 9
	if server.config.Metrics.Buckets[0] == 9 {
		t.Fatal("expected metrics buckets to be cloned")
	}
}

func TestNewHTTPServerExposesMetricsEndpoint(t *testing.T) {
	server, err := NewHTTPServer(&HTTPServerConfig{
		Metrics: &metrics.Config{Namespace: "httpserver"},
	})
	if err != nil {
		t.Fatalf("NewHTTPServer failed: %v", err)
	}

	server.Metrics().RecordHTTPRequest("GET", "/ok", "200", 0)
	resp, err := server.GetApp().Test(httptest.NewRequest("GET", "/metrics", nil))
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body failed: %v", err)
	}
	if !strings.Contains(string(body), "httpserver_http_requests_total") {
		t.Fatalf("expected metrics endpoint to expose shared collector, got %s", string(body))
	}
}
