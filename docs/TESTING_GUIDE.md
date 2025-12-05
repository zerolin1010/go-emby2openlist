# 核心功能测试指南

本文档提供完整的测试步骤，确保项目改造后所有核心功能正常工作。

---

## 📋 测试清单概览

- ✅ 编译测试
- ✅ Docker 构建测试
- ⏳ 路径映射测试
- ⏳ 健康检查测试
- ⏳ 节点选择测试
- ⏳ 302 重定向测试
- ⏳ Range 请求测试
- ⏳ CORS 跨域测试
- ⏳ 用户鉴权测试
- ⏳ Telegram Bot 测试

---

## ✅ 测试 1: 编译测试

### 目的
验证代码可以正确编译，没有语法错误或依赖问题。

### 测试步骤

```bash
# 1. 清理旧的构建产物
rm -f go-emby2openlist

# 2. 下载依赖
go mod tidy
go mod download

# 3. 编译
go build -o go-emby2openlist

# 4. 检查编译结果
ls -lh go-emby2openlist
```

### 预期结果

- ✅ 编译成功，没有错误
- ✅ 生成 `go-emby2openlist` 可执行文件
- ✅ 文件大小约 20-30MB

### 故障排查

如果编译失败：

1. 检查 Go 版本
```bash
go version  # 应该是 1.20 或更高
```

2. 清理缓存
```bash
go clean -cache -modcache -i -r
go mod download
```

3. 检查是否有 import 错误
```bash
go build 2>&1 | grep "import"
```

---

## ✅ 测试 2: Docker 构建测试

### 目的
验证 Docker 镜像可以正确构建。

### 测试步骤

```bash
# 1. 构建 Docker 镜像
docker build -t go-emby2openlist:test .

# 2. 查看镜像
docker images | grep go-emby2openlist

# 3. 检查镜像大小
docker inspect go-emby2openlist:test | grep Size
```

### 预期结果

- ✅ 构建成功，没有错误
- ✅ 镜像大小约 30-50MB（两阶段构建，最终镜像基于 Alpine）
- ✅ 镜像包含正确的可执行文件

### 测试容器启动

```bash
# 创建测试配置文件
cp config-example.yml config-test.yml

# 启动容器（测试模式）
docker run --rm -it \
  -v $(pwd)/config-test.yml:/app/config.yml \
  -p 8095:8095 \
  go-emby2openlist:test
```

观察日志输出，按 Ctrl+C 停止。

---

## ⏳ 测试 3: 路径映射测试

### 目的
验证 Emby 路径到 Nginx 路径的映射功能正常。

### 前置条件

配置 `config.yml` 中的路径映射：

```yaml
path:
  emby2nginx:
    - /media/data:/video/data
    - /media/data1:/video/data1
```

### 测试步骤

#### 3.1 单元测试

创建测试文件 `internal/config/path_test.go`:

```go
package config

import "testing"

func TestMapEmby2Nginx(t *testing.T) {
	// 模拟配置
	C = &Config{
		Path: &Path{
			Emby2Nginx: map[string]string{
				"/media/data":  "/video/data",
				"/media/data1": "/video/data1",
			},
		},
	}

	tests := []struct {
		embyPath   string
		wantNginx  string
		wantOK     bool
	}{
		{
			embyPath:  "/media/data/movie/test.mp4",
			wantNginx: "/video/data/movie/test.mp4",
			wantOK:    true,
		},
		{
			embyPath:  "/media/data1/series/show.mkv",
			wantNginx: "/video/data1/series/show.mkv",
			wantOK:    true,
		},
		{
			embyPath:  "/other/path/video.mp4",
			wantNginx: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		got, ok := C.Path.MapEmby2Nginx(tt.embyPath)
		if got != tt.wantNginx || ok != tt.wantOK {
			t.Errorf("MapEmby2Nginx(%q) = (%q, %v), want (%q, %v)",
				tt.embyPath, got, ok, tt.wantNginx, tt.wantOK)
		}
	}
}
```

运行测试：

```bash
go test ./internal/config -v -run TestMapEmby2Nginx
```

#### 3.2 集成测试

```bash
# 1. 启动服务
./go-emby2openlist &
SERVER_PID=$!

# 2. 模拟请求（需要有效的 Emby ItemId）
curl -i "http://localhost:8095/videos/{itemId}/stream?api_key=test"

# 3. 观察日志，查看路径映射过程
grep "路径映射" logs/*.log

# 4. 停止服务
kill $SERVER_PID
```

