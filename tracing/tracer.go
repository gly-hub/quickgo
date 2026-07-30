package tracing

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var (
	// globalTracer 全局 tracer
	globalTracer trace.Tracer
	// tp 全局 TracerProvider
	tp *tracesdk.TracerProvider
	mu sync.RWMutex
	// lifecycleMu serializes global OpenTelemetry provider replacement.
	lifecycleMu sync.Mutex
)

// Init 初始化链路追踪
func Init(config *Config) error {
	if config == nil || !config.Enabled {
		return Shutdown(context.Background())
	}
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()

	// 设置服务名称
	serviceName := config.ServiceName
	if serviceName == "" {
		serviceName = "quickgo-service"
	}

	// 设置服务版本
	serviceVersion := config.ServiceVersion
	if serviceVersion == "" {
		serviceVersion = "1.0.0"
	}

	// 设置环境
	environment := config.Environment
	if environment == "" {
		environment = "development"
	}

	// 创建资源
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
			semconv.DeploymentEnvironmentKey.String(environment),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// 创建 Exporter（优先使用 OTLP，其次使用 Jaeger）
	var exporter tracesdk.SpanExporter

	if config.OTLP.Enabled && config.OTLP.Endpoint != "" {
		var err error
		// 使用 OTLP Exporter（推荐）
		// 解析 endpoint，提取 host:port
		defaultPort := "4318"
		if config.OTLP.UseGRPC {
			defaultPort = "4317"
		}
		endpoint := parseOTLPEndpoint(config.OTLP.Endpoint, defaultPort)

		if config.OTLP.UseGRPC {
			// 使用 gRPC
			opts := []otlptracegrpc.Option{
				otlptracegrpc.WithEndpoint(endpoint),
			}
			if config.OTLP.Insecure {
				opts = append(opts, otlptracegrpc.WithInsecure())
			}
			if len(config.OTLP.Headers) > 0 {
				opts = append(opts, otlptracegrpc.WithHeaders(config.OTLP.Headers))
			}
			exporter, err = otlptracegrpc.New(context.Background(), opts...)
		} else {
			// 使用 HTTP
			opts := []otlptracehttp.Option{
				otlptracehttp.WithEndpoint(endpoint),
			}
			if config.OTLP.Insecure {
				opts = append(opts, otlptracehttp.WithInsecure())
			}
			if len(config.OTLP.Headers) > 0 {
				opts = append(opts, otlptracehttp.WithHeaders(config.OTLP.Headers))
			}
			exporter, err = otlptracehttp.New(context.Background(), opts...)
		}
		if err != nil {
			return fmt.Errorf("failed to create OTLP exporter (endpoint=%s, parsed=%s): %w", config.OTLP.Endpoint, endpoint, err)
		}
	} else if config.Jaeger.Enabled {
		return fmt.Errorf("the Jaeger exporter is no longer supported; configure Jaeger through OTLP")
	} else {
		// 如果未启用任何 exporter，使用 Noop Exporter（仅本地追踪，不上传）
		// 注意：NewNoopExporter 不存在，我们使用 nil 并在后面检查
		exporter = nil
	}

	// 设置采样率
	samplingRate := config.SamplingRate
	if config.DisableSampling {
		samplingRate = 0
	} else if samplingRate == 0 {
		samplingRate = 1
	} else if samplingRate < 0 {
		return fmt.Errorf("sampling rate must be between 0 and 1")
	}
	if samplingRate > 1 {
		return fmt.Errorf("sampling rate must be between 0 and 1")
	}

	// 创建 TracerProvider
	var newProvider *tracesdk.TracerProvider
	if exporter == nil {
		// 如果没有 exporter，使用 Noop TracerProvider（仅本地追踪，不上传）
		newProvider = tracesdk.NewTracerProvider(
			tracesdk.WithResource(res),
			tracesdk.WithSampler(tracesdk.TraceIDRatioBased(samplingRate)),
		)
	} else {
		// 创建 TracerProvider（带 exporter，会上传到 Jaeger）
		newProvider = tracesdk.NewTracerProvider(
			tracesdk.WithBatcher(exporter),
			tracesdk.WithResource(res),
			tracesdk.WithSampler(tracesdk.TraceIDRatioBased(samplingRate)),
		)
	}

	// 设置全局 TracerProvider
	otel.SetTracerProvider(newProvider)

	// 设置全局 TextMapPropagator（用于跨服务传播 trace context）
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 创建全局 Tracer
	mu.Lock()
	oldProvider := tp
	tp = newProvider
	globalTracer = otel.Tracer(serviceName)
	mu.Unlock()
	if oldProvider != nil && oldProvider != newProvider {
		_ = oldProvider.Shutdown(context.Background())
	}

	return nil
}

// Shutdown 关闭链路追踪
func Shutdown(ctx context.Context) error {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()

	noopProvider := noop.NewTracerProvider()
	mu.Lock()
	current := tp
	tp = nil
	globalTracer = nil
	mu.Unlock()
	otel.SetTracerProvider(noopProvider)
	if current != nil {
		return current.Shutdown(ctx)
	}
	return nil
}

// GetTracer 获取 Tracer 实例
func GetTracer() trace.Tracer {
	mu.RLock()
	current := globalTracer
	mu.RUnlock()
	if current == nil {
		// 如果未初始化，返回 Noop Tracer
		return noop.NewTracerProvider().Tracer("noop")
	}
	return current
}

// StartSpan 开始一个新的 span
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return GetTracer().Start(ctx, name, opts...)
}

// SpanFromContext 从 context 中获取 span
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// GetTraceIDFromContext 从 context 中获取 trace ID（字符串格式）
func GetTraceIDFromContext(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.TraceID().String()
}

// AddTraceIDToSpan 将 trace_id 添加到 span 的 attributes 中
func AddTraceIDToSpan(span trace.Span, ctx context.Context) {
	if span == nil || !span.IsRecording() {
		return
	}
	traceID := GetTraceIDFromContext(ctx)
	if traceID != "" {
		span.SetAttributes(attribute.String("trace_id", traceID))
	}
}

// IsEnabled 检查 tracing 是否已启用
func IsEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return globalTracer != nil && tp != nil
}

// parseOTLPEndpoint 解析 OTLP endpoint，提取 host:port
// 支持格式：
// - http://localhost:4318
// - https://localhost:4318
// - localhost:4318
func parseOTLPEndpoint(endpoint, defaultPort string) string {
	// 如果包含 scheme，解析 URL
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		u, err := url.Parse(endpoint)
		if err != nil {
			// 如果解析失败，尝试直接提取 host:port
			return extractHostPort(endpoint)
		}
		host := u.Hostname()
		port := u.Port()
		if port == "" {
			port = defaultPort
		}
		return host + ":" + port
	}
	// 如果没有 scheme，直接返回（应该是 host:port 格式）
	return endpoint
}

// extractHostPort 从 URL 字符串中提取 host:port
func extractHostPort(endpoint string) string {
	// 移除 http:// 或 https://
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	// 移除路径部分
	if idx := strings.Index(endpoint, "/"); idx != -1 {
		endpoint = endpoint[:idx]
	}
	return endpoint
}
