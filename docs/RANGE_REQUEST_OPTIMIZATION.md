# Range 请求优化指南

## 📋 问题背景

您提到的问题：**"现在改版之后只有一个完整体积的长链接了，可能是受限于鉴权机制"**

实际情况分析：
1. **Range 请求仍然正常工作**（从日志可以看到 206 响应和分片请求）
2. **某些请求的 Range 很大**（如 2.3GB 的单次请求）是**客户端决定的**，不是服务端问题
3. **鉴权机制不影响分片**（每个 Range 请求都会经过 auth_request 验证）

---

## 🔍 Range 请求分析

### 实际日志分析

从您的 Nginx 日志可以看到：

```
206 65536        ← 64KB 小分片（初始探测）
206 40508898     ← 38MB 分片（正常播放）
206 1209080      ← 1.1MB 分片（音频轨道或字幕）
206 2368287638   ← 2.2GB 大分片（⚠️ 客户端拖动进度条）
206 142662373    ← 136MB 分片（正常播放）
```

**结论**：
- ✅ 多个小分片请求（正常播放）
- ⚠️ 偶尔有大分片请求（用户快进/拖动进度条）

**大分片的原因**：
1. 用户拖动进度条到后面
2. 客户端缓存策略（Chrome 会请求较大的 Range）
3. 播放器预加载设置

---

## 🎯 优化方案

### 方案 1：速率限制（已实现，推荐）

**当前配置**：
```nginx
limit_rate_after 50m;  # 前 50MB 不限速
limit_rate 50m;        # 之后限速为 50MB/s
```

**效果**：
- ✅ 不影响正常播放（前 50MB 快速加载）
- ✅ 限制单个连接带宽（避免占用过多资源）
- ✅ 即使请求 2GB，实际传输速度也受限
- ⚠️ 不减少请求数量，只控制传输速度

**适用场景**：
- 多用户并发播放
- 带宽有限的服务器
- 需要公平分配带宽

**调整方法**（编辑 Nginx 配置）：
```nginx
# 更宽松（高带宽服务器）
limit_rate_after 100m;
limit_rate 100m;

# 更严格（低带宽/多用户）
limit_rate_after 20m;
limit_rate 20m;
```

---

### 方案 2：并发连接限制（推荐添加）

**限制单个 IP 的并发连接数**：

```nginx
# 1. 在 nginx.conf 的 http 块中添加
http {
    limit_conn_zone $binary_remote_addr zone=perip:10m;
}

# 2. 在 location 中应用
location ~ ^/internal/(data[^/]*)/(.*)$ {
    limit_conn perip 5;  # 限制单个 IP 最多 5 个并发连接
    # ... 其他配置 ...
}
```

**效果**：
- ✅ 防止单个用户打开过多连接
- ✅ 保护服务器资源
- ⚠️ 可能影响多设备同时播放

---

### 方案 3：连接超时控制（已实现）

**当前配置**：
```nginx
send_timeout 300s;     # 5分钟无数据传输则断开
keepalive_timeout 300s;
```

**效果**：
- ✅ 自动断开长时间无活动的连接
- ✅ 释放服务器资源
- ⚠️ 用户暂停播放超过5分钟会断开

**调整建议**：
```nginx
# 短超时（节省资源，但可能影响暂停播放）
send_timeout 120s;
keepalive_timeout 120s;

# 长超时（用户体验好，但占用资源）
send_timeout 600s;
keepalive_timeout 600s;
```

---

## 📊 推荐配置

### 配置 A：平衡型（推荐，适合大部分场景）

```nginx
location ~ ^/internal/(data[^/]*)/(.*)$ {
    # ... 鉴权配置 ...

    # 速率限制
    limit_rate_after 50m;
    limit_rate 50m;

    # 连接超时
    send_timeout 300s;
    keepalive_timeout 300s;

    # 并发限制（需要在 http 块中先定义 limit_conn_zone）
    limit_conn perip 5;

    root $root_path;
}
```

**特点**：
- ✅ 限制单连接带宽（50MB/s）
- ✅ 限制单 IP 并发连接（5个）
- ✅ 自动断开无活动连接（5分钟）
- ✅ 资源开销低，用户体验好

---

### 配置 B：高性能型（大带宽服务器，少量用户）

```nginx
location ~ ^/internal/(data[^/]*)/(.*)$ {
    # 更高的速率限制
    limit_rate_after 100m;
    limit_rate 100m;

    # 更长的超时
    send_timeout 600s;
    keepalive_timeout 600s;

    # 更宽松的并发限制
    limit_conn perip 10;

    root $root_path;
}
```

---

### 配置 C：资源节约型（低带宽/多用户）

