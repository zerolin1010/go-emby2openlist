package telegram

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

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

// AddNode 添加节点（支持自动命名）
func (nm *NodeManager) AddNode(newNode config.Node) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// 如果没有提供名称，自动生成
	if newNode.Name == "" {
		newNode.Name = nm.generateNodeName(newNode.Host)
	}

	// 检查节点是否已存在
	for _, node := range config.C.Nodes.List {
		if node.Name == newNode.Name {
			return fmt.Errorf("节点 %s 已存在", newNode.Name)
		}
	}

	// 添加到配置
	config.C.Nodes.List = append(config.C.Nodes.List, newNode)

	// 保存配置到文件
	if err := config.SaveToFile(); err != nil {
		// 回滚配置
		config.C.Nodes.List = config.C.Nodes.List[:len(config.C.Nodes.List)-1]
		return fmt.Errorf("保存配置失败: %v", err)
	}

	// 通知健康检查器重新加载节点
	nm.healthChecker.ReloadNodes()

	logs.Info("[Telegram] 添加节点: %s (%s)", newNode.Name, newNode.Host)
	return nil
}

// generateNodeName 自动生成节点名称
// 格式: node-{host简写}-{序号}
// 例如: node-192168-1, node-example-1
func (nm *NodeManager) generateNodeName(host string) string {
	// 提取主机名或IP的简短标识
	shortID := nm.extractHostID(host)

	// 查找可用的序号
	for i := 1; i <= 999; i++ {
		name := fmt.Sprintf("node-%s-%d", shortID, i)
		// 检查是否已存在
		exists := false
		for _, node := range config.C.Nodes.List {
			if node.Name == name {
				exists = true
				break
			}
		}
		if !exists {
			return name
		}
	}

	// 如果前999个都被占用，使用时间戳+哈希
	hash := md5.Sum([]byte(host + strconv.FormatInt(time.Now().UnixNano(), 10)))
	return "node-" + hex.EncodeToString(hash[:])[:8]
}

// extractHostID 从 host URL 中提取简短标识
func (nm *NodeManager) extractHostID(host string) string {
	u, err := url.Parse(host)
	if err != nil {
		// 解析失败，使用MD5哈希的前6位
		hash := md5.Sum([]byte(host))
		return hex.EncodeToString(hash[:])[:6]
	}

	hostname := u.Hostname()
	if hostname == "" {
		hash := md5.Sum([]byte(host))
		return hex.EncodeToString(hash[:])[:6]
	}

	// 如果是IP地址，去掉点号
	// 例如: 192.168.1.1 -> 192168
	if len(hostname) > 0 && (hostname[0] >= '0' && hostname[0] <= '9') {
		// 可能是IP，取前两段
		parts := []byte(hostname)
		result := make([]byte, 0, 10)
		dotCount := 0
		for _, b := range parts {
			if b == '.' {
				dotCount++
				if dotCount >= 2 {
					break
				}
				continue
			}
			result = append(result, b)
		}
		if len(result) > 0 {
			return string(result)
		}
	}

	// 如果是域名，取主域名部分
	// 例如: cdn.example.com -> example
	parts := []byte(hostname)
	lastDot := -1
	secondLastDot := -1
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == '.' {
			if lastDot == -1 {
				lastDot = i
			} else if secondLastDot == -1 {
				secondLastDot = i
				break
			}
		}
	}

	if secondLastDot != -1 && lastDot != -1 {
		return string(parts[secondLastDot+1 : lastDot])
	} else if lastDot != -1 {
		return string(parts[:lastDot])
	}

	// 降级：使用哈希
	hash := md5.Sum([]byte(hostname))
	return hex.EncodeToString(hash[:])[:6]
}

