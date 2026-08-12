package logger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 用于缓存包名的变量
var (
	loggerPackage     string
	callerInitOnce    sync.Once
	projectRoot       string
	projectRootOnce   sync.Once
	knownLoggerFrames = 4
	formatVerbPattern = regexp.MustCompile(`%(?:\[[0-9]+\])?[-+#0 ]*(?:\*|\[[0-9]+\]\*)?(?:\.(?:\*|\[[0-9]+\]\*))?[bcdefgGopqstTUvVxX%]`)
	frameworkPackages = []string{
		"google.golang.org/grpc",
		"github.com/spf13/cobra",
		"go.opentelemetry.io",
		"gorm.io",
	}
)

const maximumCallDepth = 25

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
	level         *atomic.Int32
	output        *os.File
	service       string
	version       string
	fields        map[string]interface{}
	traceID       string
	spanID        string
	callerSkip    int
	disableCaller bool
	mu            *sync.RWMutex
	writer        *writerState
}

// Config 日志配置
type Config struct {
	Level         Level  // 日志级别
	Output        string // 输出文件路径，空则输出到 stdout
	Service       string // 服务名称
	Version       string // 服务版本
	CallerSkip    int    // 调用栈跳过层数，0表示使用动态检测
	EnableCaller  bool   // 是否记录调用者信息，默认 false
	DisableCaller bool   // Deprecated: 使用 EnableCaller；设置为 true 会强制禁用调用者信息
	Async         bool   // 是否异步写出日志，默认 false
	BufferSize    int    // 异步写出队列长度，默认 1024
}

const defaultBufferSize = 1024

type logItem struct {
	data  []byte
	entry *LogEntry
	flush chan struct{}
}

