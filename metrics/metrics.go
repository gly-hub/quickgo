package metrics

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/team-dandelion/quickgo/logger"
)

var (
	// 全局指标实例
	globalMetrics *Metrics
	globalMu      sync.RWMutex
)

// Metrics 指标收集器
type Metrics struct {
	registry *prometheus.Registry

	// HTTP 指标
	HTTPRequestTotal    *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestInFlight prometheus.Gauge

	// gRPC 指标
	GRPCRequestTotal    *prometheus.CounterVec
	GRPCRequestDuration *prometheus.HistogramVec
	GRPCStreamTotal     *prometheus.CounterVec

	// 连接池指标
	PoolConnections *prometheus.GaugeVec
	PoolHealthy     *prometheus.GaugeVec
	PoolUnhealthy   *prometheus.GaugeVec
	PoolReconnects  *prometheus.CounterVec

	// 限流熔断指标
	RateLimitRejected   *prometheus.CounterVec
	CircuitBreakerState *prometheus.GaugeVec
	CircuitBreakerTrips *prometheus.CounterVec

	// 自定义指标
	customCounters   map[string]*prometheus.CounterVec
	customGauges     map[string]*prometheus.GaugeVec
	customHistograms map[string]*prometheus.HistogramVec
	mu               sync.RWMutex
}

// Config 指标配置
type Config struct {
	Namespace         string    // 命名空间
	Subsystem         string    // 子系统
	Buckets           []float64 // 直方图桶
	EnableHTTP        bool      // 启用 HTTP 指标
	EnableGRPC        bool      // 启用 gRPC 指标
	EnablePool        bool      // 启用连接池指标
	EnableResilience  bool      // 启用限流熔断指标
	DisableHTTP       bool      // 显式禁用 HTTP 指标
	DisableGRPC       bool      // 显式禁用 gRPC 指标
	DisablePool       bool      // 显式禁用连接池指标
	DisableResilience bool      // 显式禁用限流熔断指标
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		Namespace:        "quickgo",
		Subsystem:        "",
		Buckets:          prometheus.DefBuckets,
		EnableHTTP:       true,
		EnableGRPC:       true,
		EnablePool:       true,
		EnableResilience: true,
	}
}

// Init 初始化全局指标
func Init(config Config) *Metrics {
	m := New(config)
	globalMu.Lock()
	globalMetrics = m
	globalMu.Unlock()
	return m
}

// Global 获取全局指标实例
func Global() *Metrics {
	globalMu.RLock()
	current := globalMetrics
	globalMu.RUnlock()
	if current != nil {
		return current
	}

	globalMu.Lock()
	defer globalMu.Unlock()
	if globalMetrics == nil {
		globalMetrics = New(DefaultConfig())
	}
	return globalMetrics
}

// New 创建新的指标实例
func New(config Config) *Metrics {
	config = normalizeConfig(config)
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	m := &Metrics{
		registry:         registry,
		customCounters:   make(map[string]*prometheus.CounterVec),
		customGauges:     make(map[string]*prometheus.GaugeVec),
		customHistograms: make(map[string]*prometheus.HistogramVec),
	}

	if config.EnableHTTP {
		m.initHTTPMetrics(config)
	}

	if config.EnableGRPC {
		m.initGRPCMetrics(config)
	}

	if config.EnablePool {
		m.initPoolMetrics(config)
	}

	if config.EnableResilience {
		m.initResilienceMetrics(config)
	}

	return m
}

