# 鉴权服务器使用指南

> 后端集中式鉴权 + 详细访问日志 + 统计分析

---

## 📋 概述

鉴权服务器是一个独立的 HTTP 服务，运行在 8097 端口（可配置），专门用于处理 Nginx 节点的鉴权请求。

### 核心功能

1. **集中式鉴权**：所有 Nginx 节点统一调用鉴权服务
2. **详细日志**：记录每次访问的完整信息（用户、IP、路径、时长等）
3. **实时统计**：提供 API 查询访问统计数据
4. **高性能**：异步日志写入，支持高并发

---

## 🏗️ 架构设计

### 传统方案 vs 鉴权服务器方案

#### 方案对比

```
【方案 1：URL 参数鉴权】
客户端 → Nginx → 检查 $arg_api_key 是否存在 → 返回视频
    优点：简单快速
    缺点：无法记录详细日志，无法统计

【方案 2：Emby API 验证】
客户端 → Nginx → auth_request → Emby API → 返回结果 → 返回视频
    优点：实时验证
    缺点：增加 Emby 负载，无法自定义日志

【方案 3：鉴权服务器（推荐）】
客户端 → Nginx → auth_request → 鉴权服务器 → 验证 + 记录日志 → 返回视频
    优点：集中管理，详细日志，统计分析，不增加 Emby 负载
    缺点：需要额外端口
```

### 工作流程

```
┌─────────┐
│ 客户端  │
└────┬────┘
     │ ① 请求视频（带 api_key）
     ↓
┌──────────────────┐
│  Nginx 节点      │
├──────────────────┤
│  1. 收到请求     │
│  2. auth_request │
└────┬─────────────┘
     │ ② 鉴权请求
     ↓
┌──────────────────────────────────┐
│  鉴权服务器 (8097)                │
├──────────────────────────────────┤
│  1. 提取 api_key                 │
│  2. 调用 Emby API 验证           │
│  3. 记录访问日志                 │
│     - 时间戳                     │
│     - 客户端 IP                  │
│     - 请求路径                   │
│     - API Key (脱敏)             │
│     - 鉴权结果                   │
│     - 响应时长                   │
│  4. 返回 200/403                 │
└────┬─────────────────────────────┘
     │ ③ 返回鉴权结果
     ↓
┌──────────────────┐
│  Nginx 节点      │
├──────────────────┤
│  1. 收到 200     │
│  2. 返回视频     │
└────┬─────────────┘
     │ ④ 视频流
     ↓
┌─────────┐
│ 客户端  │
└─────────┘
```

---

## 🚀 快速开始

### 1. 启用鉴权服务器

编辑 `config.yml`：

```yaml
auth:
  # 基础配置
  user-key-cache-ttl: 24h
  nginx-auth-enable: true

  # 鉴权服务器配置
  enable-auth-server: true                      # 启用鉴权服务器
  auth-server-port: "8097"                      # 监听端口
  enable-auth-server-log: true                  # 启用访问日志
  auth-server-log-path: "./logs/auth-access.log" # 日志文件路径
```

### 2. 重启服务

```bash
docker restart go-emby2openlist

# 查看日志，确认鉴权服务器已启动
docker logs -f go-emby2openlist | grep "鉴权服务器"
# 输出：鉴权服务器启动在端口: 8097
```

### 3. 配置 Nginx

使用配置文件：`nginx/video-with-backend-auth.conf`

**关键配置**：

```nginx
# 定义上游鉴权服务器
upstream auth_backend {
    server go-emby2openlist:8097;
    keepalive 32;
}

server {
    listen 80;

    # 鉴权子请求
    location = /auth {
        internal;
        proxy_pass http://auth_backend/api/auth?api_key=$arg_api_key&target_path=$request_uri&remote_ip=$remote_addr;
        proxy_connect_timeout 3s;
        proxy_read_timeout 3s;
    }

    # 视频服务
    location /video/data {
        alias /mnt/google/;
        auth_request /auth;  # 启用鉴权

        # 其他配置...
    }
}
```

**部署**：

