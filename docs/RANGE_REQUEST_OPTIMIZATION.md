# Range 请求优化方案

## 📊 问题分析

### 现象
用户报告在 v2.5.1 之后，视频播放不再使用小分片请求，而是出现巨大的单个 Range 请求：

```
Content-Range: bytes 557056-102682361360/102682361361
Content-Length: 102681804305  (约 95.6 GB!!!)
```

### 根本原因

**这不是服务器配置的问题，而是客户端行为的变化：**

1. **Nginx 配置未变化** - v2.5.0 和 v2.5.1 的 Nginx 配置完全相同
2. **客户端请求策略改变** - Emby 播放器发送了 `Range: bytes=557056-`（开放式 Range）
3. **Nginx 正确响应** - 返回了从 557056 到文件末尾的所有内容

**可能触发原因：**
- 播放器检测到网络稳定，采用"贪婪缓冲"策略
- 浏览器或 Emby 客户端更新，缓冲策略变化
- 直接播放原始 MKV 文件（未启用 HLS 转码）

---

## ⚠️ 巨大 Range 请求的问题

### 1. 网络中断风险高
- 95.6 GB 的单个请求如果中断，需要重新下载
- 浪费带宽和时间

### 2. 单个连接占用过多资源
- Nginx 需要维护长时间连接
- 缓冲区占用更多内存
- 其他用户可能受影响

### 3. 播放器缓冲策略失效
- 无法灵活控制下载进度
- 可能下载大量用户不会观看的内容

### 4. 故障转移困难
- 一旦开始下载，无法切换到其他节点
- 节点故障会导致播放中断

---

## ✅ 解决方案

### 方案 1：Nginx 限速（推荐 ✅）

通过限制下载速率，强制播放器分批请求：

```nginx
location ~ ^/internal/(data|data1|...)$ {
    # ... 现有配置 ...

    # 限制下载速率
    # 前 50 MB 不限速（快速启动播放）
    limit_rate_after 50m;
    # 之后限制为 50 MB/s（足够 4K 播放）
    limit_rate 50m;

    # 连接超时设置
    send_timeout 300s;  # 5 分钟无数据传输则断开
    keepalive_timeout 300s;
}
```

**效果**：
- ✅ 快速启动播放（前 50 MB 全速）
- ✅ 防止单个连接占用过多带宽
- ✅ 播放器会因为限速而重新发起新的 Range 请求
- ✅ 多用户并发时更公平

**性能计算**：
- 50 MB/s = 400 Mbps（足够播放 4K 视频）
- 10 个并发用户 = 500 MB/s（4 Gbps）

---

### 方案 2：调整播放器设置

在 Emby 中配置播放器行为：

**步骤**：
1. Emby 设置 → 播放
2. 选择播放器：Web 播放器
3. 缓冲策略：选择"自适应"或"较小缓冲"
4. 预加载设置：调整预加载大小

**效果**：
- 播放器会发送更小的 Range 请求
- 但需要每个用户单独配置

---

### 方案 3：启用 HLS 转码（长期方案）

让 Emby 自动将大文件转换为 HLS 分片：

**配置**：
1. Emby 设置 → 转码
2. 启用 HLS 转码
3. 设置分片大小（2-10 秒）

**优点**：
- ✅ 自动分割为小片段
- ✅ 每个片段独立鉴权
- ✅ 支持自适应码率
- ✅ 降低网络中断风险

**缺点**：
- ❌ 增加 CPU 负载（转码）
- ❌ 需要存储空间（转码缓存）
- ❌ 启动延迟增加（首次转码）

---

## 🔄 故障转移机制

### 当前机制（v2.5.1）

#### 初始重定向阶段 ✅
```go
// 选择健康节点
selectedNode := nodeSelector.SelectNode()
if selectedNode == nil {
    // 没有健康节点 → 回源到 Emby
    ProxyOrigin(c)
    return
}

// 返回 302 重定向到健康节点
c.Redirect(http.StatusTemporaryRedirect, nodeURL)
```

**效果**：
- ✅ 只会重定向到健康节点
- ✅ 如果所有节点不健康，回源到 Emby

#### 播放过程中 ❌
- 302 重定向完成后，所有 Range 请求直接发送到该节点
- **如果该节点在播放过程中变得不健康，无法自动切换**
- 用户需要手动刷新或重新加载视频

---

### 改进方案：播放过程中的自动故障转移

#### 方案 A：Nginx upstream 健康检查

使用 Nginx Plus 或开源模块 `nginx_upstream_check_module`：

```nginx
upstream video_nodes {
    server 192.168.1.10:80 max_fails=3 fail_timeout=30s;
    server 192.168.1.11:80 max_fails=3 fail_timeout=30s;

    # 健康检查（需要 nginx_upstream_check_module）
    check interval=3000 rise=2 fall=3 timeout=1000 type=http;
    check_http_send "GET /gtm-health HTTP/1.0\r\n\r\n";
    check_http_expect_alive http_2xx http_3xx;
}

location ~ ^/internal/ {
    # 使用 upstream，自动故障转移
    proxy_pass http://video_nodes;
    proxy_next_upstream error timeout http_502 http_503 http_504;
}
```

**效果**：
- ✅ 自动检测节点健康状态
- ✅ 播放过程中自动切换到健康节点
- ✅ 用户无感知

