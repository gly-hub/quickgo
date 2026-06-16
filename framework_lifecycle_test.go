package quickgo

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/team-dandelion/quickgo/metrics"
)

func TestFrameworkLoggerFileOutput(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "quickgo_framework_logger_*.log")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	f, err := NewFramework(ConfigOptionWithLogger(LoggerConfig{
		Enabled: true,
		Level:   "info",
		Output:  "file",
		File:    tmpFile.Name(),
		Service: "test-service",
		Version: "1.0.0",
	}))
	if err != nil {
		t.Fatalf("NewFramework failed: %v", err)
	}
	if err := f.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer f.Logger().Close()

	f.Logger().Info(context.Background(), "framework file log")

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(content), "framework file log") {
		t.Fatalf("expected file log content, got %s", string(content))
	}
}

type lifecycleTestComponent struct {
	name       string
	enabled    bool
	initErr    error
	startErr   error
	stopErr    error
	events     *[]string
	eventsLock *sync.Mutex
}

func (c *lifecycleTestComponent) Name() string { return c.name }

func (c *lifecycleTestComponent) IsEnabled() bool { return c.enabled }

func (c *lifecycleTestComponent) Init(ctx context.Context) error {
	c.record("init:" + c.name)
	return c.initErr
}

func (c *lifecycleTestComponent) Start(ctx context.Context) error {
	c.record("start:" + c.name)
	return c.startErr
}

func (c *lifecycleTestComponent) Stop(ctx context.Context) error {
	c.record("stop:" + c.name)
	return c.stopErr
}

func (c *lifecycleTestComponent) record(event string) {
	if c.eventsLock != nil {
		c.eventsLock.Lock()
		defer c.eventsLock.Unlock()
	}
	*c.events = append(*c.events, event)
}

func TestFrameworkStopAfterInitCleansInitializedComponents(t *testing.T) {
	var (
		events []string
		mu     sync.Mutex
	)

	f, err := NewFramework(ConfigOptionWithLogger(LoggerConfig{Enabled: false}))
	if err != nil {
		t.Fatalf("NewFramework failed: %v", err)
	}
	for _, name := range []string{"first", "second"} {
		if err := f.RegisterComponent(&lifecycleTestComponent{name: name, enabled: true, events: &events, eventsLock: &mu}); err != nil {
			t.Fatalf("RegisterComponent(%s) failed: %v", name, err)
		}
	}

	if err := f.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if err := f.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	want := []string{"init:first", "init:second", "stop:second", "stop:first"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected lifecycle order: got %v want %v", events, want)
	}
}

func TestFrameworkInitFailureRollsBackInitializedComponents(t *testing.T) {
	var (
		events []string
		mu     sync.Mutex
	)

	f, err := NewFramework(ConfigOptionWithLogger(LoggerConfig{Enabled: false}))
	if err != nil {
		t.Fatalf("NewFramework failed: %v", err)
	}
	if err := f.RegisterComponent(&lifecycleTestComponent{name: "first", enabled: true, events: &events, eventsLock: &mu}); err != nil {
		t.Fatalf("RegisterComponent(first) failed: %v", err)
	}
	if err := f.RegisterComponent(&lifecycleTestComponent{name: "second", enabled: true, initErr: errors.New("boom"), events: &events, eventsLock: &mu}); err != nil {
		t.Fatalf("RegisterComponent(second) failed: %v", err)
	}

	if err := f.Init(); err == nil {
		t.Fatal("expected Init to fail")
	}

	want := []string{"init:first", "init:second", "stop:first"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected rollback order: got %v want %v", events, want)
	}
	if err := f.Stop(); err != nil {
		t.Fatalf("Stop after failed Init should be idempotent: %v", err)
	}
}

func TestFrameworkComponentStartStopOrderIsStable(t *testing.T) {
	var (
		events []string
		mu     sync.Mutex
	)

	f, err := NewFramework(ConfigOptionWithLogger(LoggerConfig{Enabled: false}))
	if err != nil {
		t.Fatalf("NewFramework failed: %v", err)
	}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := f.RegisterComponent(&lifecycleTestComponent{name: name, enabled: true, events: &events, eventsLock: &mu}); err != nil {
			t.Fatalf("RegisterComponent(%s) failed: %v", name, err)
		}
	}

	if err := f.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if err := f.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := f.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	want := []string{
		"init:alpha", "init:beta", "init:gamma",
		"start:alpha", "start:beta", "start:gamma",
		"stop:gamma", "stop:beta", "stop:alpha",
	}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected stable order: got %v want %v", events, want)
	}
}

