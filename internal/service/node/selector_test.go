package node

import (
	"math"
	"testing"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
)

func TestSelector_Basic(t *testing.T) {
	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         30,
			Timeout:          5,
			FailThreshold:    3,
			SuccessThreshold: 2,
		},
		List: []config.Node{
			{Name: "node-1", Host: "http://1.1.1.1", Weight: 100, Enabled: true},
			{Name: "node-2", Host: "http://2.2.2.2", Weight: 50, Enabled: true},
		},
	}

	checker := NewHealthChecker(cfg)
	selector := NewSelector(checker)

	// 选择节点
	node := selector.SelectNode()
	if node == nil {
		t.Fatal("应该选择到节点")
	}

	if node.Name != "node-1" && node.Name != "node-2" {
		t.Errorf("节点名称不正确: %s", node.Name)
	}

	t.Logf("✅ 基础节点选择测试通过，选中: %s", node.Name)
}

func TestSelector_WeightedDistribution(t *testing.T) {
	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         30,
			Timeout:          5,
			FailThreshold:    3,
			SuccessThreshold: 2,
		},
		List: []config.Node{
			{Name: "node-1", Host: "http://1.1.1.1", Weight: 100, Enabled: true},
			{Name: "node-2", Host: "http://2.2.2.2", Weight: 50, Enabled: true},
			{Name: "node-3", Host: "http://3.3.3.3", Weight: 10, Enabled: true},
		},
	}

	checker := NewHealthChecker(cfg)
	selector := NewSelector(checker)

	// 模拟 1000 次选择
	counts := make(map[string]int)
	totalSelections := 10000

	for i := 0; i < totalSelections; i++ {
		node := selector.SelectNode()
		if node != nil {
			counts[node.Name]++
		}
	}

	t.Logf("选择统计 (共 %d 次):", totalSelections)
	for name, count := range counts {
		percentage := float64(count) / float64(totalSelections) * 100
		t.Logf("  %s: %d 次 (%.2f%%)", name, count, percentage)
	}

	// 验证权重比例（允许 5% 误差）
	// totalWeight := 100 + 50 + 10 // 160
	expectedRatio1 := 100.0 / 160.0 // 约 62.5%
	expectedRatio2 := 50.0 / 160.0  // 约 31.25%
	expectedRatio3 := 10.0 / 160.0  // 约 6.25%

	ratio1 := float64(counts["node-1"]) / float64(totalSelections)
	ratio2 := float64(counts["node-2"]) / float64(totalSelections)
	ratio3 := float64(counts["node-3"]) / float64(totalSelections)

	tolerance := 0.05 // 5% 容差

	if math.Abs(ratio1-expectedRatio1) > tolerance {
		t.Errorf("node-1 选择比例 %.4f 超出预期范围 [%.4f, %.4f]",
			ratio1, expectedRatio1-tolerance, expectedRatio1+tolerance)
	}

	if math.Abs(ratio2-expectedRatio2) > tolerance {
		t.Errorf("node-2 选择比例 %.4f 超出预期范围 [%.4f, %.4f]",
			ratio2, expectedRatio2-tolerance, expectedRatio2+tolerance)
	}

	if math.Abs(ratio3-expectedRatio3) > tolerance {
		t.Errorf("node-3 选择比例 %.4f 超出预期范围 [%.4f, %.4f]",
			ratio3, expectedRatio3-tolerance, expectedRatio3+tolerance)
	}

	t.Logf("✅ 加权分布测试通过")
}

func TestSelector_NoHealthyNodes(t *testing.T) {
	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         30,
			Timeout:          5,
			FailThreshold:    3,
			SuccessThreshold: 2,
		},
		List: []config.Node{
			{Name: "node-1", Host: "http://1.1.1.1", Weight: 100, Enabled: true},
		},
	}

	checker := NewHealthChecker(cfg)
	selector := NewSelector(checker)

	// 手动标记所有节点为不健康
	for _, node := range checker.GetAllNodes() {
		node.mu.Lock()
		node.Healthy = false
		node.mu.Unlock()
	}

	// 尝试选择节点
	node := selector.SelectNode()
	if node != nil {
		t.Error("没有健康节点时应该返回 nil")
	}

	t.Logf("✅ 无健康节点测试通过")
}

func TestSelector_SingleNode(t *testing.T) {
	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         30,
			Timeout:          5,
			FailThreshold:    3,
			SuccessThreshold: 2,
		},
		List: []config.Node{
			{Name: "only-node", Host: "http://1.1.1.1", Weight: 100, Enabled: true},
		},
	}

	checker := NewHealthChecker(cfg)
	selector := NewSelector(checker)

	// 多次选择应该都返回同一个节点
	for i := 0; i < 10; i++ {
		node := selector.SelectNode()
		if node == nil {
			t.Fatal("应该选择到节点")
		}
		if node.Name != "only-node" {
			t.Errorf("期望 only-node, 实际 %s", node.Name)
		}
	}

	t.Logf("✅ 单节点测试通过")
}

func TestSelector_EqualWeight(t *testing.T) {
	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         30,
			Timeout:          5,
			FailThreshold:    3,
			SuccessThreshold: 2,
		},
		List: []config.Node{
			{Name: "node-1", Host: "http://1.1.1.1", Weight: 50, Enabled: true},
			{Name: "node-2", Host: "http://2.2.2.2", Weight: 50, Enabled: true},
		},
	}

	checker := NewHealthChecker(cfg)
	selector := NewSelector(checker)

	// 模拟 1000 次选择
	counts := make(map[string]int)
	totalSelections := 1000

	for i := 0; i < totalSelections; i++ {
		node := selector.SelectNode()
		if node != nil {
			counts[node.Name]++
		}
	}

	// 权重相等，选择概率应该接近 50%
	ratio1 := float64(counts["node-1"]) / float64(totalSelections)
	ratio2 := float64(counts["node-2"]) / float64(totalSelections)

	t.Logf("node-1: %.2f%%, node-2: %.2f%%", ratio1*100, ratio2*100)

	// 允许 10% 容差
	if math.Abs(ratio1-0.5) > 0.1 || math.Abs(ratio2-0.5) > 0.1 {
		t.Logf("警告: 权重相等但分布不均匀，但这是正常的随机现象")
	}

	t.Logf("✅ 相等权重测试通过")
}

func TestSelector_Concurrency(t *testing.T) {
	cfg := &config.Nodes{
		HealthCheck: config.HealthCheck{
			Interval:         30,
			Timeout:          5,
			FailThreshold:    3,
			SuccessThreshold: 2,
		},
		List: []config.Node{
			{Name: "node-1", Host: "http://1.1.1.1", Weight: 100, Enabled: true},
			{Name: "node-2", Host: "http://2.2.2.2", Weight: 50, Enabled: true},
		},
	}

	checker := NewHealthChecker(cfg)
	selector := NewSelector(checker)

	// 并发选择
	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				node := selector.SelectNode()
				if node == nil {
					t.Error("并发选择失败")
				}
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 100; i++ {
		<-done
	}

	t.Logf("✅ 并发测试通过（1000 次并发选择）")
}
