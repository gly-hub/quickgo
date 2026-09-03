package gerr

import (
	"errors"
	"testing"
)

func BenchmarkNewGErr(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewGErr(400, "benchmark error")
	}
}

func BenchmarkParse(b *testing.B) {
	err := errors.New("benchmark error")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Parse(err)
	}
}

func BenchmarkWithMetadata(b *testing.B) {
	err := NewGErr(400, "benchmark error")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = err.WithMetadata("request", "benchmark")
	}
}
