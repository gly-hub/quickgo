package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Level 日志级别
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
	LevelFatal: "FATAL",
}

// Logger 日志记录器
type Logger struct {
	level      Level
	output     *os.File
	service    string
	version    string
	fields     map[string]interface{}
	callerSkip int
}

// Config 日志配置
type Config struct {
	Level      Level  // 日志级别
	Output     string // 输出文件路径，空则输出到 stdout
	Service    string // 服务名称
	Version    string // 服务版本
	CallerSkip int    // 调用栈跳过层数，默认 2
}

// LogEntry 日志条目
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Service   string                 `json:"service,omitempty"`
	Version   string                 `json:"version,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
	SpanID    string                 `json:"span_id,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// NewLogger 创建新的日志记录器
func NewLogger(config Config) (*Logger, error) {
	if config.CallerSkip == 0 {
		// 默认 skip 为 2，因为：
		// skip 0 = runtime.Caller 自己
		// skip 1 = log() 方法
		// skip 2 = Info()/Debug()/Warn()/Error()/Fatal() 方法
		// skip 3 = 用户代码（实际调用位置）
		// 在 log() 方法中会 +1，所以这里设置为 2，实际会跳过 3 层
		config.CallerSkip = 2
	}

	logger := &Logger{
		level:      config.Level,
		service:    config.Service,
		version:    config.Version,
		fields:     make(map[string]interface{}),
		callerSkip: config.CallerSkip,
	}

	// 设置输出
	if config.Output == "" {
		logger.output = os.Stdout
	} else {
		file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		logger.output = file
	}

	return logger, nil
}

// WithFields 添加字段
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newLogger := *l
	newLogger.fields = make(map[string]interface{})
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return &newLogger
}

// WithField 添加单个字段
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return l.WithFields(map[string]interface{}{key: value})
}

// WithContext 从 context 中提取链路信息
func (l *Logger) WithContext(ctx context.Context) *Logger {
	traceID := GetTraceID(ctx)
	spanID := GetSpanID(ctx)

	logger := l.WithFields(map[string]interface{}{
		"trace_id": traceID,
		"span_id":  spanID,
	})
	return logger
}

// log 内部日志方法
func (l *Logger) log(ctx context.Context, level Level, msg string, err error, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	// 合并字段
	allFields := make(map[string]interface{})
	for k, v := range l.fields {
		allFields[k] = v
	}
	for k, v := range fields {
		allFields[k] = v
	}

	// 获取调用者信息（从项目根目录开始的完整路径）
	// 调用链分析：
	// skip 0 = runtime.Caller 自己
	// skip 1 = log() 方法
	// skip 2 = Info()/Debug()/Warn()/Error()/Fatal() 方法（Logger 的方法）
	// skip 3 = 用户代码（直接使用 logger.Info）或全局函数（logger.Info）
	// skip 4 = 用户代码（使用全局函数 logger.Info）
	caller := ""
	callerShort := "" // 用于控制台显示的简短格式

	// 使用 callerSkip + 1 来跳过 log() 方法
	// 默认 callerSkip = 2，所以实际 skip = 3
	skip := l.callerSkip + 1

	// 检查 skip 3 的位置是否是 logger 包内的函数（可能是全局函数）
	// 如果是，则使用 skip 4 来获取真正的用户代码位置
	if pc, file, line, ok := runtime.Caller(skip); ok {
		// 检查是否是 logger 包内的非测试文件（可能是全局函数）
		if strings.Contains(file, "/logger/") && !strings.Contains(file, "_test.go") && !strings.Contains(file, "example.go") {
			// 可能是全局函数，尝试 skip 4
			if pc2, file2, line2, ok2 := runtime.Caller(skip + 1); ok2 {
				pc, file, line = pc2, file2, line2
			}
		}

		// 获取函数信息
		fn := runtime.FuncForPC(pc)
		if fn != nil {
			funcName := fn.Name()
			// 简化函数名
			if idx := strings.LastIndex(funcName, "."); idx >= 0 {
				funcName = funcName[idx+1:]
			}

			// 获取项目根目录
			projectRoot := getProjectRoot()

			// 获取相对于项目根目录的路径
			relPath := file
			if projectRoot != "" {
				if rel, err := filepath.Rel(projectRoot, file); err == nil {
					relPath = rel
				}
			}

			// 用于 JSON 的完整路径（包含函数名）
			caller = fmt.Sprintf("%s:%d:%s", relPath, line, funcName)

			// 用于控制台显示的简短格式（只包含路径和行号）
			callerShort = fmt.Sprintf("%s:%d", relPath, line)
		}
	}

	// 从 context 获取链路信息
	traceID := GetTraceID(ctx)
	spanID := GetSpanID(ctx)

	// 判断是否是控制台输出
	isConsole := l.output == os.Stdout || l.output == os.Stderr

	if isConsole {
		// 控制台输出：使用易读的文本格式
		// 格式：时间 [级别] 日志信息 [trace_id:xxx]
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		levelStr := levelNames[level]

		// 构建日志信息
		logMsg := msg
		if err != nil {
			logMsg = fmt.Sprintf("%s | error: %s", msg, err.Error())
		}

		// 输出格式：时间 [级别] 日志信息 [trace_id:xxx] [to/file.go:123]
		var parts []string
		parts = append(parts, timestamp, fmt.Sprintf("[%s]", levelStr), logMsg)

		if traceID != "" {
			parts = append(parts, fmt.Sprintf("[trace_id:%s]", traceID))
		}

		if callerShort != "" {
			parts = append(parts, fmt.Sprintf("[%s]", callerShort))
		}

		fmt.Fprintf(l.output, "%s\n", strings.Join(parts, " "))
	} else {
		// 文件输出：使用 JSON 格式
		entry := LogEntry{
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Level:     levelNames[level],
			Service:   l.service,
			Version:   l.version,
			TraceID:   traceID,
			SpanID:    spanID,
			Caller:    caller,
			Message:   msg,
			Fields:    allFields,
		}

		if err != nil {
			entry.Error = err.Error()
		}

		// 序列化为 JSON
		data, jsonErr := json.Marshal(entry)
		if jsonErr != nil {
			// 如果 JSON 序列化失败，使用简单格式
			fmt.Fprintf(l.output, "[%s] %s: %s\n", levelNames[level], time.Now().Format(time.RFC3339), msg)
			return
		}

		// 输出日志
		fmt.Fprintln(l.output, string(data))
	}
}