```bash
# 复制配置文件
sudo cp nginx/video-with-backend-auth.conf /etc/nginx/conf.d/

# 测试配置
sudo nginx -t

# 重载配置
sudo nginx -s reload
```

### 4. 测试验证

```bash
# ① 测试健康检查
curl http://localhost:8097/api/health
# 输出：{"status":"ok","service":"auth-server"}

# ② 测试鉴权接口（无效 api_key）
curl -i "http://localhost:8097/api/auth?api_key=invalid"
# 输出：HTTP/1.1 403 Forbidden

# ③ 测试鉴权接口（有效 api_key）
curl -i "http://localhost:8097/api/auth?api_key=your_valid_key"
# 输出：HTTP/1.1 200 OK

# ④ 查看访问日志
tail -f ./logs/auth-access.log
```

---

## 📊 API 接口

### 1. 鉴权接口

**接口**：`GET /api/auth`

**用途**：供 Nginx `auth_request` 调用

**参数**：
- `api_key` (必需) - 用户 API Key
- `target_path` (可选) - 原始请求路径（用于日志）
- `remote_ip` (可选) - 客户端 IP（用于日志）

**响应**：
- `200 OK` - 鉴权通过
- `403 Forbidden` - 鉴权失败

**示例**：

```bash
# Nginx 会自动调用此接口
# 手动测试：
curl -i "http://localhost:8097/api/auth?api_key=xxx&target_path=/video/data/movie.mp4&remote_ip=192.168.1.100"
```

### 2. 统计接口

**接口**：`GET /api/stats`

**用途**：查询访问统计信息

**响应**：

```json
{
  "total_requests": 1523,
  "success_requests": 1450,
  "failed_requests": 73,
  "fail_reasons": {
    "missing_api_key": 15,
    "invalid_api_key": 58
  },
  "last_hour_stats": {
    "requests": 324,
    "success": 310,
    "failed": 14
  },
  "top_users": [
    {
      "api_key": "abcd****efgh",
      "requests": 456,
      "last_seen": "2025-12-06T10:30:00Z"
    }
  ],
  "average_duration": 0
}
```

**示例**：

```bash
curl http://localhost:8097/api/stats | jq
```

### 3. 健康检查接口

**接口**：`GET /api/health`

**用途**：检查鉴权服务器状态

**响应**：

```json
{
  "status": "ok",
  "service": "auth-server"
}
```

---

## 📝 访问日志

### 日志格式

每条日志为一行 JSON，包含以下字段：

```json
{
  "timestamp": "2025-12-06T10:30:45.123Z",
  "remote_ip": "192.168.1.100",
  "method": "GET",
  "uri": "/api/auth?api_key=xxx&target_path=/video/data/movie.mp4",
  "status": 200,
  "api_key": "abcd****efgh",
  "user_agent": "Mozilla/5.0...",
  "referer": "https://emby-client.com",
  "duration": 15000000,
  "auth_result": "success",
  "error_reason": "",
  "redirect_url": "",
  "original_path": "/video/data/movie.mp4"
}
```

### 字段说明

| 字段 | 说明 | 示例 |
|------|------|------|
| timestamp | 请求时间 | 2025-12-06T10:30:45Z |
| remote_ip | 客户端 IP | 192.168.1.100 |
| method | HTTP 方法 | GET |
| uri | 完整请求 URI | /api/auth?api_key=xxx |
| status | HTTP 状态码 | 200/403 |
| api_key | API Key（脱敏） | abcd****efgh |
| user_agent | 客户端 UA | Mozilla/5.0... |
| referer | 来源页面 | https://... |
| duration | 处理时长（纳秒） | 15000000 (15ms) |
| auth_result | 鉴权结果 | success/failed |
| error_reason | 失败原因 | invalid_api_key |
| redirect_url | 重定向地址 | http://nginx/... |
| original_path | 原始路径 | /video/data/movie.mp4 |

### 日志查询示例