func normalizeConfig(config Config) Config {
	defaults := DefaultConfig()
	if config.Namespace == "" {
		config.Namespace = defaults.Namespace
	}
	if len(config.Buckets) == 0 {
		config.Buckets = defaults.Buckets
	}
	hasExplicitEnable := config.EnableHTTP || config.EnableGRPC || config.EnablePool || config.EnableResilience
	if !hasExplicitEnable {
		config.EnableHTTP = defaults.EnableHTTP
		config.EnableGRPC = defaults.EnableGRPC
		config.EnablePool = defaults.EnablePool
		config.EnableResilience = defaults.EnableResilience
	} else if !config.DisableResilience {
		// Resilience metrics existed before the per-collector toggles and remain on
		// by default unless explicitly disabled.
		config.EnableResilience = true
	}
	if config.DisableHTTP {
		config.EnableHTTP = false
	}
	if config.DisableGRPC {
		config.EnableGRPC = false
	}
	if config.DisablePool {
		config.EnablePool = false
	}
	if config.DisableResilience {
		config.EnableResilience = false
	}
	return config
}

func (m *Metrics) initHTTPMetrics(config Config) {
	m.HTTPRequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	m.HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   config.Buckets,
		},
		[]string{"method", "path"},
	)

	m.HTTPRequestInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "http_requests_in_flight",
			Help:      "Current number of HTTP requests being processed",
		},
	)

	m.registry.MustRegister(m.HTTPRequestTotal)
	m.registry.MustRegister(m.HTTPRequestDuration)
	m.registry.MustRegister(m.HTTPRequestInFlight)
}

func (m *Metrics) initGRPCMetrics(config Config) {
	m.GRPCRequestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "grpc_requests_total",
			Help:      "Total number of gRPC requests",
		},
		[]string{"method", "code"},
	)

	m.GRPCRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "grpc_request_duration_seconds",
			Help:      "gRPC request duration in seconds",
			Buckets:   config.Buckets,
		},
		[]string{"method"},
	)

	m.GRPCStreamTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "grpc_streams_total",
			Help:      "Total number of gRPC streams",
		},
		[]string{"method", "type"},
	)

	m.registry.MustRegister(m.GRPCRequestTotal)
	m.registry.MustRegister(m.GRPCRequestDuration)
	m.registry.MustRegister(m.GRPCStreamTotal)
}

func (m *Metrics) initPoolMetrics(config Config) {
	m.PoolConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "pool_connections",
			Help:      "Number of connections in pool",
		},
		[]string{"service"},
	)

	m.PoolHealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "pool_connections_healthy",
			Help:      "Number of healthy connections in pool",
		},
		[]string{"service"},
	)

	m.PoolUnhealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "pool_connections_unhealthy",
			Help:      "Number of unhealthy connections in pool",
		},
		[]string{"service"},
	)

	m.PoolReconnects = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "pool_reconnects_total",
			Help:      "Total number of pool reconnects",
		},
		[]string{"service"},
	)

	m.registry.MustRegister(m.PoolConnections)
	m.registry.MustRegister(m.PoolHealthy)
	m.registry.MustRegister(m.PoolUnhealthy)
	m.registry.MustRegister(m.PoolReconnects)
}

func (m *Metrics) initResilienceMetrics(config Config) {
	m.RateLimitRejected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "ratelimit_rejected_total",
			Help:      "Total number of rejected requests due to rate limiting",
		},
		[]string{"limiter"},
	)

	m.CircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "circuitbreaker_state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		},
		[]string{"name"},
	)

	m.CircuitBreakerTrips = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.Namespace,
			Subsystem: config.Subsystem,
			Name:      "circuitbreaker_trips_total",
			Help:      "Total number of circuit breaker trips",
		},
		[]string{"name"},
	)

	m.registry.MustRegister(m.RateLimitRejected)
	m.registry.MustRegister(m.CircuitBreakerState)
	m.registry.MustRegister(m.CircuitBreakerTrips)
}

// Registry 获取 prometheus registry
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// Handler 返回 prometheus HTTP handler
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// RecordHTTPRequest 记录 HTTP 请求
func (m *Metrics) RecordHTTPRequest(method, path, status string, duration time.Duration) {
	if m.HTTPRequestTotal != nil {
		m.HTTPRequestTotal.WithLabelValues(method, path, status).Inc()
	}
	if m.HTTPRequestDuration != nil {
		m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
	}
}

