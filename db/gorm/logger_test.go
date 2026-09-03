package gorm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	frameworkLogger "github.com/gly-hub/quickgo/logger"
	"gorm.io/gorm/logger"
)

func TestTraceCallerResolvesPastQuickGoAdapter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gorm.log")
	if err := frameworkLogger.Init(frameworkLogger.Config{
		Level:        frameworkLogger.LevelInfo,
		Output:       path,
		EnableCaller: true,
	}); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	defer frameworkLogger.Close()

	adapter := newLogger(&GormConfig{EnableLog: true, LogLevel: "info"})
	adapter.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT 1", 1
	}, nil)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var entry frameworkLogger.LogEntry
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("decode log: %v; content=%s", err, content)
	}
	caller := filepath.ToSlash(entry.Caller)
	if !strings.Contains(caller, "db/gorm/logger_test.go") {
		t.Fatalf("caller = %q, want test call site", entry.Caller)
	}
	if strings.Contains(caller, "db/gorm/logger.go") {
		t.Fatalf("caller incorrectly points at adapter: %q", entry.Caller)
	}
}

var _ logger.Interface = (*gormLogger)(nil)
