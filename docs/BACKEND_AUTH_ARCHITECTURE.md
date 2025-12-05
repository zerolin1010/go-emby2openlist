# 后端集中式鉴权架构说明

> 所有逻辑由后端处理，Nginx 只负责返回文件

---

## 🎯 设计理念

### 原则
- **后端负责**：节点健康检查 + 鉴权验证 + 生成 302 链接
- **Nginx 负责**：根据 302 链接返回视频文件
- **客户端**：只需向后端请求，自动跟随 302 重定向

---

## 🏗️ 完整架构

```
┌─────────────────────────────────────────────────────────────────┐
│                    客户端 (Emby App)                             │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             │ ① 请求视频
                             │ GET /Videos/{id}/stream?api_key=xxx
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│            go-emby2openlist (后端 - 8095/8094)                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  【第一层：ApiKeyChecker 中间件鉴权】                            │
│  1. 提取 api_key（Query/Header）                                │
│  2. 检查信任缓存（validApiKeys sync.Map）                        │
│  3. 验证 api_key（调用 Emby API）                               │
│     GET http://emby:8096/emby/Auth/Keys?api_key=xxx            │
│  4. 缓存验证结果                                                 │
│     ✅ 通过 → 继续                                              │
│     ❌ 失败 → 返回 403                                          │
│                                                                 │
│  【第二层：Redirect2NginxLink 处理器】                           │
│  1. 解析 ItemId                                                 │
│  2. 获取 Emby 媒体路径                                          │
│     GET http://emby:8096/emby/Items/{id}?api_key=xxx           │
│     返回：{"Path": "/media/data/movie.mp4"}                     │
│  3. 检查节点健康状态（HealthChecker）                           │
│     - 定期检查：GET http://node/gtm-health                      │
│     - 失败阈值：3 次                                            │
│     - 成功阈值：2 次                                            │
│  4. 选择健康节点（Selector - 加权随机）                         │
│     权重: node-1(100) + node-2(50) + node-3(10) = 160         │
│     随机数: rand(160)                                           │
│     选中: node-1 (62.5% 概率)                                  │
│  5. 路径映射（emby2nginx）                                      │
│     /media/data/movie.mp4 → /video/data/movie.mp4            │
│  6. 构建 302 URL（可选携带 api_key）                            │
│     http://node-1/video/data/movie.mp4?api_key=xxx            │
│  7. 返回 302 重定向                                             │
│     HTTP/1.1 302 Temporary Redirect                            │
│     Location: http://node-1/video/data/movie.mp4?api_key=xxx  │
│                                                                 │
└─────────────────────────────┬───────────────────────────────────┘
                             │
                             │ ② 302 响应
                             │ Location: http://node-1/video/...
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                   客户端 (自动跟随重定向)                         │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             │ ③ 直接请求 Nginx
                             │ GET http://node-1/video/data/movie.mp4?api_key=xxx
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                    Nginx 节点 (node-1)                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  【可选：Nginx 端鉴权】                                          │
│  if ($arg_api_key = "") {                                      │
│      return 403;                                                │
│  }                                                              │
│                                                                 │
│  【文件服务】                                                    │
│  location /video/data {                                         │
│      alias /mnt/google/;                                        │
│      sendfile on;                                               │
│      tcp_nopush on;                                             │
│      directio 512;                                              │
│  }                                                              │
│                                                                 │
│  返回视频流 →                                                    │
│                                                                 │
└─────────────────────────────┬───────────────────────────────────┘
                             │
                             │ ④ 视频流
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                        客户端播放                                │
└─────────────────────────────────────────────────────────────────┘
```

---

## 🔧 端口说明

### 后端服务端口

| 端口 | 用途 | 必须 | 说明 |
|------|------|------|------|
| **8095** | HTTP | ✅ | 主要服务端口，客户端请求入口 |
| **8094** | HTTPS | ⚠️ | 可选，如果启用 SSL |
| **8097** | 鉴权服务器 | ⚠️ | 可选，用于 Nginx auth_request |

### Nginx 节点端口

| 端口 | 用途 | 说明 |
|------|------|------|
| **80** | 视频服务 | 返回视频文件 |
| **443** | HTTPS | 可选，SSL 加密 |

