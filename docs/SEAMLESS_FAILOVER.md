# 无缝故障转移机制

## 📋 概述

本文档介绍 go-emby2openlist v2.6.0 引入的**无缝故障转移**机制，实现节点故障时视频播放的自动切换，无需用户手动干预。

---

## 🎯 设计目标

1. **无感知切换**：节点故障时，客户端自动切换到健康节点，用户无需手动操作
2. **保持播放位置**：利用 HTTP 307 重定向保留 Range 请求，避免重新缓冲
3. **防止死循环**：通过重试计数器防止无限重定向
4. **快速检测**：每次 Range 请求（2-10秒）都检查节点健康状态

---

## 🔄 工作流程

### 正常播放流程

```
1. 客户端请求视频
   ↓
2. Go 应用选择健康节点 A
   ↓
3. 302 重定向到节点 A 的 /video/data/...?api_key=xxx
   ↓
4. 节点 A 代理到鉴权服务
   ↓
5. 鉴权服务验证 api_key，生成 token
   ↓
6. 302 重定向到 /internal/data/...?token=xxx&expires=xxx&uid=xxx
   ↓
7. Nginx auth_request 调用 /api/verify-token
   ↓
8. 验证 token ✅，创建播放会话（5分钟）
   ↓
9. 返回视频数据
   ↓
10. 客户端持续发送 Range 请求（每 2-10 秒）
    ↓
11. 每次 Range 请求都检查节点健康 + 续期会话
```

### 故障转移流程（新增）

```
1. 客户端发送 Range 请求到节点 A
   ↓
2. Nginx auth_request 调用 /api/verify-token
   ↓
3. Go 应用检查节点 A 健康状态 ❌
   ↓
4. 选择新的健康节点 B
   ↓
5. 返回 307 临时重定向到节点 B
   Location: http://node-b:7777/video/data/...?api_key=xxx&_retry=1
   ↓
6. 客户端自动跟随 307 重定向（保留原始 Range 头）
   ↓
7. 请求到达节点 B，重新鉴权
   ↓
8. 生成新的 token，302 重定向到节点 B 的 /internal/
   ↓
9. 验证 token ✅，创建新会话
   ↓
10. 返回视频数据（从 Range 指定的位置）
    ↓
11. 播放继续 ✅
```

---

## 🔑 核心实现

### 1. 节点健康检查（每次 Range 请求）

```go
// internal/service/videoauth/auth.go:148-174
if s.healthChecker != nil && s.nodeSelector != nil {
    requestHost := c.Request.Host  // 当前节点地址
    isHealthy := s.isNodeHealthy(requestHost)

    if !isHealthy {
        // 选择新节点
        newNode := s.nodeSelector.SelectNode()

        // 生成新的鉴权 URL
        newRedirectURL := s.buildFailoverURL(newNode.Host, path, apiKey, retryCount+1)

        // 返回 307 临时重定向
        c.Redirect(http.StatusTemporaryRedirect, newRedirectURL)
        return
    }
}
```

### 2. 故障转移 URL 生成

```go
// internal/service/videoauth/auth.go:335-358
func (s *VideoAuthService) buildFailoverURL(nodeHost, internalPath, apiKey string, retryCount int) string {
    // 将 /internal/dataX/... 转换为 /video/dataX/...
    publicPath := strings.Replace(internalPath, "/internal/", "/video/", 1)

    u, _ := url.Parse(nodeHost)
    u.Path = publicPath

    // 添加 api_key 和重试计数器
    q := u.Query()
    q.Set("api_key", apiKey)
    if retryCount > 0 {
        q.Set("_retry", fmt.Sprintf("%d", retryCount))
    }
    u.RawQuery = q.Encode()

    return u.String()
}
```

### 3. 重试次数限制（防止死循环）

```go
// internal/service/videoauth/auth.go:114-124
retryCount := 0
if retryStr := c.Query("_retry"); retryStr != "" {
    fmt.Sscanf(retryStr, "%d", &retryCount)
}

const maxRetries = 3
if retryCount >= maxRetries {
    logs.Error("[TokenVerify] 故障转移重试次数超限 (%d 次)", retryCount)
    c.Status(http.StatusServiceUnavailable)
    return
}
```

---

## 🆚 新旧机制对比

