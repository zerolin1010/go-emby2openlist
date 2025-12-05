package node

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
)

func TestHealthChecker_Basic(t *testing.T) {
	// 创建模拟健康检查服务器
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 Host 头
		if r.Host != "gtm-health" {
			t.Errorf("期望 Host=gtm-health, 实际 Host=%s", r.Host)
		}

		// 验证路径
		if r.URL.Path != "/gtm-health" {
			t.Errorf("期望路径 /gtm-health, 实际 %s", r.URL.Path)
		}

		// 返回健康状态
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer healthServer.Close()

	// 创建配置
	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         1, // 1秒
			Timeout:          2,
			FailThreshold:    2,
			SuccessThreshold: 1,
		},
		List: []config.Node{
			{
				Name:    "test-node",
				Host:    healthServer.URL,
				Weight:  100,
				Enabled: true,
			},
		},
	}

	// 创建健康检查器
	checker := NewHealthChecker(cfg)

	// 检查初始状态
	allNodes := checker.GetAllNodes()
	if len(allNodes) != 1 {
		t.Fatalf("期望 1 个节点, 实际 %d", len(allNodes))
	}

	// 执行一次健康检查
	checker.checkAll()

	// 等待检查完成
	time.Sleep(100 * time.Millisecond)

	// 验证节点健康
	healthyNodes := checker.GetHealthyNodes()
	if len(healthyNodes) != 1 {
		t.Errorf("期望 1 个健康节点, 实际 %d", len(healthyNodes))
	}

	if healthyNodes[0].Name != "test-node" {
		t.Errorf("期望节点名 test-node, 实际 %s", healthyNodes[0].Name)
	}

	t.Logf("✅ 基础健康检查测试通过")
}

func TestHealthChecker_UnhealthyNode(t *testing.T) {
	// 创建一个总是返回 500 的服务器
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         1,
			Timeout:          2,
			FailThreshold:    2, // 连续失败2次标记为不健康
			SuccessThreshold: 1,
		},
		List: []config.Node{
			{
				Name:    "unhealthy-node",
				Host:    unhealthyServer.URL,
				Weight:  100,
				Enabled: true,
			},
		},
	}

	checker := NewHealthChecker(cfg)

	// 第一次检查
	checker.checkAll()
	time.Sleep(100 * time.Millisecond)

	// 节点应该还是健康的（需要连续失败2次）
	healthy := checker.GetHealthyNodes()
	if len(healthy) != 1 {
		t.Logf("第一次检查后，健康节点数: %d (预期: 1)", len(healthy))
	}

	// 第二次检查
	checker.checkAll()
	time.Sleep(100 * time.Millisecond)

	// 现在应该标记为不健康
	healthy = checker.GetHealthyNodes()
	if len(healthy) != 0 {
		t.Errorf("连续失败2次后，期望 0 个健康节点, 实际 %d", len(healthy))
	}

	all := checker.GetAllNodes()
	if len(all) != 1 {
		t.Errorf("期望 1 个节点总数, 实际 %d", len(all))
	}

	t.Logf("✅ 不健康节点检测测试通过")
}

func TestHealthChecker_NodeRecovery(t *testing.T) {
	// 创建一个可以切换健康状态的服务器
	isHealthy := false

	toggleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isHealthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer toggleServer.Close()

	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         1,
			Timeout:          2,
			FailThreshold:    2,
			SuccessThreshold: 1, // 成功1次即可恢复
		},
		List: []config.Node{
			{
				Name:    "toggle-node",
				Host:    toggleServer.URL,
				Weight:  100,
				Enabled: true,
			},
		},
	}

	checker := NewHealthChecker(cfg)

	// 让节点变为不健康
	checker.checkAll()
	time.Sleep(50 * time.Millisecond)
	checker.checkAll()
	time.Sleep(50 * time.Millisecond)

	if len(checker.GetHealthyNodes()) != 0 {
		t.Error("节点应该已经不健康")
	}

	// 恢复健康状态
	isHealthy = true

	// 执行健康检查
	checker.checkAll()
	time.Sleep(50 * time.Millisecond)

	// 验证节点恢复
	healthy := checker.GetHealthyNodes()
	if len(healthy) != 1 {
		t.Errorf("节点恢复后，期望 1 个健康节点, 实际 %d", len(healthy))
	}

	t.Logf("✅ 节点恢复测试通过")
}

func TestHealthChecker_MultipleNodes(t *testing.T) {
	// 创建多个测试服务器
	healthyServer1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyServer1.Close()

	healthyServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyServer2.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         1,
			Timeout:          2,
			FailThreshold:    1,
			SuccessThreshold: 1,
		},
		List: []config.Node{
			{Name: "node-1", Host: healthyServer1.URL, Weight: 100, Enabled: true},
			{Name: "node-2", Host: healthyServer2.URL, Weight: 80, Enabled: true},
			{Name: "node-3", Host: unhealthyServer.URL, Weight: 60, Enabled: true},
		},
	}

	checker := NewHealthChecker(cfg)

	// 执行健康检查
	checker.checkAll()
	time.Sleep(100 * time.Millisecond)

	// 验证结果
	all := checker.GetAllNodes()
	healthy := checker.GetHealthyNodes()

	if len(all) != 3 {
		t.Errorf("期望 3 个节点总数, 实际 %d", len(all))
	}

	if len(healthy) != 2 {
		t.Errorf("期望 2 个健康节点, 实际 %d", len(healthy))
	}

	// 验证健康节点是正确的
	healthyNames := make(map[string]bool)
	for _, node := range healthy {
		healthyNames[node.Name] = true
	}

	if !healthyNames["node-1"] || !healthyNames["node-2"] {
		t.Error("健康节点应该是 node-1 和 node-2")
	}

	if healthyNames["node-3"] {
		t.Error("node-3 不应该是健康的")
	}

	t.Logf("✅ 多节点健康检查测试通过")
}

func TestHealthChecker_DisabledNode(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthServer.Close()

	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         1,
			Timeout:          2,
			FailThreshold:    2,
			SuccessThreshold: 1,
		},
		List: []config.Node{
			{Name: "enabled-node", Host: healthServer.URL, Weight: 100, Enabled: true},
			{Name: "disabled-node", Host: healthServer.URL, Weight: 100, Enabled: false},
		},
	}

	checker := NewHealthChecker(cfg)

	// 禁用的节点不应该被初始化
	all := checker.GetAllNodes()
	if len(all) != 1 {
		t.Errorf("期望 1 个节点（禁用的不应该被加载）, 实际 %d", len(all))
	}

	if all[0].Name != "enabled-node" {
		t.Errorf("期望节点名 enabled-node, 实际 %s", all[0].Name)
	}

	t.Logf("✅ 禁用节点测试通过")
}
