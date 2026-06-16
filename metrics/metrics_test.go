package metrics

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func metricValue(t *testing.T, metric prometheus.Metric) float64 {
	t.Helper()
	var pb dto.Metric
	if err := metric.Write(&pb); err != nil {
		t.Fatalf("metric.Write failed: %v", err)
	}
	if pb.Counter != nil {
		return pb.Counter.GetValue()
	}
	if pb.Gauge != nil {
		return pb.Gauge.GetValue()
	}
	t.Fatal("metric has no counter or gauge value")
	return 0
}

func TestNewZeroConfigEnablesDefaultCollectors(t *testing.T) {
	m := New(Config{})

	if m.HTTPRequestTotal == nil || m.HTTPRequestDuration == nil || m.HTTPRequestInFlight == nil {
		t.Fatal("expected HTTP collectors to be enabled by zero config")
	}
	if m.GRPCRequestTotal == nil || m.GRPCRequestDuration == nil || m.GRPCStreamTotal == nil {
		t.Fatal("expected gRPC collectors to be enabled by zero config")
	}
	if m.PoolConnections == nil || m.PoolReconnects == nil {
		t.Fatal("expected pool collectors to be enabled by zero config")
	}
}

func TestConfigCanDisableCollectorsExplicitly(t *testing.T) {
	m := New(Config{
		DisableHTTP:       true,
		DisableGRPC:       true,
		DisablePool:       true,
		DisableResilience: true,
	})

	if m.HTTPRequestTotal != nil || m.GRPCRequestTotal != nil || m.PoolConnections != nil || m.RateLimitRejected != nil {
		t.Fatal("expected all explicitly disabled collectors to be nil")
	}
}

func TestFiberMiddlewareRecordsHTTPRequest(t *testing.T) {
	m := New(Config{Namespace: "test"})
	app := fiber.New()
	app.Use(FiberMiddleware(m))
	app.Get("/users/:id", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusCreated)
	})

	req := httptest.NewRequest("GET", "/users/42", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("unexpected response status: %d", resp.StatusCode)
	}

	got := metricValue(t, m.HTTPRequestTotal.WithLabelValues("GET", "/users/:id", "201"))
	if got != 1 {
		t.Fatalf("expected one HTTP request metric, got %v", got)
	}
}

func TestUnaryServerInterceptorRecordsGRPCRequest(t *testing.T) {
	m := New(Config{Namespace: "test"})
	interceptor := UnaryServerInterceptor(m)

	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, status.Error(codes.NotFound, "missing")
	})
	if err == nil {
		t.Fatal("expected handler error")
	}

	got := metricValue(t, m.GRPCRequestTotal.WithLabelValues("/svc/Method", codes.NotFound.String()))
	if got != 1 {
		t.Fatalf("expected one gRPC request metric, got %v", got)
	}
}

func TestPoolMetricsRecordStatusAndReconnect(t *testing.T) {
	m := New(Config{Namespace: "test"})

	m.RecordPoolStatus("users", 5, 4, 1)
	m.RecordPoolReconnect("users")

	if got := metricValue(t, m.PoolConnections.WithLabelValues("users")); got != 5 {
		t.Fatalf("expected pool total 5, got %v", got)
	}
	if got := metricValue(t, m.PoolHealthy.WithLabelValues("users")); got != 4 {
		t.Fatalf("expected healthy total 4, got %v", got)
	}
	if got := metricValue(t, m.PoolUnhealthy.WithLabelValues("users")); got != 1 {
		t.Fatalf("expected unhealthy total 1, got %v", got)
	}
	if got := metricValue(t, m.PoolReconnects.WithLabelValues("users")); got != 1 {
		t.Fatalf("expected reconnect total 1, got %v", got)
	}
}

func TestMetricsHandlerExposesDefaultNamespace(t *testing.T) {
	m := New(Config{})
	m.RecordHTTPRequest("GET", "/ready", "200", 0)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "quickgo_http_requests_total") {
		t.Fatalf("expected default namespace metric in handler output")
	}
}

func TestInitCanReplaceGlobalMetrics(t *testing.T) {
	first := Init(Config{Namespace: "first"})
	second := Init(Config{Namespace: "second"})

	if first == second {
		t.Fatal("expected Init to replace global metrics instance")
	}
	if got := Global(); got != second {
		t.Fatal("expected Global to return latest Init instance")
	}
}
