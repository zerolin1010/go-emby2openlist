package node

import (
	"sync"
	"time"
)

// NodeStatus 节点状态
type NodeStatus struct {
	Name             string
	Host             string
	Weight           int
	Enabled          bool // 是否启用
	Healthy          bool
	LastCheck        time.Time
	ConsecutiveFails int
	ConsecutiveSucc  int
	mu               sync.RWMutex
}

// IsHealthy 线程安全地获取节点健康状态
func (ns *NodeStatus) IsHealthy() bool {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.Healthy
}

// GetWeight 线程安全地获取节点权重
func (ns *NodeStatus) GetWeight() int {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.Weight
}

// GetName 线程安全地获取节点名称
func (ns *NodeStatus) GetName() string {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.Name
}

// GetHost 线程安全地获取节点地址
func (ns *NodeStatus) GetHost() string {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.Host
}

// IsEnabled 线程安全地获取节点启用状态
func (ns *NodeStatus) IsEnabled() bool {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	return ns.Enabled
}