| 特性 | 旧机制（403 拒绝） | 新机制（307 重定向） |
|------|-------------------|---------------------|
| **用户体验** | 播放中断，需要手动刷新 | 自动切换，无感知 |
| **播放位置** | 可能丢失，重新缓冲 | 保留，继续播放 |
| **Range 请求** | 丢失，需要重新发送 | 保留，客户端自动携带 |
| **切换延迟** | 5-10 秒（取决于播放器重试） | 1-2 秒（HTTP 重定向） |
| **故障检测** | 每次 Range 请求（2-10秒） | 每次 Range 请求（2-10秒） |
| **防死循环** | 会话删除后自然停止 | 重试计数器（最多 3 次） |

---

## 📊 故障转移时序图

```
时间  客户端              节点 A (故障)         节点 B (健康)        Go 应用
────────────────────────────────────────────────────────────────────────────
00:00 播放中...           ✅ 正常
      ↓
00:05 Range: 1000-2000 ──→ auth_request ───→                      检查健康 ✅
      ←──────────────────  视频数据 ──────                         续期会话
      ↓
      ⚠️ 节点 A 故障
      ↓
00:10 Range: 2001-3000 ──→ auth_request ───→                      检查健康 ❌
      ←──────────────────  307 重定向 ────                         选择节点 B
      |                     Location: node-b/video/...?_retry=1
      ↓
00:10 Range: 2001-3000 ─────────────────────→ 代理鉴权 ─────→     验证 api_key
      ←─────────────────────────────────────  302 重定向 ────     生成 token
      |                                       Location: /internal/...
      ↓
00:10 Range: 2001-3000 ─────────────────────→ auth_request ──→   验证 token ✅
      ←─────────────────────────────────────  视频数据 ─────     创建会话
      ↓
00:10 ✅ 播放恢复（无中断）
```

---

## 🚀 部署步骤

### 1. 编译新版本

```bash
cd /path/to/go-emby2openlist
go build -o go-emby2openlist
```

### 2. 停止旧版本

```bash
docker stop go-emby2openlist
docker rm go-emby2openlist
```

### 3. 构建新镜像（如果使用 Docker）

```bash
docker build -t go-emby2openlist:v2.6.0 .
```

### 4. 启动新版本

```bash
docker run -d \
  --name go-emby2openlist \
  -p 8095:8095 \
  -p 8097:8097 \
  -v /usr/local/go-emby2openlist/config:/app/config \
  go-emby2openlist:v2.6.0
```

### 5. 验证部署

```bash
# 查看日志，确认节点选择器初始化
docker logs go-emby2openlist | grep "节点选择器"

# 测试播放视频，观察是否正常
```

---

## 🧪 测试故障转移

### 手动测试步骤

1. **开始播放视频**
   ```bash
   # 观察日志，确认使用的节点
   docker logs -f go-emby2openlist | grep "选择节点"
   ```

2. **模拟节点故障**（有以下几种方式）

   **方式 1：停止 Nginx**
   ```bash
   # 在节点 A 上
   systemctl stop nginx
   ```

   **方式 2：修改节点健康检查端点**
   ```bash
   # 在节点 A 上，临时返回 503
   echo "return 503;" > /tmp/maintenance.conf
   nginx -s reload
   ```

   **方式 3：使用 iptables 阻止健康检查**
   ```bash
   # 阻止健康检查端口
   iptables -A INPUT -p tcp --dport 80 -j DROP
   ```

3. **观察故障转移**
   ```bash
   # 在 Go 应用日志中查看
   docker logs -f go-emby2openlist | grep "故障转移"

   # 应该看到类似：
   # [WARN] 节点不健康，执行故障转移: 8.138.199.183:46621
   # [INFO] 故障转移到新节点: node-b (47.113.178.192:46621), 重试次数: 1
   ```

4. **验证播放继续**
   - 播放器应该自动恢复，无需用户操作
   - 播放位置保持不变
   - 可能有 1-2 秒的短暂缓冲

---

## 📈 监控指标

### 关键日志

1. **节点健康检查**
   ```
   [WARN] 节点不健康: node-a (8.138.199.183:46621)
   ```

2. **故障转移触发**
   ```
   [WARN] 节点不健康，执行故障转移: 8.138.199.183:46621, 路径: /internal/data/Movie/xxx.mkv
   ```

3. **新节点选择**
   ```
   [INFO] 故障转移到新节点: node-b (47.113.178.192:46621), 重试次数: 1
   ```

