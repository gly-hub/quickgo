package grpc

import (
	"testing"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

func TestWeightFromAddress(t *testing.T) {
	if got := weightFromAddress(resolver.Address{}); got != 1 {
		t.Fatalf("weight without metadata = %d, want 1", got)
	}
	if got := weightFromAddress(resolver.Address{Attributes: attributes.New(serviceWeightAttributeKey, 5)}); got != 5 {
		t.Fatalf("weight with metadata = %d, want 5", got)
	}
	if got := weightFromAddress(resolver.Address{Attributes: attributes.New(serviceWeightAttributeKey, 0)}); got != 1 {
		t.Fatalf("invalid weight = %d, want 1", got)
	}
	if got := weightFromAddress(resolver.Address{Attributes: attributes.New(serviceWeightAttributeKey, maxServiceWeight+1)}); got != maxServiceWeight {
		t.Fatalf("oversized weight = %d, want %d", got, maxServiceWeight)
	}
}

func TestWeightedRoundRobinPickerHonorsInstanceWeights(t *testing.T) {
	picker := &weightedRoundRobinPicker{subConns: []weightedSubConn{
		{address: "heavy", weight: 3},
		{address: "light", weight: 1},
	}}

	counts := map[string]int{}
	for range 8 {
		counts[picker.pickNext().address]++
	}
	if got, want := counts["heavy"], 6; got != want {
		t.Fatalf("heavy instance selections = %d, want %d", got, want)
	}
	if got, want := counts["light"], 2; got != want {
		t.Fatalf("light instance selections = %d, want %d", got, want)
	}
}
