package grpc

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"

	"github.com/gly-hub/quickgo/logger"
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

const (
	serviceWeightAttributeKey = "quickgo.service.weight"
	maxServiceWeight          = 1000
)

// weightedRoundRobinBuilder 加权轮询构建器。
type weightedRoundRobinBuilder struct{}

// Build 构建负载均衡器
func (b *weightedRoundRobinBuilder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	return base.NewBalancerBuilder(WeightedRoundRobinBalancer, &weightedRoundRobinPickerBuilder{}, base.Config{
		HealthCheck: true,
	}).Build(cc, opts)
}

// Name 返回名称
func (b *weightedRoundRobinBuilder) Name() string {
	return WeightedRoundRobinBalancer
}

// weightedRoundRobinPickerBuilder constructs a smooth weighted round-robin picker.
type weightedRoundRobinPickerBuilder struct{}

// Build 构建选择器
func (b *weightedRoundRobinPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}

	subConns := make([]weightedSubConn, 0, len(info.ReadySCs))
	for sc, subConnInfo := range info.ReadySCs {
		subConns = append(subConns, weightedSubConn{
			subConn: sc,
			weight:  weightFromAddress(subConnInfo.Address),
			address: subConnInfo.Address.Addr,
		})
	}
	// Map iteration is intentionally random; stabilize it so equal-weight
	// instances have predictable scheduling across picker rebuilds.
	sort.Slice(subConns, func(i, j int) bool { return subConns[i].address < subConns[j].address })

	return &weightedRoundRobinPicker{subConns: subConns}
}

func weightFromAddress(address resolver.Address) int {
	if address.Attributes == nil {
		return 1
	}
	weight, ok := address.Attributes.Value(serviceWeightAttributeKey).(int)
	if !ok || weight <= 0 {
		return 1
	}
	if weight > maxServiceWeight {
		return maxServiceWeight
	}
	return weight
}

type weightedSubConn struct {
	subConn       balancer.SubConn
	address       string
	weight        int
	currentWeight int
}

// weightedRoundRobinPicker uses the smooth weighted round-robin algorithm.
// It does not expand an instance into N entries, so untrusted weight values do
// not create an unbounded picker allocation.
type weightedRoundRobinPicker struct {
	subConns []weightedSubConn
	mu       sync.Mutex
}

// Pick 选择连接
func (p *weightedRoundRobinPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	selected := p.pickNext()
	if selected == nil {
		return balancer.PickResult{}, fmt.Errorf("no subconnections available")
	}

	return balancer.PickResult{
		SubConn: selected.subConn,
	}, nil
}

func (p *weightedRoundRobinPicker) pickNext() *weightedSubConn {
	if len(p.subConns) == 0 {
		return nil
	}

	var selected *weightedSubConn
	totalWeight := 0
	for i := range p.subConns {
		candidate := &p.subConns[i]
		candidate.currentWeight += candidate.weight
		totalWeight += candidate.weight
		if selected == nil || candidate.currentWeight > selected.currentWeight {
			selected = candidate
		}
	}
	selected.currentWeight -= totalWeight
	return selected
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
