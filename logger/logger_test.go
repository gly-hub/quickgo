package logger

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNewLogger 测试创建日志记录器
func TestNewLogger(t *testing.T) {
	config := Config{
		Level:   LevelDebug,
		Service: "test-service",
		Version: "1.0.0",
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	if logger.service != "test-service" {
		t.Errorf("Expected service 'test-service', got '%s'", logger.service)
	}

	if logger.version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", logger.version)
	}

	if logger.level != LevelInfo {
		t.Errorf("Expected level LevelInfo, got %d", logger.level)
	}
	logger.Debug(context.Background(), "debug msg")
}

// TestLoggerWithFile 测试文件输出
func TestLoggerWithFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "logger_test_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	config := Config{
		Level:  LevelInfo,
		Output: tmpFile.Name(),
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	logger.Info(ctx, "test message")

	// 读取文件内容
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test message") {
		t.Errorf("Log file should contain 'test message', got: %s", string(content))
	}
}

// TestLogLevels 测试日志级别
func TestLogLevels(t *testing.T) {
	tests := []struct {
		name     string
		level    Level
		logFunc  func(*Logger, context.Context)
		expected bool
	}{
		{"Debug at Debug level", LevelDebug, func(l *Logger, ctx context.Context) { l.Debug(ctx, "test") }, true},
		{"Debug at Info level", LevelInfo, func(l *Logger, ctx context.Context) { l.Debug(ctx, "test") }, false},
		{"Info at Info level", LevelInfo, func(l *Logger, ctx context.Context) { l.Info(ctx, "test") }, true},
		{"Info at Warn level", LevelWarn, func(l *Logger, ctx context.Context) { l.Info(ctx, "test") }, false},
		{"Warn at Warn level", LevelWarn, func(l *Logger, ctx context.Context) { l.Warn(ctx, "test") }, true},
		{"Warn at Error level", LevelError, func(l *Logger, ctx context.Context) { l.Warn(ctx, "test") }, false},
		{"Error at Error level", LevelError, func(l *Logger, ctx context.Context) { l.Error(ctx, "test: %s", "error", errors.New("test error")) }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, _ := os.CreateTemp("", "logger_test_*.log")
			defer os.Remove(tmpFile.Name())
			tmpFile.Close()

			config := Config{
				Level:  tt.level,
				Output: tmpFile.Name(),
			}

			logger, _ := NewLogger(config)
			defer logger.Close()

			ctx := context.Background()
			tt.logFunc(logger, ctx)

			content, _ := os.ReadFile(tmpFile.Name())
			hasLog := len(content) > 0

			if hasLog != tt.expected {
				t.Errorf("Expected log %v, got %v. Content: %s", tt.expected, hasLog, string(content))
			}
		})
	}
}

// TestLogFormat 测试日志格式
func TestLogFormat(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "logger_test_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	config := Config{
		Level:   LevelInfo,
		Service: "test-service",
		Version: "1.0.0",
		Output:  tmpFile.Name(),
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	ctx := WithTrace(context.Background(), "trace-123", "span-456")
	logger.Info(ctx, "test message: key1=%s, key2=%d", "value1", 123)

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var entry LogEntry
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v. Content: %s", err, string(content))
	}

	if entry.Level != "INFO" {
		t.Errorf("Expected level 'INFO', got '%s'", entry.Level)
	}

	if entry.Service != "test-service" {
		t.Errorf("Expected service 'test-service', got '%s'", entry.Service)
	}

	if entry.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", entry.Version)
	}

	if entry.TraceID != "trace-123" {
		t.Errorf("Expected trace_id 'trace-123', got '%s'", entry.TraceID)
	}

	if entry.SpanID != "span-456" {
		t.Errorf("Expected span_id 'span-456', got '%s'", entry.SpanID)
	}

	if entry.Message != "test message: key1=value1, key2=123" {
		t.Errorf("Expected message 'test message: key1=value1, key2=123', got '%s'", entry.Message)
	}

	// 验证时间戳格式
	if _, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err != nil {
		t.Errorf("Invalid timestamp format: %v", err)
	}
}