### 预期结果

- ✅ 单元测试全部通过
- ✅ 日志显示正确的路径映射
- ✅ Emby 路径 `/media/data/movie/test.mp4` 映射为 `/video/data/movie/test.mp4`

---

## ⏳ 测试 4: 健康检查测试

### 目的
验证节点健康检查机制工作正常。

### 前置条件

1. 配置至少 2 个节点
2. 至少一个节点正常运行 Nginx

### 测试步骤

#### 4.1 配置节点

```yaml
nodes:
  health-check:
    interval: 10       # 10秒检查一次（测试用）
    timeout: 3
    fail-threshold: 2
    success-threshold: 1

  list:
    - name: "node-1"
      host: "http://192.168.1.100:80"
      weight: 100
      enabled: true
    - name: "node-2"
      host: "http://192.168.1.101:80"
      weight: 80
      enabled: true
```

#### 4.2 手动测试健康检查接口

```bash
# 测试 node-1
curl -v -H "Host: gtm-health" http://192.168.1.100/gtm-health

# 测试 node-2
curl -v -H "Host: gtm-health" http://192.168.1.101/gtm-health
```

预期响应：
```
HTTP/1.1 200 OK
Content-Type: text/plain
Content-Length: 2

OK
```

#### 4.3 观察自动健康检查

```bash
# 1. 启动服务
./go-emby2openlist 2>&1 | tee test-health.log

# 2. 观察日志（另一个终端）
tail -f test-health.log | grep -E "健康|检查|节点"
```

预期日志输出：
```
[INFO] 正在初始化节点健康检查模块...
[INFO] 节点 node-1 健康检查成功
[INFO] 节点 node-2 健康检查成功
```

#### 4.4 模拟节点故障

```bash
# 1. 在 node-1 上停止 Nginx
ssh user@192.168.1.100 "sudo systemctl stop nginx"

# 2. 观察日志（30秒内应该检测到）
tail -f test-health.log | grep "node-1"
```

预期日志输出：
```
[WARN] 节点 node-1 健康检查失败: context deadline exceeded
[ERROR] 节点 node-1 标记为不健康
```

#### 4.5 模拟节点恢复

```bash
# 1. 在 node-1 上启动 Nginx
ssh user@192.168.1.100 "sudo systemctl start nginx"

# 2. 观察日志（20秒内应该恢复）
tail -f test-health.log | grep "node-1"
```

预期日志输出：
```
[SUCCESS] 节点 node-1 恢复健康
```

### 预期结果

- ✅ 健康节点返回 200 OK
- ✅ 不健康节点被自动检测并标记
- ✅ 恢复的节点被自动重新启用
- ✅ 连续失败 N 次才标记为不健康（配置的阈值）

---

## ⏳ 测试 5: 节点选择测试

### 目的
验证加权随机选择算法和负载均衡功能。

### 测试步骤

#### 5.1 单元测试

创建测试文件 `internal/service/node/selector_test.go`:

```go
package node

import (
	"testing"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
)

func TestWeightedSelection(t *testing.T) {
	// 创建测试节点
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
	for i := 0; i < 1000; i++ {
		node := selector.SelectNode()
		if node != nil {
			counts[node.Name]++
		}
	}

	t.Logf("选择统计: %+v", counts)

	// 验证权重比例（允许10%误差）
	total := float64(counts["node-1"] + counts["node-2"] + counts["node-3"])
	ratio1 := float64(counts["node-1"]) / total
	ratio2 := float64(counts["node-2"]) / total

	expectedRatio1 := 100.0 / 160.0 // 约 62.5%
	expectedRatio2 := 50.0 / 160.0  // 约 31.25%

	if ratio1 < expectedRatio1-0.1 || ratio1 > expectedRatio1+0.1 {
		t.Errorf("node-1 选择比例 %.2f 超出预期范围 [%.2f, %.2f]",
			ratio1, expectedRatio1-0.1, expectedRatio1+0.1)
	}

	if ratio2 < expectedRatio2-0.1 || ratio2 > expectedRatio2+0.1 {
		t.Errorf("node-2 选择比例 %.2f 超出预期范围 [%.2f, %.2f]",
			ratio2, expectedRatio2-0.1, expectedRatio2+0.1)
	}
}
```

运行测试：

```bash
go test ./internal/service/node -v -run TestWeightedSelection
```

#### 5.2 集成测试