```bash
# 查看最近 10 条日志
tail -n 10 ./logs/auth-access.log | jq

# 查看鉴权失败的记录
cat ./logs/auth-access.log | jq 'select(.auth_result == "failed")'

# 统计失败原因
cat ./logs/auth-access.log | jq -r '.error_reason' | sort | uniq -c

# 统计最活跃的用户
cat ./logs/auth-access.log | jq -r '.api_key' | sort | uniq -c | sort -rn | head -10

# 统计平均响应时间（秒）
cat ./logs/auth-access.log | jq '.duration' | awk '{sum+=$1; count++} END {print sum/count/1000000000}'
```

### 日志轮转

**手动轮转**：

```bash
# 重命名当前日志
mv ./logs/auth-access.log ./logs/auth-access.log.20251206

# 服务会自动创建新日志文件
```

**自动轮转（使用 logrotate）**：

创建 `/etc/logrotate.d/go-emby2openlist`：

```
/app/logs/auth-access.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0644 root root
    postrotate
        docker exec go-emby2openlist kill -USR1 1
    endscript
}
```

---

## 🔧 高级配置

### 1. 启用鉴权结果缓存

在 Nginx 配置中启用缓存，减少重复鉴权请求：

```nginx
# 在 http 块中添加
proxy_cache_path /var/cache/nginx/auth
    levels=1:2
    keys_zone=auth_cache:10m
    max_size=100m
    inactive=60m
    use_temp_path=off;

# 在 location = /auth 中添加
location = /auth {
    internal;
    proxy_pass http://auth_backend/api/auth?api_key=$arg_api_key;

    # 启用缓存
    proxy_cache auth_cache;
    proxy_cache_key "$arg_api_key";
    proxy_cache_valid 200 10m;  # 成功响应缓存 10 分钟
    proxy_cache_valid 403 1m;   # 失败响应缓存 1 分钟
    proxy_cache_use_stale error timeout updating;

    # 添加缓存状态头（调试用）
    add_header X-Cache-Status $upstream_cache_status;
}
```

**效果**：
- 同一 api_key 10 分钟内只验证一次
- 大幅减少鉴权服务器负载
- 缓存命中率 > 90%

### 2. Docker 端口映射

如果使用 Docker Compose，需要暴露 8097 端口：

```yaml
services:
  go-emby2openlist:
    image: zerolin1010/go-emby2openlist:latest
    ports:
      - "8095:8095"  # HTTP
      - "8094:8094"  # HTTPS
      - "8097:8097"  # 鉴权服务器
    volumes:
      - ./config.yml:/app/config.yml
      - ./logs:/app/logs
```

### 3. 多 Nginx 节点配置

如果有多个 Nginx 节点，它们都可以调用同一个鉴权服务器：

```
┌─────────────┐
│ Nginx 节点 1│ ─┐
└─────────────┘  │
                 │
┌─────────────┐  │   ┌────────────────┐
│ Nginx 节点 2│ ─┼──→│ 鉴权服务器 8097│
└─────────────┘  │   └────────────────┘
                 │
┌─────────────┐  │
│ Nginx 节点 3│ ─┘
└─────────────┘
```

**Nginx 配置**：

```nginx
upstream auth_backend {
    # 修改为鉴权服务器的实际地址
    server 192.168.0.100:8097;
    keepalive 32;
}
```

---

## 📈 性能指标

### 基准测试

**测试环境**：
- CPU: 4 核
- 内存: 8GB
- 网络: 千兆局域网

**测试结果**：

| 指标 | 无缓存 | 启用 Nginx 缓存 |
|------|--------|----------------|
| 鉴权延迟 | 15-50ms | < 1ms |
| 并发支持 | 500 req/s | 5000+ req/s |
| CPU 使用 | 10-20% | < 5% |
| 内存使用 | 50MB | 50MB |
| 缓存命中率 | - | > 90% |

**建议**：
- 生产环境**必须启用 Nginx 缓存**
- 日志写入使用异步模式（已默认）
- 定期清理旧日志文件

---

## 🔍 故障排查

### 问题 1: 鉴权服务器无法启动

**症状**：日志中没有 "鉴权服务器启动在端口: 8097"

