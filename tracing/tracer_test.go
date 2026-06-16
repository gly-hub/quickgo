package tracing

import (
	"context"
	"testing"
)

func TestSamplingRateZeroMeansDropAll(t *testing.T) {
	if err := Init(&Config{Enabled: true, SamplingRate: 0}); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Shutdown(context.Background())

	_, span := StartSpan(context.Background(), "drop")
	defer span.End()
	if span.IsRecording() {
		t.Fatal("expected samplingRate=0 to create non-recording spans")
	}
}

func TestDefaultConfigSamplesAll(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	if err := Init(&config); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Shutdown(context.Background())

	_, span := StartSpan(context.Background(), "record")
	defer span.End()
	if !span.IsRecording() {
		t.Fatal("expected DefaultConfig to create recording spans")
	}
}
