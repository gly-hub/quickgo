package tracing

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	// globalTracer 全局 tracer
	globalTracer trace.Tracer
	// tp 全局 TracerProvider
	tp *tracesdk.TracerProvider
)

// Init 初始化链路追踪
func Init(config *Config) error {
	if config == nil || !config.Enabled {
		return nil
	}

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
		endpoint := parseOTLPEndpoint(config.OTLP.Endpoint)
		
		if config.OTLP.UseGRPC {
			// 使用 gRPC
			opts := []otlptracegrpc.Option{
				otlptracegrpc.WithEndpoint(endpoint),
			}
			if config.OTLP.Insecure {
				opts = append(opts, otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()))
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
		// 使用 Jaeger Exporter（已废弃，但为了兼容性保留）
		var err error
		if config.Jaeger.CollectorEndpoint != "" {
			// 使用 HTTP Collector
			opts := []jaeger.CollectorEndpointOption{
				jaeger.WithEndpoint(config.Jaeger.CollectorEndpoint),
			}
			if config.Jaeger.Username != "" {
				opts = append(opts, jaeger.WithUsername(config.Jaeger.Username))
			}
			if config.Jaeger.Password != "" {
				opts = append(opts, jaeger.WithPassword(config.Jaeger.Password))
			}
			exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(opts...))
		} else {
			// 使用 UDP Agent
			agentHost := config.Jaeger.AgentHost
			if agentHost == "" {
				agentHost = "localhost"
			}
			agentPort := config.Jaeger.AgentPort
			if agentPort == 0 {
				agentPort = 6831
			}
			exporter, err = jaeger.New(jaeger.WithAgentEndpoint(jaeger.WithAgentHost(agentHost), jaeger.WithAgentPort(fmt.Sprintf("%d", agentPort))))
			if err != nil {
				return fmt.Errorf("failed to create Jaeger agent exporter: %w", err)
			}
		}
		if err != nil {
			return fmt.Errorf("failed to create Jaeger exporter: %w", err)
		}
	} else {
		// 如果未启用任何 exporter，使用 Noop Exporter（仅本地追踪，不上传）
		// 注意：NewNoopExporter 不存在，我们使用 nil 并在后面检查
		exporter = nil
	}

	// 设置采样率
	samplingRate := config.SamplingRate
	if samplingRate < 0 {
		samplingRate = 0
	}
	if samplingRate > 1 {
		samplingRate = 1
	}
	if samplingRate == 0 {
		samplingRate = 1.0 // 默认采样所有请求
	}

	// 创建 TracerProvider
	if exporter == nil {
		// 如果没有 exporter，使用 Noop TracerProvider（仅本地追踪，不上传）
		tp = tracesdk.NewTracerProvider(
			tracesdk.WithResource(res),
			tracesdk.WithSampler(tracesdk.TraceIDRatioBased(samplingRate)),
		)
	} else {
		// 创建 TracerProvider（带 exporter，会上传到 Jaeger）
		tp = tracesdk.NewTracerProvider(
			tracesdk.WithBatcher(exporter),
			tracesdk.WithResource(res),
			tracesdk.WithSampler(tracesdk.TraceIDRatioBased(samplingRate)),
		)
	}

	// 设置全局 TracerProvider
	otel.SetTracerProvider(tp)

	// 设置全局 TextMapPropagator（用于跨服务传播 trace context）
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 创建全局 Tracer
	globalTracer = otel.Tracer(serviceName)

	return nil
}

// Shutdown 关闭链路追踪
func Shutdown(ctx context.Context) error {
	if tp != nil {
		return tp.Shutdown(ctx)
	}
	return nil
}

// GetTracer 获取 Tracer 实例
func GetTracer() trace.Tracer {
	if globalTracer == nil {
		// 如果未初始化，返回 Noop Tracer
		return trace.NewNoopTracerProvider().Tracer("noop")
	}
	return globalTracer
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
	return globalTracer != nil && tp != nil
}

// parseOTLPEndpoint 解析 OTLP endpoint，提取 host:port
// 支持格式：
// - http://localhost:4318
// - https://localhost:4318
// - localhost:4318
func parseOTLPEndpoint(endpoint string) string {
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
			// 如果没有端口，根据 scheme 设置默认端口
			if u.Scheme == "https" {
				port = "4317" // gRPC 默认端口
			} else {
				port = "4318" // HTTP 默认端口
			}
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