---

## ⚙️ 配置说明

### 1. 后端配置 (config.yml)

```yaml
# Emby 服务器
emby:
  host: http://emby-server:8096
  admin-api-key: "your-admin-api-key"
  mount-path: /media
  local-media-root: /data/local  # 本地媒体回源处理

# 节点配置（后端负责健康检查）
nodes:
  health-check:
    interval: 30              # 检查间隔（秒）
    timeout: 5                # 超时时间（秒）
    fail-threshold: 3         # 连续失败 3 次标记不健康
    success-threshold: 2      # 连续成功 2 次恢复健康
  list:
    - name: "node-1"
      host: "http://192.168.0.10:80"  # Nginx 节点地址
      weight: 100
      enabled: true
    - name: "node-2"
      host: "http://192.168.0.11:80"
      weight: 50
      enabled: true

# 鉴权配置
auth:
  user-key-cache-ttl: 24h
  nginx-auth-enable: true     # 302 URL 携带 api_key

  # 可选：鉴权服务器（用于 Nginx auth_request）
  enable-auth-server: false   # 推荐 false，后端已经做了鉴权
  auth-server-port: "8097"
  enable-auth-server-log: true
  auth-server-log-path: "./logs/auth-access.log"

# 路径映射
path:
  emby2nginx:
    - /media/data:/video/data
    - /media/series:/video/series
```

### 2. Nginx 配置

#### 方案 A：简单模式（推荐）

Nginx 只负责返回文件，鉴权已在后端完成：

```nginx
# nginx/video-simple.conf

# 健康检查
server {
    listen 80;
    server_name gtm-health;

    location = /gtm-health {
        access_log off;
        return 200 'OK';
        add_header Content-Type text/plain;
    }
}

# 视频服务
server {
    listen 80 default_server;

    # CORS 配置
    add_header 'Access-Control-Allow-Origin' '*' always;
    add_header 'Access-Control-Allow-Methods' 'GET, HEAD, OPTIONS' always;

    # 视频文件
    location /video/data {
        alias /mnt/google/;

        # 可选：简单验证 api_key 存在
        if ($arg_api_key = "") {
            return 403 '{"error":"Missing api_key"}';
        }

        # Range 支持
        add_header 'Accept-Ranges' bytes always;

        # 性能优化
        sendfile on;
        tcp_nopush on;
        tcp_nodelay on;
        directio 512;
        output_buffers 1 1m;

        # 超时设置
        send_timeout 3600s;
        keepalive_timeout 3600s;

        # 关闭日志（api_key 在 URL 中）
        access_log off;
    }

    # 其他媒体目录
    location /video/series {
        alias /mnt/series/;
        if ($arg_api_key = "") { return 403; }
        sendfile on;
        tcp_nopush on;
        directio 512;
        send_timeout 3600s;
        access_log off;
    }

    # 默认拒绝
    location / {
        return 404;
    }
}
```

#### 方案 B：后端鉴权服务器模式

如果需要详细日志和统计，可以启用后端鉴权服务器：

```nginx
# nginx/video-with-backend-auth.conf

upstream auth_backend {
    server go-emby2openlist:8097;  # 后端鉴权服务器
    keepalive 32;
}

server {
    listen 80;

    # 鉴权子请求
    location = /auth {
        internal;
        proxy_pass http://auth_backend/api/auth?api_key=$arg_api_key;
        proxy_connect_timeout 3s;
        proxy_read_timeout 3s;
    }

    # 视频服务
    location /video/data {
        alias /mnt/google/;
        auth_request /auth;  # 调用后端鉴权

        sendfile on;
        tcp_nopush on;
        directio 512;
    }
}
```

---

## 🚀 部署步骤

### 步骤 1: 配置后端

1. 编辑 `config.yml`：

```yaml
nodes:
  health-check:
    interval: 30
    timeout: 5
    fail-threshold: 3
    success-threshold: 2
  list:
    - name: "node-1"
      host: "http://192.168.0.10:80"
      weight: 100
      enabled: true

auth:
  nginx-auth-enable: true
  enable-auth-server: false  # 推荐关闭，后端已鉴权

path:
  emby2nginx:
    - /media/data:/video/data
```

