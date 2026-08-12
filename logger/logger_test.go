package logger

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

	if logger.GetLevel() != LevelDebug {
		t.Errorf("Expected level LevelDebug, got %d", logger.GetLevel())
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

func TestWithContextDoesNotDuplicateTraceFields(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "logger.log")
	logger, err := NewLogger(Config{Level: LevelInfo, Output: tmpFile})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	ctx := WithTrace(context.Background(), "trace-123", "span-456")
	boundLogger := logger.WithContext(ctx)
	if boundLogger == logger {
		t.Fatal("WithContext should bind the provided trace context")
	}
	boundLogger.Info(context.Background(), "test message")

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var entry LogEntry
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if entry.TraceID != "trace-123" || entry.SpanID != "span-456" {
		t.Fatalf("unexpected trace context: trace=%q span=%q", entry.TraceID, entry.SpanID)
	}
	if len(entry.Fields) != 0 {
		t.Fatalf("trace fields should not be duplicated in fields: %#v", entry.Fields)
	}
}

func TestWithFieldsTracksLevelChanges(t *testing.T) {
	logger, err := NewLogger(Config{Level: LevelInfo})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	fieldLogger := logger.WithField("component", "test")
	logger.SetLevel(LevelError)
	if got := fieldLogger.GetLevel(); got != LevelError {
		t.Fatalf("derived logger level = %d, want %d", got, LevelError)
	}
}

func TestAsyncLoggerFlushAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logger.log")
	logger, err := NewLogger(Config{Level: LevelInfo, Output: path, Async: true, BufferSize: 1})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	logger.Info(context.Background(), "first message")
	logger.Info(context.Background(), "second message")
	if err := logger.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(content), "first message") || !strings.Contains(string(content), "second message") {
		t.Fatalf("flush did not persist queued logs: %s", content)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
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

func TestErrorLogFormatsErrorPlaceholder(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "logger_test_*.log")
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, _ := NewLogger(Config{
		Level:  LevelError,
		Output: tmpFile.Name(),
	})
	defer logger.Close()

	ctx := context.Background()
	testErr := errors.New("context deadline exceeded")
	logger.Error(ctx, "failed to connect: address=%s, error=%v", "etcd://ai-agent", testErr)

	content, _ := os.ReadFile(tmpFile.Name())
	var entry LogEntry
	json.Unmarshal(content, &entry)

	if entry.Error != "context deadline exceeded" {
		t.Errorf("Expected error 'context deadline exceeded', got '%s'", entry.Error)
	}

	expected := "failed to connect: address=etcd://ai-agent, error=context deadline exceeded"
	if entry.Message != expected {
		t.Errorf("Expected message %q, got %q", expected, entry.Message)
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

func TestGlobalCloseResetsDefaultLogger(t *testing.T) {
	if err := Init(Config{Level: LevelDebug}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	first := GetDefault()
	if first == nil {
		t.Fatal("expected default logger")
	}
	if err := Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	second := GetDefault()
	if second == nil {
		t.Fatal("expected default logger to be recreated")
	}
	if second == first {
		t.Fatal("expected Close to reset default logger")
	}
}

func TestInitClosesPreviousGlobalLogger(t *testing.T) {
	defer Close()
	firstPath := filepath.Join(t.TempDir(), "first.log")
	secondPath := filepath.Join(t.TempDir(), "second.log")
	if err := Init(Config{Level: LevelInfo, Output: firstPath}); err != nil {
		t.Fatalf("first Init failed: %v", err)
	}
	first := GetDefault()
	firstOutput := first.output

	if err := Init(Config{Level: LevelInfo, Output: secondPath}); err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
	if GetDefault() == first {
		t.Fatal("expected second Init to replace the global logger")
	}
	if _, err := firstOutput.Write([]byte("closed\n")); err == nil {
		t.Fatal("expected first logger output to be closed after reinitialization")
	}
}

func TestLoggerCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logger.log")
	logger, err := NewLogger(Config{Level: LevelInfo, Output: path})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestCloseIfDefaultDoesNotCloseReplacement(t *testing.T) {
	defer Close()
	firstPath := filepath.Join(t.TempDir(), "first.log")
	secondPath := filepath.Join(t.TempDir(), "second.log")
	if err := Init(Config{Level: LevelInfo, Output: firstPath}); err != nil {
		t.Fatalf("first Init failed: %v", err)
	}
	first := GetDefault()
	if err := Init(Config{Level: LevelInfo, Output: secondPath}); err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
	second := GetDefault()

	closed, err := CloseIfDefault(first)
	if err != nil {
		t.Fatalf("CloseIfDefault failed: %v", err)
	}
	if closed {
		t.Fatal("expected replacement logger to remain global")
	}
	if GetDefault() != second {
		t.Fatal("CloseIfDefault replaced the current global logger")
	}
}

// TestCallerInfo 测试调用者信息
func TestCallerInfo(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "logger_test_*.log")
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger, _ := NewLogger(Config{
		Level:        LevelInfo,
		Output:       tmpFile.Name(),
		EnableCaller: true,
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

func TestCallerDisabledByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logger.log")
	logger, err := NewLogger(Config{Level: LevelInfo, Output: path})
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	logger.Info(context.Background(), "test message")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var entry LogEntry
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if entry.Caller != "" {
		t.Fatalf("caller should be disabled by default, got %q", entry.Caller)
	}
}

func BenchmarkDebugFiltered(b *testing.B) {
	logger, err := NewLogger(Config{Level: LevelInfo})
	if err != nil {
		b.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Debug(context.Background(), "user=%d route=%s", i, "/v1/orders")
	}
}

func BenchmarkInfoAsync(b *testing.B) {
	logger, err := NewLogger(Config{
		Level:      LevelInfo,
		Output:     os.DevNull,
		Async:      true,
		BufferSize: 4096,
	})
	if err != nil {
		b.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info(context.Background(), "user=%d route=%s", i, "/v1/orders")
	}
	b.StopTimer()
	if err := logger.Flush(); err != nil {
		b.Fatalf("Flush failed: %v", err)
	}
}

func BenchmarkInfoWithCaller(b *testing.B) {
	logger, err := NewLogger(Config{Level: LevelInfo, Output: os.DevNull, EnableCaller: true})
	if err != nil {
		b.Fatalf("NewLogger failed: %v", err)
	}
	defer logger.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info(context.Background(), "user=%d", i)
	}
}
