package quickgo

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestFrameworkLoggerFileOutput(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "quickgo_framework_logger_*.log")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	f, err := NewFramework(ConfigOptionWithLogger(LoggerConfig{
		Enabled: true,
		Level:   "info",
		Output:  "file",
		File:    tmpFile.Name(),
		Service: "test-service",
		Version: "1.0.0",
	}))
	if err != nil {
		t.Fatalf("NewFramework failed: %v", err)
	}
	if err := f.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer f.Logger().Close()

	f.Logger().Info(context.Background(), "framework file log")

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(content), "framework file log") {
		t.Fatalf("expected file log content, got %s", string(content))
	}
}