func TestFrameworkMetricsPropagateToServersWithoutMutatingInput(t *testing.T) {
	metricsConfig := &metrics.Config{Namespace: "suite", Buckets: []float64{0.1, 0.2}}
	httpConfig := &HTTPServerConfig{Enabled: true}
	grpcConfig := &GrpcServerConfig{}

	f, err := NewFramework(
		ConfigOptionWithLogger(LoggerConfig{Enabled: false}),
		ConfigOptionWithMetrics(metricsConfig),
		ConfigOptionWithHTTPServer(httpConfig),
		ConfigOptionWithGrpcServer(grpcConfig),
	)
	if err != nil {
		t.Fatalf("NewFramework failed: %v", err)
	}
	if err := f.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer f.Stop()

	if f.config.HTTPServer.Metrics == nil {
		t.Fatal("expected framework metrics to propagate to HTTP server")
	}
	if f.config.GrpcServer.Metrics == nil {
		t.Fatal("expected framework metrics to propagate to gRPC server")
	}
	if httpConfig.Metrics != nil || grpcConfig.Metrics != nil {
		t.Fatal("expected original server configs to remain unmodified")
	}

	metricsConfig.Buckets[0] = 9
	if f.config.HTTPServer.Metrics.Buckets[0] == 9 || f.config.GrpcServer.Metrics.Buckets[0] == 9 {
		t.Fatal("expected propagated metrics buckets to be cloned")
	}
	if f.HTTPServer().Metrics() == nil || f.GrpcServer().Metrics() == nil {
		t.Fatal("expected servers to use metrics collectors")
	}
	if f.HTTPServer().Metrics() != f.GrpcServer().Metrics() || f.Metrics() != f.HTTPServer().Metrics() {
		t.Fatal("expected framework HTTP and gRPC servers to share one metrics collector")
	}
}

func TestFrameworkHTTPMetricsEndpointExposesSharedGRPCMetrics(t *testing.T) {
	f, err := NewFramework(
		ConfigOptionWithLogger(LoggerConfig{Enabled: false}),
		ConfigOptionWithMetrics(&metrics.Config{Namespace: "suite"}),
		ConfigOptionWithHTTPServer(&HTTPServerConfig{Enabled: true}),
		ConfigOptionWithGrpcServer(&GrpcServerConfig{}),
	)
	if err != nil {
		t.Fatalf("NewFramework failed: %v", err)
	}
	if err := f.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer f.Stop()

	f.GrpcServer().Metrics().RecordGRPCRequest("/svc/Method", "OK", 0)
	resp, err := f.HTTPServer().GetApp().Test(httptest.NewRequest("GET", "/metrics", nil))
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body failed: %v", err)
	}
	if !strings.Contains(string(body), `suite_grpc_requests_total{code="OK",method="/svc/Method"} 1`) {
		t.Fatalf("expected HTTP metrics endpoint to expose gRPC metrics, got %s", string(body))
	}
}

type frameworkAccessComponent struct {
	name      string
	enabled   bool
	framework *Framework
}

func (c *frameworkAccessComponent) Name() string { return c.name }

func (c *frameworkAccessComponent) IsEnabled() bool { return c.enabled }

func (c *frameworkAccessComponent) Init(ctx context.Context) error {
	_, err := c.framework.GetComponent(c.name)
	return err
}

func (c *frameworkAccessComponent) Start(ctx context.Context) error {
	_, err := c.framework.GetComponent(c.name)
	return err
}

func (c *frameworkAccessComponent) Stop(ctx context.Context) error {
	_, err := c.framework.GetComponent(c.name)
	return err
}

func TestFrameworkLifecycleAllowsComponentCallbacksToAccessFramework(t *testing.T) {
	f, err := NewFramework(ConfigOptionWithLogger(LoggerConfig{Enabled: false}))
	if err != nil {
		t.Fatalf("NewFramework failed: %v", err)
	}
	component := &frameworkAccessComponent{name: "self", enabled: true, framework: f}
	if err := f.RegisterComponent(component); err != nil {
		t.Fatalf("RegisterComponent failed: %v", err)
	}
	if err := f.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if err := f.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := f.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}