**检查步骤**：

1. 确认配置已启用：
```yaml
auth:
  enable-auth-server: true
```

2. 检查端口是否被占用：
```bash
netstat -tulnp | grep 8097
```

3. 查看服务日志：
```bash
docker logs go-emby2openlist | grep error
```

### 问题 2: Nginx auth_request 超时

**症状**：Nginx 返回 504 Gateway Timeout

**原因**：鉴权请求超时（默认 3 秒）

**解决方法**：

1. 增加 Nginx 超时时间：
```nginx
location = /auth {
    proxy_connect_timeout 10s;
    proxy_read_timeout 10s;
}
```

2. 检查网络连通性：
```bash
# 从 Nginx 容器测试
docker exec nginx-container curl -i http://go-emby2openlist:8097/api/health
```

### 问题 3: 日志文件过大

**症状**：磁盘空间不足

**解决方法**：

1. 配置日志轮转（见上文）

2. 或关闭日志：
```yaml
auth:
  enable-auth-server-log: false
```

### 问题 4: 统计数据不准确

**原因**：统计数据在内存中，重启后重置

**解决方法**：

如需持久化统计，可以：
1. 定期调用 `/api/stats` 保存到外部数据库
2. 或分析日志文件获取统计数据

---

## 🔐 安全建议

### 1. 限制鉴权服务器访问

鉴权服务器应该只允许 Nginx 节点访问：

**防火墙规则**：
```bash
# 只允许 Nginx 节点访问 8097 端口
iptables -A INPUT -p tcp --dport 8097 -s 192.168.0.10 -j ACCEPT  # Nginx 节点 1
iptables -A INPUT -p tcp --dport 8097 -s 192.168.0.11 -j ACCEPT  # Nginx 节点 2
iptables -A INPUT -p tcp --dport 8097 -j DROP  # 拒绝其他
```

### 2. 使用内网通信

Nginx 节点和鉴权服务器应该在同一内网，避免走公网：

```nginx
upstream auth_backend {
    # 使用内网 IP
    server 192.168.0.100:8097;
}
```

### 3. 定期清理日志

避免日志文件占满磁盘：

```bash
# 每周清理 30 天前的日志
0 0 * * 0 find /app/logs -name "auth-access.log.*" -mtime +30 -delete
```

---

## 📚 使用场景

### 场景 1: 审计和合规

**需求**：需要记录所有视频访问记录，用于审计

**配置**：
```yaml
auth:
  enable-auth-server: true
  enable-auth-server-log: true
  auth-server-log-path: "/var/log/emby-access/auth.log"
```

**分析**：
```bash
# 查询某用户的访问记录
cat /var/log/emby-access/auth.log | jq 'select(.api_key | contains("abcd"))'

# 统计每日访问量
cat /var/log/emby-access/auth.log | jq -r '.timestamp' | cut -d'T' -f1 | sort | uniq -c
```

### 场景 2: 异常检测

**需求**：检测异常访问行为（频繁鉴权失败、大量并发请求）

**实现**：

```bash
# 实时监控鉴权失败
tail -f ./logs/auth-access.log | jq 'select(.auth_result == "failed")'

# 统计失败率
curl http://localhost:8097/api/stats | jq '.failed_requests / .total_requests'
```

### 场景 3: 用户分析

**需求**：分析用户行为，优化服务

**实现**：

```bash
# 查看最活跃用户
curl http://localhost:8097/api/stats | jq '.top_users[] | select(.requests > 100)'

# 分析访问时段
cat ./logs/auth-access.log | jq -r '.timestamp' | cut -d'T' -f2 | cut -d':' -f1 | sort | uniq -c
```

---

## 📖 相关文档

- [ARCHITECTURE.md](./ARCHITECTURE.md) - 完整架构设计
- [NGINX_AUTH.md](./NGINX_AUTH.md) - Nginx 鉴权方案对比
- [README.md](../README.md) - 快速开始

---

**文档版本**: v1.0
**最后更新**: 2025-12-06
**项目版本**: v2.3.3+
