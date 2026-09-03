package grpc

import (
	"testing"

	"google.golang.org/grpc/balancer"
)

func BenchmarkWeightedRoundRobinPick(b *testing.B) {
	picker := &weightedRoundRobinPicker{subConns: make([]weightedSubConn, 16)}
	for i := range picker.subConns {
		picker.subConns[i].address = "backend-" + string(rune('a'+i))
		picker.subConns[i].weight = (i % 4) + 1
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := picker.Pick(balancer.PickInfo{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkStaticResolverResolve(b *testing.B) {
	resolver := NewStaticResolver([]string{"127.0.0.1:50051", "127.0.0.1:50052", "127.0.0.1:50053"})
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := resolver.Resolve(nil, "bench"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
