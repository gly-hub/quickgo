package json

import (
	stdjson "encoding/json"
	"testing"
)

type benchmarkPayload struct {
	ID      int               `json:"id"`
	Name    string            `json:"name"`
	Tags    []string          `json:"tags"`
	Attrs   map[string]string `json:"attrs"`
	Enabled bool              `json:"enabled"`
}

var benchmarkJSONPayload = benchmarkPayload{
	ID:      42,
	Name:    "quickgo",
	Tags:    []string{"http", "grpc", "metrics", "trace"},
	Attrs:   map[string]string{"region": "local", "tier": "test"},
	Enabled: true,
}

var benchmarkJSONData = []byte(`{"id":42,"name":"quickgo","tags":["http","grpc","metrics","trace"],"attrs":{"region":"local","tier":"test"},"enabled":true}`)

func BenchmarkMarshal(b *testing.B) {
	b.Run("compatible", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data, err := JSON.Marshal(benchmarkJSONPayload)
			if err != nil {
				b.Fatal(err)
			}
			_ = data
		}
	})
	b.Run("fast", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data, err := JSONFast.Marshal(benchmarkJSONPayload)
			if err != nil {
				b.Fatal(err)
			}
			_ = data
		}
	})
	b.Run("stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data, err := stdjson.Marshal(benchmarkJSONPayload)
			if err != nil {
				b.Fatal(err)
			}
			_ = data
		}
	})
}

func BenchmarkUnmarshal(b *testing.B) {
	b.Run("compatible", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var payload benchmarkPayload
			if err := JSON.Unmarshal(benchmarkJSONData, &payload); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("fast", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var payload benchmarkPayload
			if err := JSONFast.Unmarshal(benchmarkJSONData, &payload); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var payload benchmarkPayload
			if err := stdjson.Unmarshal(benchmarkJSONData, &payload); err != nil {
				b.Fatal(err)
			}
		}
	})
}
