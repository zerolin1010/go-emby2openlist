package node

import (
	"math/rand"
	"sync/atomic"
	"time"
)

// Selector 节点选择器
type Selector struct {
	checker *HealthChecker
	counter uint64 // 用于轮询
	rng     *rand.Rand
}

// NewSelector 创建选择器
func NewSelector(checker *HealthChecker) *Selector {
	return &Selector{
		checker: checker,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SelectNode 选择最优节点
// 策略: 加权随机
func (s *Selector) SelectNode() *NodeStatus {
	nodes := s.checker.GetHealthyNodes()
	if len(nodes) == 0 {
		return nil
	}

	// 计算总权重
	totalWeight := 0
	for _, node := range nodes {
		totalWeight += node.GetWeight()
	}

	if totalWeight == 0 {
		// 所有权重为0，使用轮询
		return s.SelectNodeRoundRobin()
	}

	// 加权随机选择
	r := s.rng.Intn(totalWeight)
	for _, node := range nodes {
		r -= node.GetWeight()
		if r < 0 {
			return node
		}
	}

	return nodes[0]
}

// SelectNodeRoundRobin 轮询选择节点
func (s *Selector) SelectNodeRoundRobin() *NodeStatus {
	nodes := s.checker.GetHealthyNodes()
	if len(nodes) == 0 {
		return nil
	}

	idx := atomic.AddUint64(&s.counter, 1) % uint64(len(nodes))
	return nodes[idx]
}
