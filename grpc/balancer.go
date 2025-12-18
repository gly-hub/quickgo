package grpc

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"

	"github.com/team-dandelion/quickgo/logger"
)

const (
	// RoundRobinBalancer 轮询负载均衡器
	RoundRobinBalancer = "round_robin"
	// PickFirstBalancer 选择第一个可用连接
	PickFirstBalancer = "pick_first"
	// WeightedRoundRobinBalancer 加权轮询负载均衡器
	WeightedRoundRobinBalancer = "weighted_round_robin"
)

// LoadBalancingPolicy 负载均衡策略
type LoadBalancingPolicy string

const (
	// PolicyRoundRobin 轮询策略
	PolicyRoundRobin LoadBalancingPolicy = RoundRobinBalancer
	// PolicyPickFirst 选择第一个策略
	PolicyPickFirst LoadBalancingPolicy = PickFirstBalancer
	// PolicyWeightedRoundRobin 加权轮询策略
	PolicyWeightedRoundRobin LoadBalancingPolicy = WeightedRoundRobinBalancer
)

// WeightedAddress 加权地址
type WeightedAddress struct {
	Address string
	Weight  int // 权重，默认为 1
}

// weightedRoundRobinBuilder 加权轮询构建器（简化实现，使用轮询策略）
type weightedRoundRobinBuilder struct{}

// Build 构建负载均衡器
func (b *weightedRoundRobinBuilder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	// 使用 base 包构建轮询负载均衡器
	return base.NewBalancerBuilder(WeightedRoundRobinBalancer, &roundRobinPickerBuilder{}, base.Config{
		HealthCheck: true,
	}).Build(cc, opts)
}

// Name 返回名称
func (b *weightedRoundRobinBuilder) Name() string {
	return WeightedRoundRobinBalancer
}

// roundRobinPickerBuilder 轮询选择器构建器
type roundRobinPickerBuilder struct{}

// Build 构建选择器
func (b *roundRobinPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	// 构建轮询选择器
	scs := make([]balancer.SubConn, 0, len(info.ReadySCs))
	for sc := range info.ReadySCs {
		scs = append(scs, sc)
	}

	// 使用简单的轮询选择器
	return &roundRobinPicker{
		subConns: scs,
		next:     0,
		mu:       sync.Mutex{},
	}
}

// roundRobinPicker 轮询选择器
type roundRobinPicker struct {
	subConns []balancer.SubConn
	next     int
	mu       sync.Mutex
}

// Pick 选择连接
func (p *roundRobinPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.subConns) == 0 {
		return balancer.PickResult{}, fmt.Errorf("no subconnections available")
	}

	sc := p.subConns[p.next]
	p.next = (p.next + 1) % len(p.subConns)

	return balancer.PickResult{
		SubConn: sc,
	}, nil
}

// RegisterWeightedRoundRobinBalancer 注册加权轮询负载均衡器
func RegisterWeightedRoundRobinBalancer() {
	balancer.Register(&weightedRoundRobinBuilder{})
	logger.Info(context.Background(), "Weighted round robin balancer registered")
}

// GetLoadBalancingOption 获取负载均衡选项
func GetLoadBalancingOption(policy LoadBalancingPolicy) grpc.DialOption {
	switch policy {
	case PolicyRoundRobin:
		return grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingPolicy":"%s"}`, RoundRobinBalancer))
	case PolicyPickFirst:
		return grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingPolicy":"%s"}`, PickFirstBalancer))
	case PolicyWeightedRoundRobin:
		RegisterWeightedRoundRobinBalancer()
		return grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingPolicy":"%s"}`, WeightedRoundRobinBalancer))
	default:
		return grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingPolicy":"%s"}`, RoundRobinBalancer))
	}
}