type writerState struct {
	async  bool
	queue  chan logItem
	worker sync.WaitGroup
	closed bool
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
	// CallerSkip 为 0 表示使用动态检测，不需要设置默认值

	level := &atomic.Int32{}
	level.Store(int32(config.Level))
	enableCaller := config.EnableCaller || config.CallerSkip > 0
	if config.DisableCaller {
		enableCaller = false
	}

	logger := &Logger{
		level:         level,
		service:       config.Service,
		version:       config.Version,
		fields:        make(map[string]interface{}),
		callerSkip:    config.CallerSkip,
		disableCaller: !enableCaller,
		mu:            &sync.RWMutex{},
		writer:        &writerState{async: config.Async},
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
	if logger.writer.async {
		bufferSize := config.BufferSize
		if bufferSize <= 0 {
			bufferSize = defaultBufferSize
		}
		logger.writer.queue = make(chan logItem, bufferSize)
		logger.writer.worker.Add(1)
		go logger.runWriter()
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
	newLogger := *l
	newLogger.fields = make(map[string]interface{}, len(l.fields)+1)
	for fieldKey, fieldValue := range l.fields {
		newLogger.fields[fieldKey] = fieldValue
	}
	newLogger.fields[key] = value
	return &newLogger
}

// WithContext 从 context 中提取链路信息
func (l *Logger) WithContext(ctx context.Context) *Logger {
	traceID := GetTraceID(ctx)
	spanID := GetSpanID(ctx)
	if traceID == "" && spanID == "" {
		return l
	}
	newLogger := *l
	newLogger.traceID = traceID
	newLogger.spanID = spanID
	return &newLogger
}

// log 内部日志方法
func (l *Logger) log(ctx context.Context, level Level, msg string, err error, fields map[string]interface{}) {
	if !l.isEnabled(level) {
		return
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.writer.closed || !l.isEnabled(level) {
		return
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

	if !l.disableCaller {
		// 获取调用者信息
		var pc uintptr
		var file string
		var line int

		if l.callerSkip > 0 {
			// 使用固定的跳过层级
			skip := l.callerSkip + 1
			var ok bool
			pc, file, line, ok = runtime.Caller(skip)
			if !ok {
				pc, file, line = 0, "", 0
			}
		} else {
			// 使用动态检测
			pc, file, line = getCaller()
		}

		// 获取函数信息
		if pc != 0 {
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
	}

	// 从 context 获取链路信息
	traceID := GetTraceID(ctx)
	spanID := GetSpanID(ctx)
	if traceID == "" {
		traceID = l.traceID
	}
	if spanID == "" {
		spanID = l.spanID
	}

	isConsole := l.output == os.Stdout || l.output == os.Stderr
	allFields := l.fields
	if !isConsole && len(fields) > 0 {
		allFields = make(map[string]interface{}, len(l.fields)+len(fields))
		for k, v := range l.fields {
			allFields[k] = v
		}
		for k, v := range fields {
			allFields[k] = v
		}
	}

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
		var builder strings.Builder
		builder.Grow(len(timestamp) + len(levelStr) + len(logMsg) + len(traceID) + len(callerShort) + 24)
		builder.WriteString(timestamp)
		builder.WriteString(" [")
		builder.WriteString(levelStr)
		builder.WriteString("] ")
		builder.WriteString(logMsg)
		if traceID != "" {
			builder.WriteString(" [trace_id:")
			builder.WriteString(traceID)
			builder.WriteByte(']')
		}
		if callerShort != "" {
			builder.WriteString(" [")
			builder.WriteString(callerShort)
			builder.WriteByte(']')
		}
		builder.WriteByte('\n')
		l.write(logItem{data: []byte(builder.String())})
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

		l.write(logItem{entry: &entry})
	}
}

// Debug 调试日志，支持 fmt.Sprintf 风格格式化
func (l *Logger) Debug(ctx context.Context, format string, args ...interface{}) {
	if !l.isEnabled(LevelDebug) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.log(ctx, LevelDebug, msg, nil, nil)
}

// Info 信息日志，支持 fmt.Sprintf 风格格式化
func (l *Logger) Info(ctx context.Context, format string, args ...interface{}) {
	if !l.isEnabled(LevelInfo) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.log(ctx, LevelInfo, msg, nil, nil)
}

// Warn 警告日志，支持 fmt.Sprintf 风格格式化
func (l *Logger) Warn(ctx context.Context, format string, args ...interface{}) {
	if !l.isEnabled(LevelWarn) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.log(ctx, LevelWarn, msg, nil, nil)
}

// Error 错误日志，支持 fmt.Sprintf 风格格式化
// 如果最后一个参数是 error，会被提取为独立的 error 字段；否则所有参数用于格式化消息
func (l *Logger) Error(ctx context.Context, format string, args ...interface{}) {
	if !l.isEnabled(LevelError) {
		return
	}
	msg, err := formatLogMessage(format, args...)
	l.log(ctx, LevelError, msg, err, nil)
}

// Fatal 致命错误日志（会调用 os.Exit(1)），支持 fmt.Sprintf 风格格式化
// 如果最后一个参数是 error，会被提取为独立的 error 字段；否则所有参数用于格式化消息
func (l *Logger) Fatal(ctx context.Context, format string, args ...interface{}) {
	if l.isEnabled(LevelFatal) {
		msg, err := formatLogMessage(format, args...)
		l.log(ctx, LevelFatal, msg, err, nil)
		_ = l.Flush()
	}
	os.Exit(1)
}

func formatLogMessage(format string, args ...interface{}) (string, error) {
	var err error
	if len(args) == 0 {
		return format, nil
	}

	if e, ok := args[len(args)-1].(error); ok {
		err = e
		formatArgs := args
		if countFormatVerbs(format) < len(args) {
			formatArgs = args[:len(args)-1]
		}
		if len(formatArgs) == 0 {
			return format, err
		}
		return sprintf(format, formatArgs...), err
	}

	return sprintf(format, args...), nil
}

func sprintf(format string, args ...interface{}) string {
	sprintfFunc := fmt.Sprintf
	return sprintfFunc(format, args...)
}

func countFormatVerbs(format string) int {
	matches := formatVerbPattern.FindAllString(format, -1)
	count := 0
	for _, match := range matches {
		if strings.HasSuffix(match, "%") {
			continue
		}
		count++
	}
	return count
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level Level) {
	l.level.Store(int32(level))
}

// GetLevel 获取日志级别
func (l *Logger) GetLevel() Level {
	return Level(l.level.Load())
}

func (l *Logger) isEnabled(level Level) bool {
	return level >= Level(l.level.Load())
}

// Close 关闭日志记录器
func (l *Logger) Close() error {
	l.mu.Lock()
	if l.writer.closed {
		l.mu.Unlock()
		return nil
	}
	l.writer.closed = true
	if l.writer.queue != nil {
		close(l.writer.queue)
	}
	output := l.output
	l.mu.Unlock()

	l.writer.worker.Wait()
	if output != nil && output != os.Stdout && output != os.Stderr {
		err := output.Close()
		if errors.Is(err, os.ErrClosed) {
			return nil
		}
		return err
	}
	return nil
}

// Flush waits until all previously queued asynchronous logs are written.
func (l *Logger) Flush() error {
	l.mu.RLock()
	if l.writer.closed || !l.writer.async {
		l.mu.RUnlock()
		return nil
	}
	done := make(chan struct{})
	l.writer.queue <- logItem{flush: done}
	l.mu.RUnlock()
	<-done
	return nil
}

func (l *Logger) write(item logItem) {
	if l.writer.async {
		l.writer.queue <- item
		return
	}
	l.writeItem(item)
}

func (l *Logger) runWriter() {
	defer l.writer.worker.Done()
	for item := range l.writer.queue {
		if item.flush != nil {
			close(item.flush)
			continue
		}
		l.writeItem(item)
	}
}

func (l *Logger) writeItem(item logItem) {
	if item.entry != nil {
		data, err := json.Marshal(item.entry)
		if err != nil {
			item.data = []byte(fmt.Sprintf("[%s] %s: %s\n", item.entry.Level, time.Now().Format(time.RFC3339), item.entry.Message))
		} else {
			item.data = append(data, '\n')
		}
	}
	_, _ = l.output.Write(item.data)
}

// getProjectRoot 获取项目根目录
// 通过查找 go.mod 文件来确定项目根目录
func getProjectRoot() string {
	projectRootOnce.Do(func() {
		// 获取当前工作目录
		wd, err := os.Getwd()
		if err != nil {
			return
		}

		// 从当前目录向上查找 go.mod 文件
		dir := wd
		for {
			goModPath := filepath.Join(dir, "go.mod")
			if _, err := os.Stat(goModPath); err == nil {
				projectRoot = dir
				return
			}

			// 向上查找父目录
			parent := filepath.Dir(dir)
			if parent == dir {
				projectRoot = wd
				return
			}
			dir = parent
		}
	})
	return projectRoot
}

// getPackageName reduces a fully qualified function name to the package name
func getPackageName(f string) string {
	for {
		lastPeriod := strings.LastIndex(f, ".")
		lastSlash := strings.LastIndex(f, "/")
		if lastPeriod > lastSlash {
			f = f[:lastPeriod]
		} else {
			break
		}
	}
	return f
}

// getCaller retrieves the name of the first non-framework calling function
func getCaller() (uintptr, string, int) {
	// cache this package's fully-qualified name
	callerInitOnce.Do(func() {
		var pcs [maximumCallDepth]uintptr
		depth := runtime.Callers(0, pcs[:])

		// dynamic get the package name and the minimum caller depth
		for _, pc := range pcs[:depth] {
			fn := runtime.FuncForPC(pc)
			if fn == nil {
				continue
			}
			funcName := fn.Name()
			if strings.Contains(funcName, "getCaller") {
				loggerPackage = getPackageName(funcName)
				break
			}
		}
	})

	// Restrict the lookback frames to avoid runaway lookups
	var pcs [maximumCallDepth]uintptr
	depth := runtime.Callers(knownLoggerFrames, pcs[:])
	frames := runtime.CallersFrames(pcs[:depth])

	for f, again := frames.Next(); again; f, again = frames.Next() {
		pkg := getPackageName(f.Function)

		if strings.HasSuffix(f.File, "_test.go") {
			return f.PC, f.File, f.Line
		}

		// Check if this is the logger package
		if pkg == loggerPackage {
			continue
		}

		// Check if this is a framework package
		isFramework := false
		for _, frameworkPkg := range frameworkPackages {
			if strings.Contains(pkg, frameworkPkg) || strings.Contains(f.File, frameworkPkg) {
				isFramework = true
				break
			}
		}

		// If the caller isn't part of framework packages, we're done
		if !isFramework {
			return f.PC, f.File, f.Line
		}
	}

	// if we got here, we failed to find the caller's context
	return 0, "", 0
}