**缺点**：
- ❌ 需要重新设计架构（从直接文件服务改为 proxy）
- ❌ 增加网络跳转（性能损失）

---

#### 方案 B：客户端侧重试机制

在客户端（Emby 播放器）配置重试策略：

**步骤**：
1. Emby 设置 → 播放
2. 网络设置 → 启用自动重试
3. 重试次数：3 次
4. 重试延迟：5 秒

**效果**：
- ✅ Range 请求失败后自动重试
- ✅ 可能触发重新 302 重定向（选择新节点）

**缺点**：
- ❌ 依赖客户端支持
- ❌ 可能会重新请求相同的不健康节点

---

#### 方案 C：Token 续期时检测节点健康（推荐 ✅）

修改 v2.5.1 的 Token 自动续期逻辑，在续期时检测节点健康：

```go
func (s *VideoAuthService) HandleVerifyToken(c *gin.Context) {
    // ... 现有验证逻辑 ...

    // 检查当前节点是否健康
    currentNode := s.findNodeByHost(c.Request.Host)
    if currentNode != nil && !s.healthChecker.IsHealthy(currentNode) {
        // 节点不健康 → 拒绝续期 → 强制客户端重新 302 重定向
        logs.Warn("[TokenVerify] 节点不健康，拒绝续期: %s", currentNode.Name)
        c.Status(http.StatusForbidden)
        return
    }

    // 节点健康 → 正常续期
    s.playingSessions.Set(sessionKey, ...)
    c.Status(http.StatusOK)
}
```

**效果**：
- ✅ 播放过程中检测节点健康
- ✅ 节点不健康时，拒绝 auth_request → 403 错误
- ✅ Emby 播放器收到 403 后，重新请求 → 触发新的 302 重定向
- ✅ 自动切换到健康节点

**优点**：
- 利用现有的 Token 续期机制
- 无需修改 Nginx 配置
- 对用户透明（短暂缓冲后恢复）

**缺点**：
- 需要修改 Go 代码
- 短暂的播放中断（重新 302 重定向）

---

## 🚀 推荐部署方案

### 立即部署（Nginx 限速）

```bash
# 1. 更新 Nginx 配置
cd /usr/local/go-emby2openlist
git pull && git checkout v2.5.1

# 2. 复制最新配置
cp nginx/video-gateway-URL-DECODE-FIX.conf /etc/nginx/sites-available/video-gateway.conf

# 3. 测试配置
nginx -t

# 4. 重新加载
nginx -s reload
```

**验证**：
```bash
# 查看 Nginx 配置是否包含限速
cat /etc/nginx/sites-available/video-gateway.conf | grep limit_rate
# 应输出:
# limit_rate_after 50m;
# limit_rate 50m;
```

---

### 长期优化（故障转移）

**选项 1**：等待我实现方案 C（Token 续期时检测节点健康）

**选项 2**：手动配置 Nginx upstream 健康检查（复杂，不推荐）

**选项 3**：启用 Emby HLS 转码（简单，但增加 CPU 负载）

---

## 📊 性能影响分析

### 限速前（无限制）
```
单个用户: 可能占用全部带宽（1 Gbps+）
95.6 GB 文件: 传输时间 ~13 分钟（@ 100 MB/s）
10 个并发用户: 可能导致带宽耗尽
```

### 限速后（50 MB/s）
```
单个用户: 最大 50 MB/s（400 Mbps）
4K 视频需求: 约 25 Mbps（完全足够）
95.6 GB 文件: 传输时间 ~32 分钟
10 个并发用户: 总带宽 500 MB/s（4 Gbps）
```

**结论**：
- ✅ 单用户体验不受影响（50 MB/s 远超 4K 需求）
- ✅ 多用户并发更公平
- ✅ 避免单个连接占用过多资源

---

## 🔍 故障排查

### 问题 1：仍然出现巨大 Range 请求

**排查**：
```bash
# 检查 Nginx 配置是否生效
cat /etc/nginx/sites-available/video-gateway.conf | grep -A 3 "limit_rate"

# 查看 Nginx 日志
tail -f /var/log/nginx/video_internal_access.log
```

**解决**：
```bash
# 确保配置重新加载
nginx -s reload

# 如果不生效，重启 Nginx
systemctl restart nginx
```

---

### 问题 2：播放过程中节点故障，视频卡住

**现象**：
- 视频播放一段时间后突然卡住
- 无限加载，不恢复

**临时解决**：
- 用户手动刷新页面
- 或跳转到其他时间点（触发新的 302 重定向）

**永久解决**：
- 等待方案 C 实现（Token 续期时检测节点健康）
- 或启用 Emby 自动重试功能

---

## 📝 总结

| 问题 | 当前状态 | 解决方案 | 优先级 |
|------|----------|----------|--------|
| 巨大 Range 请求 | ❌ 存在 | Nginx 限速 | **高** |
| 单连接占用过多带宽 | ❌ 存在 | Nginx 限速 | **高** |
| 初始重定向选择不健康节点 | ✅ 已解决 | nodeSelector.SelectNode() | - |
| 播放中节点故障无法转移 | ❌ 未实现 | Token 续期时检测健康 | 中 |

**立即行动**：
1. ✅ 部署 Nginx 限速配置
2. ⏳ 监控效果，观察 Range 请求大小
3. ⏳ 评估是否需要实现方案 C（故障转移）