// DeleteNode 删除节点
func (nm *NodeManager) DeleteNode(name string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// 查找并删除节点
	found := false
	oldList := config.C.Nodes.List
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

	// 保存配置到文件
	if err := config.SaveToFile(); err != nil {
		// 回滚配置
		config.C.Nodes.List = oldList
		return fmt.Errorf("保存配置失败: %v", err)
	}

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

	// 保存配置到文件
	if err := config.SaveToFile(); err != nil {
		return fmt.Errorf("保存配置失败: %v", err)
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

// BatchAddNodes 批量添加节点
// hosts: 节点主机列表（可选包含权重，格式：host 或 host:weight）
// 返回：成功数量、失败的节点列表（主机名）、错误
func (nm *NodeManager) BatchAddNodes(hosts []string) (int, []string, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	successCount := 0
	failedHosts := make([]string, 0)

	// 收集所有新节点
	nodesToAdd := make([]config.Node, 0, len(hosts))

	for _, hostStr := range hosts {
		// 解析主机和权重
		host, weight := nm.parseHostWeight(hostStr)

		// 生成节点名称
		name := nm.generateNodeName(host)

		// 检查节点是否已存在
		exists := false
		for _, node := range config.C.Nodes.List {
			if node.Name == name || node.Host == host {
				exists = true
				break
			}
		}

		if exists {
			logs.Warn("[Telegram] 节点 %s 已存在，跳过", host)
			failedHosts = append(failedHosts, host)
			continue
		}

		nodesToAdd = append(nodesToAdd, config.Node{
			Name:    name,
			Host:    host,
			Weight:  weight,
			Enabled: true,
		})
	}

	if len(nodesToAdd) == 0 {
		return 0, failedHosts, fmt.Errorf("没有可添加的新节点")
	}

	// 批量添加到配置
	oldList := config.C.Nodes.List
	config.C.Nodes.List = append(config.C.Nodes.List, nodesToAdd...)

	// 保存配置到文件
	if err := config.SaveToFile(); err != nil {
		// 回滚配置
		config.C.Nodes.List = oldList
		return 0, failedHosts, fmt.Errorf("保存配置失败: %v", err)
	}

	// 通知健康检查器重新加载节点
	nm.healthChecker.ReloadNodes()

	successCount = len(nodesToAdd)
	logs.Info("[Telegram] 批量添加 %d 个节点成功", successCount)

	return successCount, failedHosts, nil
}

// BatchDeleteNodes 批量删除节点
// names: 节点名称列表
// 返回：成功数量、失败的节点列表、错误
func (nm *NodeManager) BatchDeleteNodes(names []string) (int, []string, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	oldList := config.C.Nodes.List
	newList := make([]config.Node, 0, len(config.C.Nodes.List))
	deletedCount := 0
	failedNames := make([]string, 0)

	// 创建删除集合
	toDelete := make(map[string]bool)
	for _, name := range names {
		toDelete[name] = true
	}

	// 过滤要删除的节点
	for _, node := range config.C.Nodes.List {
		if toDelete[node.Name] {
			deletedCount++
			delete(toDelete, node.Name)
		} else {
			newList = append(newList, node)
		}
	}

	// 记录不存在的节点
	for name := range toDelete {
		failedNames = append(failedNames, name)
	}

	if deletedCount == 0 {
		return 0, failedNames, fmt.Errorf("没有删除任何节点")
	}

	config.C.Nodes.List = newList

	// 保存配置到文件
	if err := config.SaveToFile(); err != nil {
		// 回滚配置
		config.C.Nodes.List = oldList
		return 0, failedNames, fmt.Errorf("保存配置失败: %v", err)
	}

	// 通知健康检查器重新加载节点
	nm.healthChecker.ReloadNodes()

	logs.Info("[Telegram] 批量删除 %d 个节点成功", deletedCount)

	return deletedCount, failedNames, nil
}

// parseHostWeight 解析主机和权重
// 格式：host 或 host:weight
// 例如：http://1.2.3.4:80 或 http://1.2.3.4:80:100
func (nm *NodeManager) parseHostWeight(hostStr string) (string, int) {
	// 默认权重
	weight := 100

	// 尝试从最后一个冒号分割权重
	// 但要注意 http://host:port 的情况
	lastColon := -1
	colonCount := 0
	for i := len(hostStr) - 1; i >= 0; i-- {
		if hostStr[i] == ':' {
			colonCount++
			if colonCount == 1 {
				lastColon = i
			}
			// http://host:port 格式最多2个冒号
			// http://host:port:weight 格式最多3个冒号
			if colonCount >= 3 {
				break
			}
		}
	}

	// 如果有3个冒号，最后一个可能是权重
	if colonCount >= 3 && lastColon > 0 {
		weightStr := hostStr[lastColon+1:]
		if w, err := strconv.Atoi(weightStr); err == nil && w >= 1 && w <= 100 {
			weight = w
			hostStr = hostStr[:lastColon]
		}
	}

	return hostStr, weight
}