// Debug 调试日志，支持 fmt.Sprintf 风格格式化
func (l *Logger) Debug(ctx context.Context, format string, args ...interface{}) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.log(ctx, LevelDebug, msg, nil, nil)
}

// Info 信息日志，支持 fmt.Sprintf 风格格式化
func (l *Logger) Info(ctx context.Context, format string, args ...interface{}) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.log(ctx, LevelInfo, msg, nil, nil)
}

// Warn 警告日志，支持 fmt.Sprintf 风格格式化
func (l *Logger) Warn(ctx context.Context, format string, args ...interface{}) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.log(ctx, LevelWarn, msg, nil, nil)
}

// Error 错误日志，支持 fmt.Sprintf 风格格式化
// 如果最后一个参数是 error，会被提取为独立的 error 字段；否则所有参数用于格式化消息
func (l *Logger) Error(ctx context.Context, format string, args ...interface{}) {
	msg := format
	var err error

	if len(args) > 0 {
		// 检查最后一个参数是否是 error
		if e, ok := args[len(args)-1].(error); ok {
			err = e
			// 从 args 中移除 error，用于格式化消息
			args = args[:len(args)-1]
		}
		// 格式化消息
		if len(args) > 0 {
			msg = fmt.Sprintf(format, args...)
		} else if err == nil {
			// 如果没有格式化参数且没有 error，直接使用 format
			msg = format
		}
	}
	l.log(ctx, LevelError, msg, err, nil)
}

// Fatal 致命错误日志（会调用 os.Exit(1)），支持 fmt.Sprintf 风格格式化
// 如果最后一个参数是 error，会被提取为独立的 error 字段；否则所有参数用于格式化消息
func (l *Logger) Fatal(ctx context.Context, format string, args ...interface{}) {
	msg := format
	var err error

	if len(args) > 0 {
		// 检查最后一个参数是否是 error
		if e, ok := args[len(args)-1].(error); ok {
			err = e
			// 从 args 中移除 error，用于格式化消息
			args = args[:len(args)-1]
		}
		// 格式化消息
		if len(args) > 0 {
			msg = fmt.Sprintf(format, args...)
		} else if err == nil {
			// 如果没有格式化参数且没有 error，直接使用 format
			msg = format
		}
	}
	l.log(ctx, LevelFatal, msg, err, nil)
	os.Exit(1)
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level Level) {
	l.level = level
}

// GetLevel 获取日志级别
func (l *Logger) GetLevel() Level {
	return l.level
}

// Close 关闭日志记录器
func (l *Logger) Close() error {
	if l.output != nil && l.output != os.Stdout && l.output != os.Stderr {
		return l.output.Close()
	}
	return nil
}

// getProjectRoot 获取项目根目录
// 通过查找 go.mod 文件来确定项目根目录
func getProjectRoot() string {
	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// 从当前目录向上查找 go.mod 文件
	dir := wd
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir
		}

		// 向上查找父目录
		parent := filepath.Dir(dir)
		if parent == dir {
			// 已经到达根目录
			break
		}
		dir = parent
	}

	// 如果找不到 go.mod，返回当前工作目录
	return wd
}
