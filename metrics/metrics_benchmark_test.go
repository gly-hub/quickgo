package metrics

import (
	"testing"
	"time"
)

func BenchmarkRecordHTTPRequest(b *testing.B) {
	m := New(Config{Namespace: "benchmark"})
	m.RecordHTTPRequest("GET", "/users/:id", "200", time.Millisecond)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordHTTPRequest("GET", "/users/:id", "200", time.Millisecond)
	}
}

func BenchmarkRecordHTTPRequestParallel(b *testing.B) {
	m := New(Config{Namespace: "benchmark_parallel"})
	m.RecordHTTPRequest("GET", "/users/:id", "200", time.Millisecond)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.RecordHTTPRequest("GET", "/users/:id", "200", time.Millisecond)
		}
	})
}

func BenchmarkRecordGRPCRequest(b *testing.B) {
	m := New(Config{Namespace: "grpc_benchmark"})
	m.RecordGRPCRequest("/bench.Ping", "OK", time.Millisecond)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecordGRPCRequest("/bench.Ping", "OK", time.Millisecond)
	}
}
