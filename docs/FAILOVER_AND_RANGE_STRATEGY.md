# 故障转移与 Range 分片策略详解

## 📋 目录

1. [无缝故障转移机制](#一无缝故障转移机制)
2. [Range 分片策略](#二range-分片策略)
3. [两者如何协同工作](#三两者如何协同工作)

---

## 一、无缝故障转移机制

### 🎯 设计目标

当节点故障时，**用户无感知切换**到健康节点，保持播放连续性。

### 🔄 完整流程

#### 1. 正常播放阶段

```
用户 → 节点 A → 播放中
  ↓
每 2-10 秒发送 Range 请求
  ↓
Nginx auth_request → Go 应用验证 token
  ↓
检查节点 A 健康状态 ✅
  ↓
续期会话（延长 5 分钟）
  ↓
返回视频数据 → 继续播放
```

#### 2. 故障转移阶段

```
用户发送 Range 请求 → 节点 A
  ↓
Nginx auth_request → Go 应用
  ↓
检查节点 A 健康状态 ❌ 不健康！
  ↓
选择新节点 B（从节点池）
  ↓
生成新鉴权 URL：
http://node-b:7777/video/data/Movie/xxx.mkv?api_key=xxx&_retry=1
  ↓
返回 307 临时重定向
  ↓
客户端自动跟随重定向（保留 Range 头）
  ↓
请求到达节点 B
  ↓
重新鉴权（生成新 token）
  ↓
验证通过 → 返回视频数据
  ↓
播放恢复 ✅（从原位置继续）
```

### 🔑 核心代码

**检查节点健康并执行故障转移**：

```go
// internal/service/videoauth/auth.go:159-186

// 检查当前节点是否健康
if s.healthChecker != nil && s.nodeSelector != nil {
    requestHost := c.Request.Host  // 例如：8.138.199.183:46621
    isHealthy := s.isNodeHealthy(requestHost)
    
    if !isHealthy {
        // 🚨 节点不健康 → 执行故障转移
        logs.Warn("[TokenVerify] 节点不健康，执行故障转移: %s", requestHost)
        
        // 选择新的健康节点
        newNode := s.nodeSelector.SelectNode()
        if newNode == nil {
            // 没有可用节点 → 返回 503
            c.Status(http.StatusServiceUnavailable)
            return
        }
        
        // 生成新的鉴权 URL（指向新节点）
        newRedirectURL := s.buildFailoverURL(newNode.Host, path, apiKey, retryCount+1)
        
        // 返回 307 临时重定向（保留原始 Range 头）
        c.Redirect(http.StatusTemporaryRedirect, newRedirectURL)
        return
    }
}
```

**生成故障转移 URL**：

```go
// internal/service/videoauth/auth.go:335-358

func (s *VideoAuthService) buildFailoverURL(nodeHost, internalPath, apiKey string, retryCount int) string {
    // 1. 将 /internal/dataX/... 转换为 /video/dataX/...
    publicPath := strings.Replace(internalPath, "/internal/", "/video/", 1)
    
    // 2. 解析节点地址
    u, _ := url.Parse(nodeHost)
    u.Path = publicPath
    
    // 3. 添加参数
    q := u.Query()
    q.Set("api_key", apiKey)         // 用户 API Key
    q.Set("_retry", fmt.Sprintf("%d", retryCount))  // 重试计数
    u.RawQuery = q.Encode()
    
    return u.String()
}
```

### 📊 时序图（节点 A → 节点 B）

```
时间  用户              节点 A (故障)    节点 B (健康)    Go 应用        健康检查器
────────────────────────────────────────────────────────────────────────────────
00:00 播放中...         ✅ 正常
      │
00:05 Range 请求 ──→   auth_request ─→                  验证 token ✅
      ←─────────────   视频数据 ←────                   续期会话
      │
      ⚠️ 节点 A 故障                                                └─ 标记不健康
      │
00:10 Range 请求 ──→   auth_request ─→                  检查健康 ❌
      ←─────────────   307 重定向 ←──                   选择节点 B
      │                Location: node-b/video/...?_retry=1
      │
00:10 跟随重定向 ──────────────────→  代理鉴权 ──→      验证 api_key
      ←──────────────────────────────  302 重定向 ←─    生成新 token
      │                               Location: /internal/...?token=new
      │
00:10 请求 /internal ──────────────→  auth_request ─→   验证新 token ✅
      ←──────────────────────────────  视频数据 ←────   创建新会话
      │
00:10 ✅ 播放恢复（无中断，从原位置继续）
```

### 🛡️ 防死循环机制

**问题**：如果所有节点都不健康，可能导致无限重定向循环。

**解决方案**：重试计数器

```go
// internal/service/videoauth/auth.go:114-124

// 检查重试次数（从 URL 参数 _retry 获取）
retryCount := 0
if retryStr := c.Query("_retry"); retryStr != "" {
    fmt.Sscanf(retryStr, "%d", &retryCount)
}

const maxRetries = 3  // 最多重试 3 次
if retryCount >= maxRetries {
    logs.Error("[TokenVerify] 故障转移重试次数超限 (%d 次)", retryCount)
    c.Status(http.StatusServiceUnavailable)  // 返回 503
    return
}
```

**重试流程**：

```
第 1 次故障转移：_retry=1 → 节点 B
第 2 次故障转移：_retry=2 → 节点 C
第 3 次故障转移：_retry=3 → 节点 D
第 4 次请求：    _retry=4 → ❌ 超过限制，返回 503
```

### ⚡ 性能指标

| 指标 | 数值 | 说明 |
|------|------|------|
| **故障检测延迟** | 2-10 秒 | 每次 Range 请求检查一次 |
| **故障转移延迟** | 1-2 秒 | HTTP 重定向 + 重新鉴权 |
| **用户体验** | 短暂缓冲 | 可能看到 1-2 秒缓冲 |
| **播放位置** | 保持 | Range 头自动携带 |
| **会话状态** | 重建 | 在新节点创建新会话 |

---

## 二、Range 分片策略

### 🎯 策略概述

系统**不主动控制**分片大小，而是通过**速率限制 + 超时控制**来管理资源。

### 📐 分片由谁决定？

**客户端决定 Range 大小**，服务端只控制传输速度：

```
客户端请求:
GET /internal/data/Movie/xxx.mkv
Range: bytes=0-10485760          ← 客户端要求 10MB

Nginx 响应:
HTTP/1.1 206 Partial Content
Content-Range: bytes 0-10485760/4173009158
Content-Length: 10485760

实际传输:
- 前 50MB: 不限速（快速启动）
- 之后: 50MB/s 限速
- 10MB 数据实际传输时间: 0.2 秒
```

### 🔍 实际 Range 请求分析

从 Nginx 日志可以看到真实的分片模式：

```bash
# 典型的播放会话日志

206 65536         # 64KB   - 初始探测请求
206 1209080       # 1.1MB  - 音频轨道/字幕
206 40508898      # 38MB   - 正常播放缓冲
206 26627222      # 25MB   - 正常播放
206 2368287638    # 2.2GB  - 用户拖动进度条（快进到后面）
206 142662373     # 136MB  - 恢复正常播放
```

**分片大小分布**：
- **小分片**（< 1MB）：初始探测、音频轨道、字幕
- **中分片**（10-100MB）：正常播放，最常见
- **大分片**（> 1GB）：用户拖动进度条、快进、跳转

### ⚙️ 服务端控制策略

#### 策略 1：速率限制（已实现）

```nginx
# nginx/video-gateway-URL-DECODE-FIX.conf:146-147

limit_rate_after 50m;  # 前 50MB 不限速
limit_rate 50m;        # 之后限速为 50MB/s
```

**效果演示**：

```
客户端请求 2.2GB 数据（拖动进度条）:
Range: bytes=2000000000-4200000000

Nginx 响应:
- 前 50MB:   不限速  → 约 1 秒（假设带宽 100MB/s）
- 剩余 2150MB: 限速 50MB/s → 约 43 秒

总传输时间: 44 秒
实际带宽占用: 最高 100MB/s → 稳定 50MB/s
```

**优点**：
- ✅ 不影响播放启动速度（前 50MB 快速加载）
- ✅ 控制长期带宽占用（50MB/s）
- ✅ 公平分配带宽（多用户场景）
- ✅ 即使请求很大，传输也受控

#### 策略 2：连接超时（已实现）

```nginx
# nginx/video-gateway-URL-DECODE-FIX.conf:150-151

send_timeout 300s;      # 5 分钟无数据传输则断开
keepalive_timeout 300s; # 5 分钟无活动断开连接
```

**效果**：
- ✅ 自动清理长时间无活动的连接
- ✅ 释放服务器资源
- ⚠️ 用户暂停 > 5 分钟会断开（需要重新连接）

#### 策略 3：并发连接限制（可选）

```nginx
# 需要在 nginx.conf 的 http 块中添加
limit_conn_zone $binary_remote_addr zone=perip:10m;

# 在 server 或 location 中使用
limit_conn perip 5;  # 单个 IP 最多 5 个并发连接
```

**效果**：
- ✅ 防止单用户打开过多连接
- ✅ 保护服务器资源
- ⚠️ 可能影响多设备同时播放

### 📊 不同分片大小的处理

| Range 大小 | 典型场景 | 传输时间 (50MB/s限速) | 说明 |
|-----------|---------|---------------------|------|
| 64 KB | 初始探测 | < 0.01 秒 | 瞬间完成 |
| 1-10 MB | 音频轨道 | 0.02-0.2 秒 | 几乎瞬间 |
| 10-50 MB | 正常播放 | 0.2-1 秒 | 不限速，快速 |
| 50-100 MB | 正常播放 | 1-2 秒 | 部分限速 |
| 100-500 MB | 快进/缓存 | 2-10 秒 | 全程限速 |
| 1-5 GB | 进度条拖动 | 20-100 秒 | 全程限速 |

### 🎬 实际播放场景分析

#### 场景 1：正常播放（最常见）

```
00:00  初始请求: Range: 0-65535 (64KB)
       → 瞬间返回，探测文件格式

00:00  音频轨道: Range: 65536-1274615 (1.1MB)
       → 0.02 秒，加载音频

00:01  视频数据: Range: 1274616-41783514 (38MB)
       → 0.8 秒，不限速快速加载

00:02  ✅ 播放开始

持续播放:
       每 5-10 秒请求: 25-50MB
       → 0.5-1 秒/次，流畅播放
```

#### 场景 2：用户拖动进度条

```
用户拖动到 1 小时 30 分钟处（文件的 70% 位置）

客户端请求:
Range: bytes=2900000000-4200000000 (1.3GB)

服务端响应:
- 前 50MB:   快速传输（约 1 秒）
- 剩余 1250MB: 50MB/s 限速（约 25 秒）

用户体验:
- 初始缓冲快（1 秒后开始播放）
- 后续持续加载（25 秒内完成全部缓存）
- 播放流畅（因为边播边传）
```

#### 场景 3：多用户并发

```
用户 A: 正常播放，50MB/s
用户 B: 正常播放，50MB/s
用户 C: 拖动进度条，50MB/s

总带宽占用: 150MB/s（可控）

如果不限速:
用户 C 可能占用 200MB/s → 影响 A 和 B
```

---

## 三、两者如何协同工作

### 🔗 协同机制

故障转移和 Range 分片**完美兼容**，互不影响：

#### 1. 正常播放时

```
用户 → Range 请求 → 节点 A
  ↓
验证 token ✅
  ↓
检查节点健康 ✅
  ↓
续期会话
  ↓
返回 Range 数据（受速率限制）
  ↓
客户端接收数据，继续播放
```

#### 2. 故障转移时

```
用户 → Range: bytes=1000000-2000000 → 节点 A
  ↓
验证 token ✅
  ↓
检查节点健康 ❌
  ↓
307 重定向到节点 B
  ↓
客户端跟随重定向（保留 Range 头）
  ↓
Range: bytes=1000000-2000000 → 节点 B
  ↓
重新鉴权，生成新 token
  ↓
验证新 token ✅
  ↓
返回 Range 数据（从 1000000 开始）
  ↓
播放恢复（无缝，保持位置）
```

### 🎯 关键点

1. **Range 头自动携带**
   - HTTP 307 重定向保留原始请求头
   - 客户端不需要重新计算播放位置

2. **速率限制仍然生效**
   - 故障转移到新节点后
   - 新节点的速率限制立即生效
   - 避免瞬间带宽冲击

3. **会话独立管理**
   - 每个节点有独立的播放会话
   - 故障转移后创建新会话
   - 新会话同样支持续期

### 📊 完整示例：故障转移 + Range 请求

```
时间  动作                Range 请求              节点    传输速度
────────────────────────────────────────────────────────────────
00:00 开始播放            0-40MB                 A       不限速
00:01 正常播放            40-90MB                A       50MB/s
00:06 正常播放            90-140MB               A       50MB/s

⚠️  节点 A 故障

00:11 Range 请求          140-190MB              A       -
      → 检测到故障
      → 307 重定向到 B
      
00:11 跟随重定向          140-190MB (保留!)      B       -
      → 重新鉴权
      → 生成新 token
      
00:12 恢复播放            140-190MB              B       50MB/s ✅
00:17 继续播放            190-240MB              B       50MB/s
```

**用户感知**：
- ⏸️ 00:11 短暂缓冲（1-2 秒）
- ▶️ 00:12 播放恢复
- ✅ 从 140MB 位置继续（无需重新加载前面的内容）

---

## 📝 总结

### 无缝故障转移

- ✅ 自动检测节点健康（每次 Range 请求）
- ✅ 使用 HTTP 307 重定向切换节点
- ✅ 保留原始 Range 头，保持播放位置
- ✅ 防死循环（最多重试 3 次）
- ✅ 1-2 秒内完成切换

### Range 分片策略

- ✅ 客户端决定分片大小（服务端不强制）
- ✅ 速率限制控制传输速度（50MB/s）
- ✅ 前 50MB 不限速（快速启动）
- ✅ 连接超时自动清理（5 分钟）
- ✅ 可选并发限制（防止资源耗尽）

### 协同优势

- ✅ 故障转移不丢失播放位置
- ✅ 速率限制在所有节点生效
- ✅ 无缝切换，用户无感知
- ✅ 资源可控，带宽公平分配

---

## 🔧 配置建议

### 默认配置（推荐）

```nginx
# 适合大部分场景
limit_rate_after 50m;
limit_rate 50m;
send_timeout 300s;
keepalive_timeout 300s;
```

### 高带宽服务器

```nginx
limit_rate_after 100m;
limit_rate 100m;
send_timeout 600s;
keepalive_timeout 600s;
```

### 低带宽/多用户

```nginx
limit_rate_after 20m;
limit_rate 20m;
send_timeout 180s;
keepalive_timeout 180s;
limit_conn perip 3;  # 添加并发限制
```