// RecordGRPCRequest 记录 gRPC 请求
func (m *Metrics) RecordGRPCRequest(method, code string, duration time.Duration) {
	if m.GRPCRequestTotal != nil {
		m.GRPCRequestTotal.WithLabelValues(method, code).Inc()
	}
	if m.GRPCRequestDuration != nil {
		m.GRPCRequestDuration.WithLabelValues(method).Observe(duration.Seconds())
	}
}

// RecordPoolStatus 记录连接池状态
func (m *Metrics) RecordPoolStatus(service string, total, healthy, unhealthy int) {
	if m.PoolConnections != nil {
		m.PoolConnections.WithLabelValues(service).Set(float64(total))
	}
	if m.PoolHealthy != nil {
		m.PoolHealthy.WithLabelValues(service).Set(float64(healthy))
	}
	if m.PoolUnhealthy != nil {
		m.PoolUnhealthy.WithLabelValues(service).Set(float64(unhealthy))
	}
}

// RecordPoolReconnect 记录连接池重连
func (m *Metrics) RecordPoolReconnect(service string) {
	if m.PoolReconnects != nil {
		m.PoolReconnects.WithLabelValues(service).Inc()
	}
}

// RecordRateLimitRejected 记录限流拒绝
func (m *Metrics) RecordRateLimitRejected(limiter string) {
	if m.RateLimitRejected != nil {
		m.RateLimitRejected.WithLabelValues(limiter).Inc()
	}
}

// RecordCircuitBreakerState 记录熔断器状态
func (m *Metrics) RecordCircuitBreakerState(name string, state int) {
	if m.CircuitBreakerState != nil {
		m.CircuitBreakerState.WithLabelValues(name).Set(float64(state))
	}
}

// RecordCircuitBreakerTrip 记录熔断器触发
func (m *Metrics) RecordCircuitBreakerTrip(name string) {
	if m.CircuitBreakerTrips != nil {
		m.CircuitBreakerTrips.WithLabelValues(name).Inc()
	}
}

// Counter 获取或创建自定义计数器
func (m *Metrics) Counter(name string, labels []string) *prometheus.CounterVec {
	m.mu.RLock()
	if c, ok := m.customCounters[name]; ok {
		m.mu.RUnlock()
		return c
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.customCounters[name]; ok {
		return c
	}

	c := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: name,
			Help: "Custom counter: " + name,
		},
		labels,
	)

	if err := m.registry.Register(c); err != nil {
		logger.Error(context.Background(), "Failed to register counter %s: %v", name, err)
		return nil
	}

	m.customCounters[name] = c
	return c
}

// Gauge 获取或创建自定义仪表
func (m *Metrics) Gauge(name string, labels []string) *prometheus.GaugeVec {
	m.mu.RLock()
	if g, ok := m.customGauges[name]; ok {
		m.mu.RUnlock()
		return g
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if g, ok := m.customGauges[name]; ok {
		return g
	}

	g := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: name,
			Help: "Custom gauge: " + name,
		},
		labels,
	)

	if err := m.registry.Register(g); err != nil {
		logger.Error(context.Background(), "Failed to register gauge %s: %v", name, err)
		return nil
	}

	m.customGauges[name] = g
	return g
}

// Histogram 获取或创建自定义直方图
func (m *Metrics) Histogram(name string, labels []string, buckets []float64) *prometheus.HistogramVec {
	m.mu.RLock()
	if h, ok := m.customHistograms[name]; ok {
		m.mu.RUnlock()
		return h
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if h, ok := m.customHistograms[name]; ok {
		return h
	}

	if buckets == nil {
		buckets = prometheus.DefBuckets
	}

	h := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    name,
			Help:    "Custom histogram: " + name,
			Buckets: buckets,
		},
		labels,
	)

	if err := m.registry.Register(h); err != nil {
		logger.Error(context.Background(), "Failed to register histogram %s: %v", name, err)
		return nil
	}

	m.customHistograms[name] = h
	return h
}