```bash
# 创建测试脚本
cat > test-selection.sh << 'EOF'
#!/bin/bash

echo "开始测试节点选择..."

# 发送 100 个请求，统计重定向到的节点
for i in {1..100}; do
  curl -s -I "http://localhost:8095/videos/test123/stream?api_key=test" \
    | grep -i "Location" \
    | awk '{print $2}'
done | sort | uniq -c

echo "测试完成"
EOF

chmod +x test-selection.sh
./test-selection.sh
```

### 预期结果

- ✅ 权重高的节点被选中的概率更高
- ✅ 权重比例符合配置（允许统计误差）
- ✅ 不健康的节点不会被选中
- ✅ 禁用的节点不会被选中

---

## ⏳ 测试 6: 302 重定向测试

### 目的
验证 HTTP 302 重定向功能和 URL 构建正确性。

### 测试步骤

#### 6.1 基础重定向测试

```bash
# 1. 发送播放请求
curl -i "http://localhost:8095/videos/{itemId}/stream?api_key=your_key"
```

预期响应：
```http
HTTP/1.1 302 Temporary Redirect
Location: http://192.168.1.100/video/data/movie/test.mp4?api_key=cached_key
Access-Control-Allow-Origin: *
Content-Length: 0
```

验证点：
- ✅ 状态码是 302
- ✅ Location 头包含节点地址
- ✅ Location 头包含正确的 Nginx 路径
- ✅ Location 头包含 api_key 参数（如果启用鉴权）

#### 6.2 测试不同媒体类型

```bash
# 测试视频
curl -I "http://localhost:8095/videos/{videoId}/stream?api_key=test"

# 测试下载
curl -I "http://localhost:8095/Items/{itemId}/Download?api_key=test"
```

#### 6.3 测试 MediaSourceId

```bash
# 携带 MediaSourceId
curl -I "http://localhost:8095/videos/{itemId}/stream?MediaSourceId=abc123&api_key=test"
```

### 预期结果

- ✅ 返回 302 重定向
- ✅ Location 指向健康的 Nginx 节点
- ✅ 路径正确映射
- ✅ 携带必要的查询参数

---

## ⏳ 测试 7: Range 请求测试

### 目的
验证视频拖拽（Range 请求）功能正常。

### 测试步骤

#### 7.1 测试 Range 请求支持

```bash
# 1. 请求前 1024 字节
curl -I -H "Range: bytes=0-1023" \
  "http://node-1-ip/video/data/movie/test.mp4"
```

预期响应：
```http
HTTP/1.1 206 Partial Content
Content-Range: bytes 0-1023/12345678
Content-Length: 1024
Accept-Ranges: bytes
```

#### 7.2 测试中间部分

```bash
# 请求中间 1KB
curl -H "Range: bytes=1000000-1001023" \
  "http://node-1-ip/video/data/movie/test.mp4" \
  -o /tmp/test-range.bin

# 验证文件大小
ls -lh /tmp/test-range.bin  # 应该是 1024 bytes
```

#### 7.3 端到端测试

```bash
# 1. 获取重定向地址
REDIRECT_URL=$(curl -s -I "http://localhost:8095/videos/{itemId}/stream?api_key=test" \
  | grep -i "Location" \
  | awk '{print $2}' \
  | tr -d '\r')

# 2. 直接测试 Range 请求
curl -I -H "Range: bytes=0-1023" "$REDIRECT_URL"
```

### 预期结果

- ✅ 支持 Range 请求
- ✅ 返回 206 Partial Content
- ✅ Content-Range 头正确
- ✅ Accept-Ranges: bytes 存在
- ✅ 实际下载的数据大小正确

---

## ⏳ 测试 8: CORS 跨域测试

### 目的
验证 CORS 配置正确，支持 Web 播放器。

### 测试步骤

#### 8.1 测试 OPTIONS 预检请求

```bash
curl -i -X OPTIONS \
  -H "Origin: https://example.com" \
  -H "Access-Control-Request-Method: GET" \
  -H "Access-Control-Request-Headers: Range" \
  "http://node-1-ip/video/data/movie/test.mp4"
```

预期响应：
```http
HTTP/1.1 204 No Content
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, HEAD, OPTIONS
Access-Control-Allow-Headers: Range, Origin, Accept, Content-Type, Authorization, X-Emby-Token, X-Emby-Authorization
Access-Control-Max-Age: 86400
```

#### 8.2 测试实际请求

```bash
curl -i \
  -H "Origin: https://example.com" \
  -H "Range: bytes=0-1023" \
  "http://node-1-ip/video/data/movie/test.mp4"
```