4. **重试超限**（异常情况）
   ```
   [ERROR] 故障转移重试次数超限 (3 次)，拒绝访问
   ```

### Prometheus 指标建议（TODO）

```
# 故障转移次数
go_emby2openlist_failover_total{from_node="node-a", to_node="node-b"}

# 故障转移延迟
go_emby2openlist_failover_duration_seconds{from_node="node-a", to_node="node-b"}

# 重试超限次数
go_emby2openlist_failover_retry_limit_exceeded_total
```

---

## ⚠️ 注意事项

### 1. 客户端兼容性

**支持的客户端**：
- ✅ 现代浏览器（Chrome, Firefox, Safari, Edge）
- ✅ Emby 官方客户端
- ✅ Jellyfin 客户端
- ✅ VLC、PotPlayer 等桌面播放器
- ✅ iOS Safari、Android Chrome

**不支持的客户端**：
- ❌ 不支持 HTTP 307 重定向的古老播放器
- ❌ 不自动跟随重定向的自定义客户端

### 2. 性能影响

- **正常情况**：无性能影响，只是增加了健康检查判断
- **故障转移**：增加 1-2 秒延迟（重定向 + 重新鉴权）
- **网络开销**：每次故障转移额外 2 个 HTTP 请求

### 3. 故障场景

**可以处理的故障**：
- ✅ 节点 Nginx 崩溃
- ✅ 节点网络故障
- ✅ 节点磁盘满
- ✅ 节点 CPU/内存耗尽

**无法处理的故障**：
- ❌ 所有节点同时故障
- ❌ 源视频文件损坏
- ❌ Go 应用本身故障
- ❌ 健康检查服务故障

### 4. 重试次数限制

- 默认最多重试 3 次
- 超过 3 次返回 503 Service Unavailable
- 可在代码中调整 `maxRetries` 常量

---

## 🔧 配置选项

当前版本无需额外配置，故障转移机制默认启用。

未来版本可能添加以下配置：

```yaml
failover:
  enabled: true               # 是否启用故障转移
  max_retries: 3             # 最大重试次数
  retry_timeout: 5s          # 重试超时时间
  health_check_interval: 10s # 健康检查间隔
```

---

## 🐛 故障排查

### 问题 1：故障转移不生效

**症状**：节点故障后，播放器直接报错，没有自动切换

**排查步骤**：
```bash
# 1. 检查是否启用了节点健康检查
docker logs go-emby2openlist | grep "健康检查"

# 2. 检查节点选择器是否初始化
docker logs go-emby2openlist | grep "节点选择器"

# 3. 检查是否有其他健康节点
curl http://localhost:8095/api/nodes

# 4. 查看详细日志
docker logs -f go-emby2openlist
```

### 问题 2：无限重定向循环

**症状**：播放器一直缓冲，日志显示不断重试

**原因**：所有节点都不健康，或健康检查逻辑错误

**解决方法**：
```bash
# 检查所有节点健康状态
curl http://localhost:8095/api/nodes

# 检查重试次数
docker logs go-emby2openlist | grep "_retry"
```

### 问题 3：切换后播放位置丢失

**症状**：故障转移后，视频从头开始播放

**原因**：客户端不支持 307 重定向，或 Range 头丢失

**解决方法**：
- 更换支持 HTTP 307 的播放器
- 检查 Nginx 日志，确认 Range 头是否传递

---

## 📚 相关文档

- [节点健康检查机制](./HEALTH_CHECK.md)
- [视频鉴权流程](./VIDEO_AUTH.md)
- [故障回退机制](./FALLBACK_MECHANISM.md)
- [Nginx 404 故障排查](./NGINX_404_TROUBLESHOOTING.md)

---

## 📝 更新日志

### v2.6.0 (2025-12-07)

- ✨ 新增无缝故障转移机制（HTTP 307 重定向）
- 🔒 新增故障转移重试次数限制（防止死循环）
- 📊 新增故障转移日志和监控
- 🐛 修复 data_2 和 data_3_oumeiguochan 路径不匹配问题
- 🐛 修复 Nginx rewrite 规则顺序导致的 404 错误

---

## 🙏 反馈

如有问题或建议，请提交 Issue：
https://github.com/AmbitiousJun/go-emby2openlist/issues
