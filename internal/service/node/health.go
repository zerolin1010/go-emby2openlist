package node

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"
)

// HealthChecker 健康检查器
type HealthChecker struct {
	nodes    map[string]*NodeStatus
	client   *http.Client
	interval time.Duration
	timeout  time.Duration
	failTh   int // 失败阈值
	succTh   int // 成功阈值
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(cfg *config.Nodes) *HealthChecker {
	hc := &HealthChecker{
		nodes: make(map[string]*NodeStatus),
		client: &http.Client{
			Timeout: time.Duration(cfg.HealthCheck.Timeout) * time.Second,
			// 不跟随重定向
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		interval: time.Duration(cfg.HealthCheck.Interval) * time.Second,
		timeout:  time.Duration(cfg.HealthCheck.Timeout) * time.Second,
		failTh:   cfg.HealthCheck.FailThreshold,
		succTh:   cfg.HealthCheck.SuccessThreshold,
		stopCh:   make(chan struct{}),
	}

	// 初始化节点状态
	for _, node := range cfg.List {
		if !node.Enabled {
			continue
		}
		hc.nodes[node.Name] = &NodeStatus{
			Name:    node.Name,
			Host:    node.Host,
			Weight:  node.Weight,
			Enabled: node.Enabled,
			Healthy: true, // 初始假定健康
		}
	}

	return hc
}

// Start 启动健康检查
func (hc *HealthChecker) Start() {
	// 立即执行一次检查
	hc.checkAll()

	// 定时检查
	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hc.checkAll()
		case <-hc.stopCh:
			return
		}
	}
}

// Stop 停止健康检查
func (hc *HealthChecker) Stop() {
	close(hc.stopCh)
}

// checkAll 检查所有节点
func (hc *HealthChecker) checkAll() {
	var wg sync.WaitGroup

	hc.mu.RLock()
	nodes := make([]*NodeStatus, 0, len(hc.nodes))
	for _, node := range hc.nodes {
		nodes = append(nodes, node)
	}
	hc.mu.RUnlock()

	for _, node := range nodes {
		wg.Add(1)
		go func(n *NodeStatus) {
			defer wg.Done()
			hc.checkNode(n)
		}(node)
	}

	wg.Wait()
}

// checkNode 检查单个节点
func (hc *HealthChecker) checkNode(node *NodeStatus) {
	ctx, cancel := context.WithTimeout(context.Background(), hc.timeout)
	defer cancel()

	// 构建健康检查 URL
	// 健康检查固定使用 80 端口（从 node.Host 提取 IP/域名）
	healthCheckURL := buildHealthCheckURL(node.Host)

	// 构建健康检查请求
	// curl -v -H "Host: gtm-health" http://<IP>:80/gtm-health
	req, err := http.NewRequestWithContext(ctx, "GET", healthCheckURL, nil)
	if err != nil {
		hc.markUnhealthy(node)
		return
	}
	req.Host = "gtm-health"

	resp, err := hc.client.Do(req)
	if err != nil {
		logs.Warn("节点 %s 健康检查失败: %v", node.Name, err)
		hc.markUnhealthy(node)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		hc.markHealthy(node)
	} else {
		logs.Warn("节点 %s 健康检查返回非200: %d", node.Name, resp.StatusCode)
		hc.markUnhealthy(node)
	}
}

// markHealthy 标记节点健康
func (hc *HealthChecker) markHealthy(node *NodeStatus) {
	node.mu.Lock()
	defer node.mu.Unlock()

	node.LastCheck = time.Now()
	node.ConsecutiveFails = 0
	node.ConsecutiveSucc++

	if !node.Healthy && node.ConsecutiveSucc >= hc.succTh {
		node.Healthy = true
		logs.Success("节点 %s 恢复健康", node.Name)
	}
}

// markUnhealthy 标记节点不健康
func (hc *HealthChecker) markUnhealthy(node *NodeStatus) {
	node.mu.Lock()
	defer node.mu.Unlock()

	node.LastCheck = time.Now()
	node.ConsecutiveSucc = 0
	node.ConsecutiveFails++

	if node.Healthy && node.ConsecutiveFails >= hc.failTh {
		node.Healthy = false
		logs.Error("节点 %s 标记为不健康", node.Name)
	}
}

// buildHealthCheckURL 构建健康检查 URL
// 从节点 Host 提取 scheme 和 hostname，固定使用 80 端口
// 例如: http://1.2.3.4:46621 -> http://1.2.3.4:80/gtm-health
func buildHealthCheckURL(nodeHost string) string {
	u, err := url.Parse(nodeHost)
	if err != nil {
		// 解析失败，直接拼接（兼容旧逻辑）
		return nodeHost + "/gtm-health"
	}

	// 提取 scheme (http/https)
	scheme := u.Scheme
	if scheme == "" {
		scheme = "http"
	}

	// 提取 hostname (IP 或域名)
	hostname := u.Hostname()
	if hostname == "" {
		// 如果无法提取，使用原 Host（兼容）
		return nodeHost + "/gtm-health"
	}

	// 固定使用 80 端口进行健康检查
	return scheme + "://" + hostname + ":80/gtm-health"
}

// GetHealthyNodes 获取所有健康节点
func (hc *HealthChecker) GetHealthyNodes() []*NodeStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	healthy := make([]*NodeStatus, 0)
	for _, node := range hc.nodes {
		node.mu.RLock()
		if node.Healthy {
			healthy = append(healthy, node)
		}
		node.mu.RUnlock()
	}
	return healthy
}

// GetAllNodes 获取所有节点（包含健康状态）
func (hc *HealthChecker) GetAllNodes() []*NodeStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	all := make([]*NodeStatus, 0, len(hc.nodes))
	for _, node := range hc.nodes {
		all = append(all, node)
	}
	return all
}

// ReloadNodes 重新加载节点配置
func (hc *HealthChecker) ReloadNodes() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	// 清空现有节点
	hc.nodes = make(map[string]*NodeStatus)

	// 从配置中重新加载
	for _, node := range config.C.Nodes.List {
		if !node.Enabled {
			continue
		}
		hc.nodes[node.Name] = &NodeStatus{
			Name:    node.Name,
			Host:    node.Host,
			Weight:  node.Weight,
			Enabled: node.Enabled,
			Healthy: true, // 初始假定健康
		}
	}

	logs.Info("节点配置已重新加载，当前节点数: %d", len(hc.nodes))

	// 立即执行一次健康检查
	go hc.checkAll()
}
