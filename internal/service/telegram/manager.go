package telegram

import (
	"fmt"
	"sync"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/node"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"
)

// NodeManager 节点管理器
type NodeManager struct {
	healthChecker *node.HealthChecker
	mu            sync.RWMutex
}

// NewNodeManager 创建节点管理器
func NewNodeManager(healthChecker *node.HealthChecker) *NodeManager {
	return &NodeManager{
		healthChecker: healthChecker,
	}
}

// ListNodes 列出所有节点
func (nm *NodeManager) ListNodes() []config.Node {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	nodes := make([]config.Node, len(config.C.Nodes.List))
	copy(nodes, config.C.Nodes.List)
	return nodes
}

// AddNode 添加节点
func (nm *NodeManager) AddNode(newNode config.Node) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// 检查节点是否已存在
	for _, node := range config.C.Nodes.List {
		if node.Name == newNode.Name {
			return fmt.Errorf("节点 %s 已存在", newNode.Name)
		}
	}

	// 添加到配置
	config.C.Nodes.List = append(config.C.Nodes.List, newNode)

	// 通知健康检查器重新加载节点
	nm.healthChecker.ReloadNodes()

	logs.Info("[Telegram] 添加节点: %s (%s)", newNode.Name, newNode.Host)
	return nil
}

// DeleteNode 删除节点
func (nm *NodeManager) DeleteNode(name string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// 查找并删除节点
	found := false
	newList := make([]config.Node, 0, len(config.C.Nodes.List))

	for _, node := range config.C.Nodes.List {
		if node.Name == name {
			found = true
			continue
		}
		newList = append(newList, node)
	}

	if !found {
		return fmt.Errorf("节点 %s 不存在", name)
	}

	config.C.Nodes.List = newList

	// 通知健康检查器重新加载节点
	nm.healthChecker.ReloadNodes()

	logs.Info("[Telegram] 删除节点: %s", name)
	return nil
}

// EnableNode 启用/禁用节点
func (nm *NodeManager) EnableNode(name string, enable bool) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// 查找节点
	found := false
	for i := range config.C.Nodes.List {
		if config.C.Nodes.List[i].Name == name {
			config.C.Nodes.List[i].Enabled = enable
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("节点 %s 不存在", name)
	}

	// 通知健康检查器重新加载节点
	nm.healthChecker.ReloadNodes()

	status := "禁用"
	if enable {
		status = "启用"
	}
	logs.Info("[Telegram] %s节点: %s", status, name)

	return nil
}