预期响应头包含：
```http
Access-Control-Allow-Origin: *
Access-Control-Expose-Headers: Content-Length, Content-Range, Accept-Ranges
```

### 预期结果

- ✅ OPTIONS 请求返回 204
- ✅ CORS 头正确配置
- ✅ 支持所有必要的请求方法和头
- ✅ Web 播放器可以正常播放

---

## ⏳ 测试 9: 用户鉴权测试

### 目的
验证用户 API Key 缓存功能。

### 测试步骤

#### 9.1 配置鉴权

```yaml
emby:
  admin-api-key: "your-admin-api-key"

auth:
  user-key-cache-ttl: 1h
  nginx-auth-enable: true
```

#### 9.2 测试首次请求

```bash
# 1. 发送请求（使用用户的 api_key）
curl -i "http://localhost:8095/videos/{itemId}/stream?api_key=user_key_123"

# 2. 观察日志
tail -f logs/*.log | grep -i "key"
```

预期日志：
```
[INFO] 用户 API Key 缓存未命中，从 Emby 获取
[INFO] 缓存用户 {userId} 的 API Key
```

#### 9.3 测试缓存命中

```bash
# 立即发送第二个请求
curl -i "http://localhost:8095/videos/{itemId}/stream?api_key=user_key_123"
```

预期日志：
```
[INFO] 用户 API Key 缓存命中
```

#### 9.4 验证 302 URL 携带正确的 Key

```bash
curl -s -I "http://localhost:8095/videos/{itemId}/stream?api_key=user_key" \
  | grep -i "Location"
```

Location 应该包含 `?api_key=cached_key`

### 预期结果

- ✅ 首次请求从 Emby 获取 Key
- ✅ 后续请求使用缓存
- ✅ 302 URL 携带正确的 Key
- ✅ TTL 到期后重新获取

---

## ⏳ 测试 10: Telegram Bot 测试

### 目的
验证 Telegram Bot 节点管理功能。

### 测试步骤

参考 [Telegram Bot 测试文档](./TELEGRAM_BOT.md#-测试步骤)

---

## 📊 完整测试报告模板

```markdown
# 测试报告

**测试时间**: 2025-01-15 10:00:00
**测试环境**:
- OS: Ubuntu 20.04
- Go: 1.21.5
- Docker: 20.10.23

## 测试结果

| 测试项 | 状态 | 备注 |
|--------|------|------|
| 编译测试 | ✅ 通过 | 编译时间: 15s |
| Docker 构建 | ✅ 通过 | 镜像大小: 35MB |
| 路径映射 | ✅ 通过 | 所有映射规则正确 |
| 健康检查 | ✅ 通过 | 故障检测时间: 20s |
| 节点选择 | ✅ 通过 | 权重分布符合预期 |
| 302 重定向 | ✅ 通过 | 平均响应时间: 5ms |
| Range 请求 | ✅ 通过 | 支持视频拖拽 |
| CORS 跨域 | ✅ 通过 | Web 播放正常 |
| 用户鉴权 | ✅ 通过 | 缓存命中率: 95% |
| Telegram Bot | ✅ 通过 | 所有命令正常 |

## 性能指标

- 302 重定向平均延迟: 5ms
- 健康检查间隔: 30s
- 节点故障检测时间: < 90s
- 用户 Key 缓存命中率: > 90%

## 发现的问题

1. [问题描述]
   - 严重程度: 高/中/低
   - 影响范围: [...]
   - 解决方案: [...]

## 建议

1. [建议1]
2. [建议2]

---
测试人员: [姓名]
```

---

## 🔧 故障排查工具

### 查看实时日志

```bash
# 所有日志
docker logs -f go-emby2openlist

# 仅健康检查相关
docker logs -f go-emby2openlist 2>&1 | grep -E "健康|health"

# 仅重定向相关
docker logs -f go-emby2openlist 2>&1 | grep -E "重定向|302|Redirect"
```

### 检查节点状态

```bash
# 使用 Telegram Bot
/status

# 或手动请求（需要添加管理接口）
curl http://localhost:8095/admin/nodes/status
```

### 网络诊断

```bash
# 测试节点连通性
ping node-1-ip

# 测试健康检查接口
curl -v -H "Host: gtm-health" http://node-1-ip/gtm-health

# 测试视频文件访问
curl -I http://node-1-ip/video/data/movie/test.mp4
```

---

**更新时间**: 2025-01-15
**版本**: v2.3.2+nginx