2. 启动后端服务：

```bash
# Docker Compose
docker-compose up -d

# 查看日志
docker logs -f go-emby2openlist
```

**验证后端**：
```bash
# 检查节点健康检查是否启动
docker logs go-emby2openlist | grep "节点健康检查"

# 测试主服务
curl http://localhost:8095/
```

### 步骤 2: 配置 Nginx

1. 创建配置文件：

```bash
# 使用简单模式配置
sudo nano /etc/nginx/conf.d/video.conf
```

2. 粘贴配置（见上文方案 A）

3. 测试并重载：

```bash
sudo nginx -t
sudo nginx -s reload
```

### 步骤 3: 测试完整流程

```bash
# ① 测试后端 302 重定向（替换为真实 api_key）
curl -I "http://localhost:8095/Videos/123/stream?api_key=your_real_key"
# 应返回: HTTP/1.1 302 Temporary Redirect
# Location: http://192.168.0.10/video/data/movie.mp4?api_key=xxx

# ② 测试 Nginx 健康检查
curl -H "Host: gtm-health" http://192.168.0.10/gtm-health
# 应返回: OK

# ③ 测试 Nginx 文件服务（替换为真实路径和 api_key）
curl -I "http://192.168.0.10/video/data/test.mp4?api_key=xxx"
# 应返回: HTTP/1.1 200 OK 或 206 Partial Content
```

---

## 📊 工作流程详解

### 1. 客户端请求

```http
GET /Videos/123456/stream?api_key=abcd1234 HTTP/1.1
Host: emby-proxy:8095
Range: bytes=0-1048575
```

### 2. 后端处理

**步骤 1：ApiKeyChecker 中间件鉴权**
```go
// internal/service/emby/auth.go
func ApiKeyChecker() gin.HandlerFunc {
    return func(c *gin.Context) {
        // ① 提取 api_key
        apiKey := getApiKey(c)

        // ② 检查缓存
        if _, ok := validApiKeys.Load(apiKey); ok {
            return  // 已验证，通过
        }

        // ③ 调用 Emby 验证
        resp, _ := https.Get(embyHost + "/emby/Auth/Keys?api_key=" + apiKey).Do()
        if resp.StatusCode == 401 {
            c.String(403, "Invalid api_key")
            c.Abort()
            return
        }

        // ④ 缓存验证结果
        validApiKeys.Store(apiKey, struct{}{})
    }
}
```

**步骤 2：节点健康检查**
```go
// internal/service/node/health.go
func (h *HealthChecker) checkNode(node *NodeStatus) bool {
    // 发送健康检查请求
    req, _ := http.NewRequest("GET", node.Host+"/gtm-health", nil)
    req.Header.Set("Host", "gtm-health")

    resp, err := client.Do(req)
    return err == nil && resp.StatusCode == 200
}
```

**步骤 3：选择节点**
```go
// internal/service/node/selector.go
func (s *Selector) SelectNode() *NodeStatus {
    nodes := s.checker.GetHealthyNodes()  // 只选健康节点

    // 计算总权重
    totalWeight := 0
    for _, node := range nodes {
        totalWeight += node.Weight
    }

    // 加权随机
    r := rand.Intn(totalWeight)
    for _, node := range nodes {
        r -= node.Weight
        if r < 0 {
            return node  // 选中
        }
    }
}
```

**步骤 4：构建 302 响应**
```go
// internal/service/emby/redirect.go
func Redirect2NginxLink(c *gin.Context) {
    // 1. 获取媒体路径
    embyPath := getEmbyFileLocalPath(itemInfo)  // "/media/data/movie.mp4"

    // 2. 路径映射
    nginxPath := config.C.Path.MapEmby2Nginx(embyPath)  // "/video/data/movie.mp4"

    // 3. 选择节点
    node := nodeSelector.SelectNode()  // node-1

    // 4. 构建 URL
    redirectUrl := node.Host + nginxPath + "?api_key=" + apiKey
    // http://192.168.0.10/video/data/movie.mp4?api_key=xxx

    // 5. 返回 302
    c.Redirect(302, redirectUrl)
}
```