```nginx
location ~ ^/internal/(data[^/]*)/(.*)$ {
    # 较低的速率限制
    limit_rate_after 20m;
    limit_rate 20m;

    # 较短的超时
    send_timeout 180s;
    keepalive_timeout 180s;

    # 严格的并发限制
    limit_conn perip 3;

    root $root_path;
}
```

---

## 🧪 测试和监控

### 查看当前 Range 请求分布

```bash
# 统计不同大小的 Range 响应
awk '{print $10}' /var/log/nginx/video_internal_access.log | sort -n | uniq -c | tail -20

# 查看最大的 Range 响应
awk '{print $10}' /var/log/nginx/video_internal_access.log | sort -n | tail -10

# 统计平均响应大小
awk '{sum+=$10; count++} END {print "平均: " sum/count/1024/1024 " MB/请求"}' /var/log/nginx/video_internal_access.log
```

### 监控并发连接数

```bash
# 查看当前活跃连接
netstat -an | grep :7777 | grep ESTABLISHED | wc -l

# 查看每个 IP 的连接数
netstat -an | grep :7777 | awk '{print $5}' | cut -d: -f1 | sort | uniq -c | sort -rn
```

### 监控传输速率

```bash
# 实时查看访问日志
tail -f /var/log/nginx/video_internal_access.log

# 统计最近 100 个请求的总流量
tail -100 /var/log/nginx/video_internal_access.log | awk '{sum+=$10} END {print sum/1024/1024/1024 " GB"}'
```

---

## ❓ 常见问题

### Q1：为什么有些请求很大（2GB）？

**A**：这是客户端决定的，常见原因：
1. 用户拖动进度条到视频后半部分（Range: bytes=2000000000-4000000000）
2. Chrome 的预加载策略（请求大 Range 但不一定全部下载）
3. 播放器的缓存逻辑

**重要**：Nginx 日志显示的是**响应大小**，不是请求的 Range。即使客户端请求 Range: bytes=0-2GB，如果启用了 `limit_rate 50m`，实际传输速度也受限为 50MB/s。

### Q2：鉴权机制会影响分片吗？

**A**：不会。每个 Range 请求都会：
1. 触发 `auth_request` 验证 token
2. 验证通过后，Nginx 返回对应的 Range 数据
3. 会话续期机制保证长时间播放不中断

**鉴权只影响访问权限，不影响分片逻辑**。客户端可以自由决定 Range 大小。

### Q3：如何直接限制单个请求的最大 Range？

**A**：Nginx **不支持**直接限制 Range 请求的大小。这是因为 Range 是 HTTP 协议的一部分，由客户端控制。

**替代方案**：
1. **速率限制**：控制传输速度，即使 Range 很大，传输也受限
2. **连接超时**：限制传输时间，自动断开长时间传输
3. **并发限制**：限制同时连接数，间接控制总流量

### Q4：Nginx Slice 模块能解决问题吗？

**A**：Slice 模块主要用于**缓存场景**，不适合直接服务文件的场景：

```nginx
# Slice 模块用法（需要配合 proxy_cache）
slice 100m;
proxy_cache video_cache;
proxy_cache_key $uri$is_args$args$slice_range;
proxy_set_header Range $slice_range;
```

**限制**：
- ❌ 需要 Nginx 作为反向代理
- ❌ 当前架构是直接 `root` 读取文件，不能使用 slice
- ❌ 即使使用，也只是强制缓存分片，不改变客户端请求

---

## 📝 总结

**当前状态**：
- ✅ Range 请求正常工作（多个 206 响应）
- ✅ 已启用速率限制（50MB/s）
- ✅ 已设置连接超时（5分钟）
- ⚠️ 偶尔有大 Range 请求（客户端行为）

**建议操作**：
1. **保持当前配置**（如果带宽充足，用户体验良好）
2. **添加并发限制**（如果多用户并发，防止单用户占用过多资源）
3. **调整速率限制**（根据实际带宽和用户需求）

**不需要的操作**：
- ❌ 不需要修改鉴权逻辑（鉴权不影响分片）
- ❌ 不需要强制分片（客户端自己会分片）
- ❌ 不需要担心大 Range 请求（速率限制已经控制了传输速度）

**关键理解**：
> Nginx 日志中的大数字（如 2368287638）是**实际传输的字节数**，不是瞬间传输的。如果启用了 `limit_rate 50m`，这 2.2GB 数据实际需要约 44 秒才能传输完成（2200MB ÷ 50MB/s）。所以即使单个请求很大，也不会瞬间占用全部带宽。

需要我帮您应用某个具体的配置吗？例如添加并发连接限制？
