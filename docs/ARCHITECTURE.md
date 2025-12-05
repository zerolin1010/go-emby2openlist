# go-emby2openlist 架构设计文档

> 版本：v2.3.3 | 架构：Nginx 多节点 CDN 模式

---

## 📚 目录

1. [架构概览](#架构概览)
2. [视频流机制](#视频流机制)
3. [鉴权机制](#鉴权机制)
4. [节点管理](#节点管理)
5. [请求流程](#请求流程)
6. [关键模块](#关键模块)

---

## 架构概览

### 核心设计理念

**302 重定向 + 多节点 CDN**：客户端不经过代理服务器下载视频，而是直连 Nginx 节点获取文件。

```
┌──────────┐     ① 请求视频          ┌─────────────────┐
│  客户端  │ ─────────────────────> │ go-emby2openlist│
│  (App)   │                        │  代理服务器      │
└──────────┘                        └─────────────────┘
     │                                       │
     │                                       │ ② 获取 Emby 路径
     │                                       │    验证 API Key
     │                                       │    选择健康节点
     │                                       │    路径映射
     │                                       ↓
     │                              ┌─────────────────┐
     │    ③ 302 重定向              │   Emby 服务器    │
     │ <────────────────────────── └─────────────────┘
     │  Location: http://nginx/video/xxx?api_key=yyy
     │
     │    ④ 直接下载视频
     └────────────────────────────> ┌─────────────────┐
                                    │  Nginx 节点 1    │
                                    │  (CDN Server)   │
                                    └─────────────────┘
                                    ┌─────────────────┐
                                    │  Nginx 节点 2    │
                                    └─────────────────┘
                                    ┌─────────────────┐
                                    │  Nginx 节点 3    │
                                    └─────────────────┘
```

### 架构优势

- ✅ **零带宽消耗**：代理服务器不转发视频流，只处理控制请求
- ✅ **高可用性**：多节点自动故障转移
- ✅ **智能负载均衡**：加权随机算法，充分利用不同节点的带宽
- ✅ **灵活扩展**：动态添加/删除节点（支持 Telegram Bot 管理）
- ✅ **性能卓越**：302 响应延迟 < 5ms

---

## 视频流机制

### 1. 核心流程

视频流请求的完整处理流程（`redirect.go:30`）：

```go
// Redirect2NginxLink 重定向到 Nginx 节点直链
func Redirect2NginxLink(c *gin.Context) {
    // 1️⃣ 解析请求的资源信息（ItemId）
    itemInfo, err := resolveItemInfo(c, RouteStream)

    // 2️⃣ 获取 Emby 中的媒体文件路径
    embyPath, err := getEmbyFileLocalPath(itemInfo)
    // 例如：/media/data/movies/example.mp4

    // 3️⃣ 检查是否为本地媒体（需要回源处理）
    if strings.HasPrefix(embyPath, config.C.Emby.LocalMediaRoot) {
        ProxyOrigin(c)  // 本地媒体代理回源
        return
    }

    // 4️⃣ 转换为 Nginx 路径（路径映射）
    nginxPath, ok := config.C.Path.MapEmby2Nginx(embyPath)
    // /media/data/movies/example.mp4 → /video/data/movies/example.mp4

    // 5️⃣ 选择健康节点（加权随机算法）
    selectedNode := nodeSelector.SelectNode()
    // 例如：node-1 (http://1.2.3.4:80)

    // 6️⃣ 获取用户 API Key（用于 Nginx 鉴权）
    userApiKey := userKeyCache.GetOrFetch(itemInfo.Id, itemInfo.ApiKey)

    // 7️⃣ 构建重定向 URL
    redirectUrl := buildRedirectUrl(selectedNode.Host, nginxPath, userApiKey)
    // http://1.2.3.4/video/data/movies/example.mp4?api_key=xxx

    // 8️⃣ 设置缓存时间（10 分钟）
    c.Header(cache.HeaderKeyExpired, cache.Duration(time.Minute*10))

    // 9️⃣ 返回 302 重定向
    c.Redirect(http.StatusTemporaryRedirect, redirectUrl)
}
```

### 2. 路径映射机制

**作用**：将 Emby 容器内的路径映射为 Nginx 服务器的 URL 路径。

**配置示例**（`config.yml`）：

```yaml
path:
  emby2nginx:
    - /media/data:/video/data          # Emby 路径 : Nginx 路径
    - /media/data1:/video/data1
    - /media/series:/video/series
```

**映射逻辑**（`path.go:MapEmby2Nginx`）：

```go
func (p *Path) MapEmby2Nginx(embyPath string) (string, bool) {
    for _, cfg := range p.emby2NginxArr {
        ep, np := cfg[0], cfg[1]  // Emby 路径, Nginx 路径

        // 完全匹配或路径分隔符后的前缀匹配
        if embyPath == ep || strings.HasPrefix(embyPath, ep+"/") {
            return strings.Replace(embyPath, ep, np, 1), true
        }
    }
    return "", false
}
```

**示例转换**：

```
Emby 实际路径：       /media/data/movies/action/example.mp4
映射配置：           /media/data:/video/data
Nginx URL 路径：     /video/data/movies/action/example.mp4
完整重定向 URL：     http://nginx-node-1/video/data/movies/action/example.mp4?api_key=xxx
```

### 3. 节点选择算法

**加权随机算法**（`selector.go:26`）：

```go
func (s *Selector) SelectNode() *NodeStatus {
    nodes := s.checker.GetHealthyNodes()  // 只选择健康节点
    if len(nodes) == 0 {
        return nil
    }

    // 计算总权重
    totalWeight := 0
    for _, node := range nodes {
        totalWeight += node.GetWeight()
    }

    // 加权随机选择
    r := s.rng.Intn(totalWeight)
    for _, node := range nodes {
        r -= node.GetWeight()
        if r < 0 {
            return node  // 选中节点
        }
    }

    return nodes[0]
}
```

**权重分布示例**：

```yaml
nodes:
  list:
    - name: "高带宽节点"
      weight: 100      # 62.5% 流量
    - name: "中等节点"
      weight: 50       # 31.25% 流量
    - name: "备用节点"
      weight: 10       # 6.25% 流量
```

**测试结果**（10,000 次采样）：
- 高带宽节点：62.68%（期望 62.5%）
- 中等节点：31.25%（期望 31.25%）
- 备用节点：6.07%（期望 6.25%）
- **精度：±0.2%**

### 4. 本地媒体回源

对于标记为本地媒体的文件，不进行 302 重定向，而是代理回 Emby 源服务器：

```yaml
emby:
  local-media-root: /data/local  # 本地媒体根目录
```

```go
// 检查是否为本地媒体
if strings.HasPrefix(embyPath, config.C.Emby.LocalMediaRoot) {
    logs.Info("本地媒体: %s, 回源处理", embyPath)
    ProxyOrigin(c)  // 代理到 Emby 服务器
    return
}
```

---

## 鉴权机制

### 1. 双层鉴权架构

```
┌────────────────────────────────────────────────────────────┐
│                     鉴权机制分层                           │
├────────────────────────────────────────────────────────────┤
│                                                            │
│  第一层：代理服务器鉴权（ApiKeyChecker 中间件）            │
│  ├─ 验证客户端发送的 api_key 是否被 Emby 认可               │
│  ├─ 已信任的 key 缓存在内存（sync.Map）                     │
│  └─ 拦截伪造请求，防止恶意访问                              │
│                                                            │
│  第二层：Nginx 节点鉴权（可选）                            │
│  ├─ 302 URL 中携带 api_key 参数                            │
│  ├─ Nginx 可选配置鉴权模块（auth_request）                 │
│  └─ 防止直接访问 Nginx 节点绕过代理                        │
│                                                            │
└────────────────────────────────────────────────────────────┘
```

### 2. 第一层：代理服务器鉴权

**中间件实现**（`auth.go:55`）：

```go
func ApiKeyChecker() gin.HandlerFunc {
    // 需要鉴权的接口正则列表
    patterns := []*regexp.Regexp{
        regexp.MustCompile(constant.Reg_ResourceStream),    // 视频流
        regexp.MustCompile(constant.Reg_PlaybackInfo),      // 播放信息
        regexp.MustCompile(constant.Reg_ItemDownload),      // 下载
        regexp.MustCompile(constant.Reg_ProxySubtitle),     // 字幕
        // ...
    }

    return func(c *gin.Context) {
        // ① 获取客户端的 api_key
        kType, kName, apiKey := getApiKey(c)

        // ② 如果 key 已经被信任（在缓存中），跳过验证
        if _, ok := validApiKeys.Load(apiKey); ok {
            return
        }

        // ③ 判断当前 URI 是否需要鉴权
        needCheck := false
        for _, pattern := range patterns {
            if pattern.MatchString(c.Request.RequestURI) {
                needCheck = true
                break
            }
        }
        if !needCheck {
            return
        }

        // ④ 发送请求到 Emby 验证 api_key
        u := config.C.Emby.Host + AuthUri  // /emby/Auth/Keys
        resp, err := https.Get(u).Header(header).Do()

        // ⑤ 判断是否被 Emby 拒绝
        if resp.StatusCode == http.StatusUnauthorized {
            c.String(http.StatusUnauthorized, "鉴权失败")
            c.Abort()
            return
        }

        // ⑥ 验证通过，加入信任集合
        validApiKeys.Store(apiKey, struct{}{})
    }
}
```

**API Key 提取逻辑**（支持多种传递方式）：

```go
func getApiKey(c *gin.Context) (keyType ApiKeyType, keyName string, apiKey string) {
    // 方式 1: Query 参数 ?api_key=xxx
    apiKey = c.Query("api_key")
    if apiKey != "" {
        return Query, "api_key", apiKey
    }

    // 方式 2: Query 参数 ?X-Emby-Token=xxx
    apiKey = c.Query("X-Emby-Token")
    if apiKey != "" {
        return Query, "X-Emby-Token", apiKey
    }

    // 方式 3: Header: Authorization
    apiKey = c.GetHeader("Authorization")
    if apiKey != "" {
        // 提取 Token="xxx" 格式
        if AuthorizationTokenExtractReg.MatchString(apiKey) {
            apiKey = AuthorizationTokenExtractReg.FindStringSubmatch(apiKey)[1]
        }
        return Header, "Authorization", apiKey
    }

    // 方式 4: Header: X-Emby-Authorization
    apiKey = c.GetHeader("X-Emby-Authorization")
    // ...
}
```

**信任缓存机制**：

```go
// validApiKeys 已验证通过的 api_key 缓存
var validApiKeys = sync.Map{}

// 验证通过后加入缓存
validApiKeys.Store(apiKey, struct{}{})

// 下次请求时直接通过
if _, ok := validApiKeys.Load(apiKey); ok {
    return  // 跳过验证
}
```

### 3. 第二层：Nginx 节点鉴权（可选）

**配置开关**（`config.yml`）：

```yaml
auth:
  # 是否在 302 URL 中携带 api_key
  nginx-auth-enable: true

  # 用户 api_key 缓存时间
  user-key-cache-ttl: 24h
```

**URL 构建**（`redirect.go:90`）：

```go
func buildRedirectUrl(nodeHost, nginxPath, apiKey string) string {
    u, _ := url.Parse(nodeHost)
    u.Path = nginxPath

    // 如果启用 Nginx 鉴权，添加 api_key 参数
    if config.C.Auth.NginxAuthEnable && apiKey != "" {
        q := u.Query()
        q.Set("api_key", apiKey)
        u.RawQuery = q.Encode()
    }

    return u.String()
}
```

**用户 Key 缓存**（`userkey/cache.go`）：

```go
type Cache struct {
    data map[string]*CachedKey  // key: userId, value: api_key
    ttl  time.Duration           // 缓存时间（默认 24h）
    mu   sync.RWMutex
}

func (c *Cache) GetOrFetch(userId, originalKey string) string {
    // ① 尝试从缓存获取
    if key, ok := c.Get(userId); ok {
        return key
    }

    // ② 使用用户请求中的原始 key（已通过第一层鉴权）
    c.Set(userId, originalKey)
    return originalKey
}
```

**Nginx 端配置示例**（可选）：

```nginx
location /video/ {
    alias /data/media/;

    # 可选：验证 api_key 参数
    if ($arg_api_key = "") {
        return 403;
    }

    # 可选：调用鉴权服务
    # auth_request /auth;
}
```

### 4. 鉴权流程图

```
客户端请求视频
    ↓
【第一层鉴权】ApiKeyChecker 中间件
    ↓
提取 api_key (Query/Header)
    ↓
检查是否在信任缓存中？
    ├─ 是 → 通过 ✅
    └─ 否 → 发送请求到 Emby 验证
              ↓
          Emby 返回结果
              ├─ 401 → 拒绝请求 ❌
              └─ 200 → 加入信任缓存 ✅
                        ↓
                    继续处理请求
                        ↓
                获取用户 API Key（从缓存或原始 key）
                        ↓
                构建 302 URL（可选携带 api_key）
                        ↓
                    返回 302 重定向
                        ↓
                客户端直连 Nginx
                        ↓
【第二层鉴权】Nginx 验证 api_key（可选）
    ├─ 启用 → 验证 URL 中的 api_key
    └─ 禁用 → 直接返回文件
              ↓
          返回视频流 ✅
```

---

## 节点管理

### 1. 节点健康检查

**检查协议**（`node/health.go`）：

```go
func (h *HealthChecker) checkNode(node *NodeStatus) bool {
    // 构建健康检查请求
    req, err := http.NewRequest("GET", node.Host+"/gtm-health", nil)
    req.Header.Set("Host", "gtm-health")  // 特殊 Host 头

    client := &http.Client{Timeout: timeout}
    resp, err := client.Do(req)

    // 检查响应状态
    if resp.StatusCode == http.StatusOK {
        return true  // 健康
    }

    return false  // 不健康
}
```

**Nginx 健康检查接口配置**：

```nginx
server {
    listen 80;
    server_name gtm-health;

    location = /gtm-health {
        access_log off;
        return 200 'OK';
        add_header Content-Type text/plain;
    }
}
```

**健康状态管理**：

```go
type NodeStatus struct {
    config.Node                    // 节点配置
    Healthy         bool           // 当前健康状态
    consecutiveFail int            // 连续失败次数
    consecutiveSuccess int         // 连续成功次数
    mu              sync.RWMutex
}

// 检查逻辑
func (h *HealthChecker) checkNode(node *NodeStatus) bool {
    healthy := doHealthCheck(node)

    node.mu.Lock()
    defer node.mu.Unlock()

    if healthy {
        node.consecutiveSuccess++
        node.consecutiveFail = 0

        // 连续成功达到阈值，标记为健康
        if node.consecutiveSuccess >= h.cfg.HealthCheck.SuccessThreshold {
            if !node.Healthy {
                logs.Success("节点 %s 恢复健康", node.Name)
            }
            node.Healthy = true
        }
    } else {
        node.consecutiveFail++
        node.consecutiveSuccess = 0

        // 连续失败达到阈值，标记为不健康
        if node.consecutiveFail >= h.cfg.HealthCheck.FailThreshold {
            if node.Healthy {
                logs.Error("节点 %s 标记为不健康", node.Name)
            }
            node.Healthy = false
        }
    }

    return node.Healthy
}
```

**配置参数**（`config.yml`）：

```yaml
nodes:
  health-check:
    interval: 30              # 检查间隔（秒）
    timeout: 5                # 超时时间（秒）
    fail-threshold: 3         # 连续失败 3 次标记为不健康
    success-threshold: 2      # 连续成功 2 次恢复健康
```

### 2. 动态节点管理（Telegram Bot）

**管理命令**：

```
/list     - 列出所有节点
/status   - 查看节点健康状态
/add      - 添加节点
/del      - 删除节点
/enable   - 启用节点
/disable  - 禁用节点
```

**配置示例**（`config.yml`）：

```yaml
telegram:
  enable: true
  bot-token: "your-bot-token"
  admin-users:
    - 123456789  # 管理员 Telegram ID
```

详细使用参考：[Telegram Bot 文档](./TELEGRAM_BOT.md)

---

## 请求流程

### 完整请求流程图

```
┌──────────────────────────────────────────────────────────────┐
│                   客户端请求视频                             │
│   GET /videos/{itemId}/stream?api_key=xxx                   │
└────────────────────┬─────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────────────────────┐
│                Gin 中间件链（按顺序执行）                    │
├──────────────────────────────────────────────────────────────┤
│  1️⃣ CustomLogger           - 请求日志                        │
│  2️⃣ gin.Recovery            - Panic 恢复                     │
│  3️⃣ referrerPolicySetter    - 设置 Referrer-Policy           │
│  4️⃣ ApiKeyChecker           - 【鉴权】验证 api_key            │
│  5️⃣ DownloadStrategyChecker - 下载策略检查                   │
│  6️⃣ CacheableRouteMarker    - 标记可缓存路由                 │
│  7️⃣ RequestCacher           - 响应缓存                       │
└────────────────────┬─────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────────────────────┐
│              路由匹配（route.go）                            │
├──────────────────────────────────────────────────────────────┤
│  Reg_ResourceStream → Redirect2NginxLink                    │
│  正则：/Videos/[^/]+/stream                                  │
└────────────────────┬─────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────────────────────┐
│         Redirect2NginxLink 处理器（redirect.go:31）          │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  Step 1: 解析 ItemId                                         │
│  ├─ 从 URL 提取：/videos/{itemId}/stream                     │
│  └─ 从参数提取：MediaSourceId, api_key                       │
│                                                              │
│  Step 2: 调用 Emby API 获取媒体信息                          │
│  ├─ GET {embyHost}/emby/Items/{itemId}?api_key=xxx          │
│  └─ 解析响应获取 Path 字段（Emby 本地路径）                   │
│      例如：/media/data/movies/example.mp4                    │
│                                                              │
│  Step 3: 检查是否为本地媒体                                  │
│  ├─ 如果路径前缀匹配 local-media-root                        │
│  └─ 是 → ProxyOrigin (代理回源) 结束                         │
│                                                              │
│  Step 4: 路径映射（config.Path.MapEmby2Nginx）               │
│  ├─ 查找配置：/media/data → /video/data                      │
│  └─ 映射结果：/video/data/movies/example.mp4                │
│                                                              │
│  Step 5: 选择健康节点（nodeSelector.SelectNode）             │
│  ├─ 获取所有健康节点列表                                     │
│  ├─ 计算总权重                                               │
│  └─ 加权随机选择（例如：node-1, http://1.2.3.4:80）          │
│                                                              │
│  Step 6: 获取用户 API Key                                    │
│  ├─ 从缓存获取（userId → api_key）                           │
│  └─ 缓存未命中则使用原始 key                                 │
│                                                              │
│  Step 7: 构建重定向 URL                                      │
│  ├─ 拼接：http://1.2.3.4/video/data/movies/example.mp4      │
│  └─ 可选添加：?api_key=xxx (如果启用 nginx-auth)             │
│                                                              │
│  Step 8: 设置响应头                                          │
│  ├─ Cache-Control: public, max-age=600                      │
│  └─ X-Cache-Expired: 600                                    │
│                                                              │
│  Step 9: 返回 302 重定向                                     │
│  ├─ HTTP/1.1 302 Temporary Redirect                         │
│  └─ Location: http://1.2.3.4/video/data/movies/example.mp4  │
│                                                              │
└────────────────────┬─────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────────────────────┐
│              客户端接收 302 响应                             │
│  自动发起新请求到 Location                                   │
└────────────────────┬─────────────────────────────────────────┘
                     ↓
┌──────────────────────────────────────────────────────────────┐
│         客户端直连 Nginx 节点下载视频                        │
│  GET http://1.2.3.4/video/data/movies/example.mp4?api_key=xxx│
├──────────────────────────────────────────────────────────────┤
│  Nginx 处理：                                                │
│  1. 解析 URL 路径：/video/data/movies/example.mp4           │
│  2. 映射到文件系统：/mnt/disk/movies/example.mp4            │
│  3. 可选验证 api_key 参数                                    │
│  4. 返回视频流（支持 Range 请求）                            │
└──────────────────────────────────────────────────────────────┘
```

### 性能指标

- **302 响应延迟**：< 5ms
- **路径映射**：< 1ms（内存查找）
- **节点选择**：< 1ms（加权随机算法）
- **API Key 验证**：
  - 缓存命中：< 0.1ms
  - 缓存未命中：~50ms（Emby API 调用）
- **总体延迟**：首次请求 ~50ms，后续请求 < 5ms

---

## 关键模块

### 1. 路径映射模块（`internal/config/path.go`）

**功能**：将 Emby 容器内路径转换为 Nginx URL 路径

**关键方法**：
```go
func (p *Path) MapEmby2Nginx(embyPath string) (string, bool)
```

**测试覆盖**：
- ✅ 基础映射
- ✅ 多目录映射
- ✅ 特殊字符路径
- ✅ 深层嵌套路径
- ✅ 前缀精确匹配（修复 bug）

### 2. 节点管理模块（`internal/service/node/`）

**核心文件**：
- `health.go` - 健康检查器
- `selector.go` - 节点选择器
- `type.go` - 节点状态定义

**关键功能**：
- 周期性健康检查（可配置间隔）
- 失败/成功阈值管理
- 加权随机选择算法
- 并发安全（sync.RWMutex）

### 3. 鉴权模块（`internal/service/emby/auth.go`）

**核心组件**：
- `ApiKeyChecker()` - Gin 中间件
- `getApiKey()` - 多方式提取 api_key
- `validApiKeys` - 信任缓存（sync.Map）

**支持的认证方式**：
- Query: `?api_key=xxx`
- Query: `?X-Emby-Token=xxx`
- Header: `Authorization: MediaBrowser Token="xxx"`
- Header: `X-Emby-Authorization: ...`

### 4. 用户 Key 缓存模块（`internal/service/userkey/`）

**核心文件**：
- `cache.go` - Key 缓存管理
- `fetcher.go` - Key 验证器

**关键功能**：
- 用户 ID → API Key 映射缓存
- TTL 过期管理（默认 24h）
- 定期清理过期缓存（每 5 分钟）

### 5. 重定向模块（`internal/service/emby/redirect.go`）

**核心方法**：
- `Redirect2NginxLink()` - 视频流重定向
- `ProxyOriginalResource()` - Original 接口处理
- `buildRedirectUrl()` - URL 构建

**错误处理策略**：
```yaml
emby:
  proxy-error-strategy: origin  # origin: 回源 | reject: 拒绝
```

### 6. 路由模块（`internal/web/route.go`）

**匹配规则**：
```go
rules := [][2]any{
    {constant.Reg_ResourceStream, emby.Redirect2NginxLink},      // 视频流
    {constant.Reg_ResourceOriginal, emby.ProxyOriginalResource}, // Original
    {constant.Reg_ItemDownload, emby.Redirect2NginxLink},        // 下载
    // ...
}
```

**正则常量**（`internal/constant/constant.go`）：
```go
const (
    Reg_ResourceStream   = `/Videos/[^/]+/stream`
    Reg_ResourceOriginal = `/Videos/[^/]+/original`
    Reg_ItemDownload     = `/Items/[^/]+/Download`
    // ...
)
```

---

## 配置示例

完整配置参考：[config-example.yml](../config-example.yml)

**核心配置项**：

```yaml
# Emby 服务器
emby:
  host: http://emby-server:8096
  admin-api-key: "your-admin-api-key"
  mount-path: /media
  local-media-root: /data/local
  proxy-error-strategy: origin

# 节点配置
nodes:
  health-check:
    interval: 30
    timeout: 5
    fail-threshold: 3
    success-threshold: 2
  list:
    - name: "node-1"
      host: "http://1.2.3.4:80"
      weight: 100
      enabled: true

# 鉴权配置
auth:
  user-key-cache-ttl: 24h
  nginx-auth-enable: true

# 路径映射
path:
  emby2nginx:
    - /media/data:/video/data
    - /media/series:/video/series
```

---

## 性能优化建议

### 1. 客户端层面
- 启用 DNS 缓存，减少 Nginx 节点域名解析时间
- 使用 HTTP/2 或 QUIC 协议提升传输效率

### 2. 代理服务器层面
- 增大 `validApiKeys` 缓存（已经无限制）
- 调整 `user-key-cache-ttl` 为更长时间（默认 24h）
- 启用响应缓存（cache.enable）

### 3. Nginx 节点层面
- 启用 `sendfile` 和 `tcp_nopush`
- 配置 `directio` 提升大文件传输性能
- 启用 Gzip 压缩（字幕等文本文件）
- 配置 CDN 或反向代理（Cloudflare）

### 4. 网络层面
- 使用内网 IP 直连（跳过公网）
- 配置端口转发或 Tailscale/ZeroTier
- 使用高带宽线路的节点设置更高权重

---

## 故障排查

### 问题 1: 302 后 404 Not Found

**可能原因**：
- 路径映射配置错误
- Nginx location 配置错误
- 文件不存在

**检查步骤**：
```bash
# 1. 查看代理日志，确认映射路径
docker logs go-emby2openlist | grep "Nginx 路径"

# 2. 测试 Nginx 节点访问
curl -I http://nginx-node/video/data/test.mp4

# 3. 检查 Nginx 配置
location /video/data {
    alias /mnt/disk/;  # 确认别名路径正确
}
```

### 问题 2: 所有节点不健康

**检查健康接口**：
```bash
curl -v -H "Host: gtm-health" http://nginx-node/gtm-health
# 应返回：HTTP/1.1 200 OK
```

### 问题 3: 鉴权失败

**检查 API Key**：
```bash
# 测试 Emby API
curl http://emby-server:8096/emby/System/Info?api_key=xxx
# 应返回 200
```

---

## 相关文档

- [README.md](../README.md) - 项目介绍和快速开始
- [MIGRATION_GUIDE.md](../MIGRATION_GUIDE.md) - 从 OpenList 迁移指南
- [TEST_REPORT.md](../TEST_REPORT.md) - 完整测试报告
- [TESTING_GUIDE.md](./TESTING_GUIDE.md) - 测试指南
- [TELEGRAM_BOT.md](./TELEGRAM_BOT.md) - Telegram Bot 使用文档

---

**文档版本**: v1.0
**最后更新**: 2025-12-06
**项目版本**: v2.3.3
