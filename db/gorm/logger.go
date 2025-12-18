package gorm

import (
	"context"
	"regexp"
	"strings"
	"time"

	frameworkLogger "quickgo/logger"
	"quickgo/tracing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"gorm.io/gorm/logger"
)

// newLogger 创建 GORM 日志适配器
func newLogger(config *GormConfig) logger.Interface {
	if !config.EnableLog {
		return logger.Default.LogMode(logger.Silent)
	}

	slowThreshold := time.Duration(config.SlowThreshold) * time.Millisecond
	if slowThreshold == 0 {
		slowThreshold = 200 * time.Millisecond // 默认 200ms
	}

	// 根据配置的日志级别设置
	var logLevel logger.LogLevel
	switch config.LogLevel {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	case "info":
		logLevel = logger.Info
	default:
		logLevel = logger.Info
	}

	// 创建自定义 logger
	return &gormLogger{
		config:        config,
		slowThreshold: slowThreshold,
		logLevel:      logLevel,
	}
}

// gormLogger 自定义 GORM logger，实现 logger.Interface
type gormLogger struct {
	config        *GormConfig
	slowThreshold time.Duration
	logLevel      logger.LogLevel
}

// LogMode 设置日志级别
func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *l
	newLogger.logLevel = level
	return &newLogger
}

// Info 实现 logger.Interface.Info
func (l *gormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Info {
		frameworkLogger.Info(ctx, "[GORM] "+msg, data...)
	}
}

// Warn 实现 logger.Interface.Warn
func (l *gormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Warn {
		frameworkLogger.Warn(ctx, "[GORM] "+msg, data...)
	}
}

// Error 实现 logger.Interface.Error
func (l *gormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= logger.Error {
		frameworkLogger.Error(ctx, "[GORM] "+msg, data...)
	}
}

// Trace 实现 logger.Interface.Trace
// 这是最重要的方法，GORM 的 SQL 查询日志通过这里输出
func (l *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.logLevel <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	// 去除日志消息中的文件路径（格式：[/path/to/file.go:123]）
	// GORM 默认会在日志末尾添加文件路径，我们需要去除它
	sql = removeFilePath(sql)

	// 如果启用了 OpenTelemetry tracing，创建数据库操作的 span
	var span trace.Span
	if tracing.IsEnabled() {
		ctx, span = tracing.StartSpan(ctx, "gorm.query")
		defer span.End()

		// 设置 span 属性
		span.SetAttributes(
			attribute.String("db.system", "gorm"),
			attribute.String("db.statement", sql),
			attribute.Int64("db.rows_affected", rows),
			attribute.Float64("db.duration_ms", float64(elapsed.Nanoseconds())/1e6),
		)

		// 添加 trace_id 到 span attributes（方便在 Jaeger 中查询）
		tracing.AddTraceIDToSpan(span, ctx)

		// 检测是否是慢查询
		if elapsed > l.slowThreshold && l.slowThreshold != 0 {
			span.SetAttributes(attribute.Bool("db.slow_query", true))
		}

		// 设置错误状态
		if err != nil {
			tracing.SetSpanError(span, err)
		}
	}

	switch {
	case err != nil && l.logLevel >= logger.Error:
		// 错误日志
		frameworkLogger.Error(ctx, "[GORM] [%.3fms] [rows:%d] %s | error: %v",
			float64(elapsed.Nanoseconds())/1e6, rows, sql, err)
	case elapsed > l.slowThreshold && l.slowThreshold != 0 && l.logLevel >= logger.Warn:
		// 慢查询日志
		frameworkLogger.Warn(ctx, "[GORM] [%.3fms] [rows:%d] %s | slow query",
			float64(elapsed.Nanoseconds())/1e6, rows, sql)
	case l.logLevel >= logger.Info:
		// 普通查询日志
		frameworkLogger.Info(ctx, "[GORM] [%.3fms] [rows:%d] %s",
			float64(elapsed.Nanoseconds())/1e6, rows, sql)
	}
}

// removeFilePath 去除日志消息中的文件路径
// GORM 会在日志末尾添加文件路径，格式：[/path/to/file.go:123]
// 我们需要去除这部分，避免重复输出
func removeFilePath(msg string) string {
	// 匹配文件路径模式：[/path/to/file.go:123] 或 [file.go:123]
	// 这个模式通常在消息的末尾
	filePathPattern := regexp.MustCompile(`\s*\[[^\]]+\.go:\d+\]\s*$`)

	// 去除末尾的文件路径
	msg = filePathPattern.ReplaceAllString(msg, "")

	// 同时去除可能的重复路径（如果 GORM 在消息中间也添加了路径）
	// 匹配格式：[replica] 或其他标签后的文件路径
	msg = regexp.MustCompile(`\[[^\]]+\.go:\d+\]`).ReplaceAllString(msg, "")

	// 清理多余的空格
	msg = strings.TrimSpace(msg)

	return msg
}