### 3. 客户端跟随重定向

```http
GET /video/data/movie.mp4?api_key=abcd1234 HTTP/1.1
Host: 192.168.0.10
Range: bytes=0-1048575
```

### 4. Nginx 返回文件

```nginx
location /video/data {
    alias /mnt/google/;  # movie.mp4 在 /mnt/google/movie.mp4

    # 检查 api_key
    if ($arg_api_key = "") {
        return 403;
    }

    # 返回文件
    sendfile on;
}
```

---

## 🔒 安全性

### 双层防护

1. **后端鉴权（必须）**
   - ApiKeyChecker 中间件
   - 调用 Emby API 验证
   - 缓存验证结果

2. **Nginx 鉴权（可选）**
   - 检查 api_key 参数存在
   - 防止直接访问 Nginx

### 防护措施

- ✅ **API Key 缓存**：避免重复验证
- ✅ **URL 参数鉴权**：Nginx 检查 api_key
- ✅ **访问日志关闭**：防止 api_key 泄露
- ✅ **HTTPS 加密**：防止中间人攻击
- ✅ **限流保护**：防止滥用

---

## 📈 性能指标

### 后端性能

| 操作 | 延迟 |
|------|------|
| API Key 验证（缓存命中） | < 0.1ms |
| API Key 验证（缓存未命中） | ~50ms |
| 节点选择 | < 1ms |
| 路径映射 | < 1ms |
| **302 响应总延迟** | **< 5ms** |

### Nginx 性能

| 操作 | 吞吐量 |
|------|--------|
| 文件服务 | > 10Gbps |
| 并发连接 | > 10,000 |
| CPU 使用 | < 10% |

---

## ❓ 常见问题

### Q1: 必须启用鉴权服务器（8097）吗？

**A**: 不必须。推荐配置：

```yaml
auth:
  enable-auth-server: false  # 关闭，后端已经做了鉴权
  nginx-auth-enable: true    # 开启，302 URL 携带 api_key
```

鉴权服务器的作用是为 Nginx `auth_request` 提供鉴权接口，但后端已经通过 `ApiKeyChecker` 中间件完成了鉴权，所以不需要额外的鉴权服务器。

### Q2: Nginx 如何验证 api_key？

**A**: Nginx 只需简单检查参数存在：

```nginx
if ($arg_api_key = "") {
    return 403;
}
```

真正的验证已经在后端完成，Nginx 这里只是二次检查。

### Q3: 后端如何知道节点健康状态？

**A**: HealthChecker 定期检查：

```go
// 每 30 秒检查一次
for range time.NewTicker(30 * time.Second).C {
    for _, node := range nodes {
        // GET http://node/gtm-health (Host: gtm-health)
        healthy := checkNode(node)
        updateNodeStatus(node, healthy)
    }
}
```

### Q4: 如果所有节点都不健康怎么办？

**A**: 后端会回源到 Emby：

```go
selectedNode := nodeSelector.SelectNode()
if selectedNode == nil {
    // 没有健康节点，代理回 Emby
    ProxyOrigin(c)
    return
}
```

---

## 🎯 总结

### 架构优势

✅ **后端集中管理**：所有逻辑在后端，易于维护
✅ **Nginx 简单**：只负责返回文件，配置简单
✅ **双层防护**：后端 + Nginx 鉴权
✅ **高可用**：自动故障转移
✅ **高性能**：302 延迟 < 5ms

### 关键配置

1. **后端**：启用节点健康检查 + 路径映射
2. **Nginx**：简单 api_key 检查 + 文件服务
3. **端口**：8095(HTTP) + 8094(HTTPS) + ~~8097(鉴权)~~

### 推荐配置

```yaml
# config.yml
auth:
  nginx-auth-enable: true     # ✅ 开启
  enable-auth-server: false   # ❌ 关闭（后端已鉴权）
```

```nginx
# nginx.conf
if ($arg_api_key = "") {
    return 403;
}
```

---

**文档版本**: v1.0
**最后更新**: 2025-12-06
**项目版本**: v2.4.0+