// TestWithFields 测试字段添加
func TestWithFields(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "logger_test_*.log")
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, _ := NewLogger(Config{
		Level:  LevelInfo,
		Output: tmpFile.Name(),
	})
	defer logger.Close()

	// 添加字段
	fieldLogger := logger.WithFields(map[string]interface{}{
		"module": "user",
		"env":    "test",
	})

	ctx := context.Background()
	fieldLogger.Info(ctx, "test message: action=%s", "login")

	content, _ := os.ReadFile(tmpFile.Name())
	var entry LogEntry
	json.Unmarshal(content, &entry)

	if entry.Fields["module"] != "user" {
		t.Errorf("Expected module='user', got '%v'", entry.Fields["module"])
	}

	if entry.Fields["env"] != "test" {
		t.Errorf("Expected env='test', got '%v'", entry.Fields["env"])
	}

	if entry.Message != "test message: action=login" {
		t.Errorf("Expected message 'test message: action=login', got '%s'", entry.Message)
	}
}

// TestWithField 测试单个字段添加
func TestWithField(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "logger_test_*.log")
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, _ := NewLogger(Config{
		Level:  LevelInfo,
		Output: tmpFile.Name(),
	})
	defer logger.Close()

	fieldLogger := logger.WithField("request_id", "req-123")
	ctx := context.Background()
	fieldLogger.Info(ctx, "test message")

	content, _ := os.ReadFile(tmpFile.Name())
	var entry LogEntry
	json.Unmarshal(content, &entry)

	if entry.Fields["request_id"] != "req-123" {
		t.Errorf("Expected request_id='req-123', got '%v'", entry.Fields["request_id"])
	}
}

// TestErrorLog 测试错误日志
func TestErrorLog(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "logger_test_*.log")
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, _ := NewLogger(Config{
		Level:  LevelError,
		Output: tmpFile.Name(),
	})
	defer logger.Close()

	ctx := context.Background()
	testErr := errors.New("test error message")
	logger.Error(ctx, "operation failed: code=%d", 500, testErr)

	content, _ := os.ReadFile(tmpFile.Name())
	var entry LogEntry
	json.Unmarshal(content, &entry)

	if entry.Level != "ERROR" {
		t.Errorf("Expected level 'ERROR', got '%s'", entry.Level)
	}

	if entry.Error != "test error message" {
		t.Errorf("Expected error 'test error message', got '%s'", entry.Error)
	}

	if entry.Message != "operation failed: code=500" {
		t.Errorf("Expected message 'operation failed: code=500', got '%s'", entry.Message)
	}
}

// TestSetLevel 测试设置日志级别
func TestSetLevel(t *testing.T) {
	logger, _ := NewLogger(Config{
		Level: LevelInfo,
	})
	defer logger.Close()

	if logger.GetLevel() != LevelInfo {
		t.Errorf("Expected level LevelInfo, got %d", logger.GetLevel())
	}

	logger.SetLevel(LevelWarn)
	if logger.GetLevel() != LevelWarn {
		t.Errorf("Expected level LevelWarn, got %d", logger.GetLevel())
	}
}

// TestCallerInfo 测试调用者信息
func TestCallerInfo(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "logger_test_*.log")
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, _ := NewLogger(Config{
		Level:  LevelInfo,
		Output: tmpFile.Name(),
	})
	defer logger.Close()

	ctx := context.Background()
	logger.Info(ctx, "test message")

	content, _ := os.ReadFile(tmpFile.Name())
	var entry LogEntry
	json.Unmarshal(content, &entry)

	if entry.Caller == "" {
		t.Error("Expected caller info, got empty string")
	}

	if !strings.Contains(entry.Caller, "logger_test.go") {
		t.Errorf("Expected caller to contain 'logger_test.go', got '%s'", entry.Caller)
	}
}
