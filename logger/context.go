package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type contextKey string

const (
	traceIDKey contextKey = "trace_id"
	spanIDKey  contextKey = "span_id"
)

// WithTraceID 在 context 中设置 trace ID
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// WithSpanID 在 context 中设置 span ID
func WithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, spanIDKey, spanID)
}

// WithTrace 在 context 中设置 trace ID 和 span ID
func WithTrace(ctx context.Context, traceID, spanID string) context.Context {
	ctx = WithTraceID(ctx, traceID)
	ctx = WithSpanID(ctx, spanID)
	return ctx
}

// GetTraceID 从 context 中获取 trace ID
func GetTraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// GetSpanID 从 context 中获取 span ID
func GetSpanID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if spanID, ok := ctx.Value(spanIDKey).(string); ok {
		return spanID
	}
	return ""
}

// GenerateTraceID 生成新的 trace ID
func GenerateTraceID() string {
	return generateID(16)
}

// GenerateSpanID 生成新的 span ID
func GenerateSpanID() string {
	return generateID(8)
}

// generateID 生成随机 ID
func generateID(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// 如果随机数生成失败，使用时间戳
		return fmt.Sprintf("%d", length)
	}
	return hex.EncodeToString(bytes)
}

// StartSpan 开始一个新的 span，返回带有新 span ID 的 context
func StartSpan(ctx context.Context) context.Context {
	traceID := GetTraceID(ctx)
	if traceID == "" {
		// 如果没有 trace ID，创建一个新的
		traceID = GenerateTraceID()
		ctx = WithTraceID(ctx, traceID)
	}
	
	// 生成新的 span ID
	spanID := GenerateSpanID()
	return WithSpanID(ctx, spanID)
}

// WithParentSpan 使用父 span 的 trace ID 创建新的 span
func WithParentSpan(ctx context.Context) context.Context {
	traceID := GetTraceID(ctx)
	if traceID == "" {
		traceID = GenerateTraceID()
	}
	
	spanID := GenerateSpanID()
	return WithTrace(ctx, traceID, spanID)
}

