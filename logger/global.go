package logger

import (
	"context"
	"fmt"
)

var (
	// defaultLogger 默认的全局日志记录器
	defaultLogger *Logger
)

// Init 初始化全局日志记录器
func Init(config Config) error {
	logger, err := NewLogger(config)
	if err != nil {
		return err
	}
	defaultLogger = logger
	return nil
}

// SetDefault 设置默认日志记录器
func SetDefault(logger *Logger) {
	defaultLogger = logger
}

// GetDefault 获取默认日志记录器
func GetDefault() *Logger {
	if defaultLogger == nil {
		// 如果没有初始化，创建一个默认的
		defaultLogger, _ = NewLogger(Config{
			Level:   LevelInfo,
			Service: "default",
		})
	}
	return defaultLogger
}

// Debug 使用默认日志记录器记录调试日志，支持 fmt.Sprintf 风格格式化
func Debug(ctx context.Context, format string, args ...interface{}) {
	GetDefault().Debug(ctx, format, args...)
}

// Info 使用默认日志记录器记录信息日志，支持 fmt.Sprintf 风格格式化
func Info(ctx context.Context, format string, args ...interface{}) {
	GetDefault().Info(ctx, format, args...)
}

// Warn 使用默认日志记录器记录警告日志，支持 fmt.Sprintf 风格格式化
func Warn(ctx context.Context, format string, args ...interface{}) {
	GetDefault().Warn(ctx, format, args...)
}

// Error 使用默认日志记录器记录错误日志，支持 fmt.Sprintf 风格格式化
func Error(ctx context.Context, format string, args ...interface{}) {
	GetDefault().Error(ctx, format, args...)
}

// Fatal 使用默认日志记录器记录致命错误日志，支持 fmt.Sprintf 风格格式化
func Fatal(ctx context.Context, format string, args ...interface{}) {
	GetDefault().Fatal(ctx, format, args...)
}

// WithFields 使用默认日志记录器添加字段
func WithFields(fields map[string]interface{}) *Logger {
	return GetDefault().WithFields(fields)
}

// WithField 使用默认日志记录器添加单个字段
func WithField(key string, value interface{}) *Logger {
	return GetDefault().WithField(key, value)
}

// WithContext 使用默认日志记录器从 context 提取链路信息
func WithContext(ctx context.Context) *Logger {
	return GetDefault().WithContext(ctx)
}

// SetLevel 设置默认日志记录器的级别
func SetLevel(level Level) {
	GetDefault().SetLevel(level)
}

// Close 关闭默认日志记录器
func Close() error {
	if defaultLogger != nil {
		return defaultLogger.Close()
	}
	return nil
}

// MustInit 初始化全局日志记录器，失败则 panic
func MustInit(config Config) {
	if err := Init(config); err != nil {
		panic(fmt.Sprintf("failed to init logger: %v", err))
	}
}
