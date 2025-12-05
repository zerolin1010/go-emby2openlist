# 鉴权服务器快速开始

> 5 分钟配置后端集中式鉴权

---

## 🎯 适用场景

如果你需要以下任一功能，建议使用鉴权服务器：

- ✅ **详细访问日志**：记录每个用户的访问记录
- ✅ **统计分析**：查看访问量、失败率、Top 用户
- ✅ **集中管理**：多个 Nginx 节点统一鉴权
- ✅ **审计合规**：满足安全审计要求

---

## ⚡ 5 分钟配置

### 步骤 1: 修改配置文件（1 分钟）

编辑 `config.yml`，找到 `auth` 部分：

```yaml
auth:
  user-key-cache-ttl: 24h
  nginx-auth-enable: true

  # 👇 添加或修改以下配置
  enable-auth-server: true                        # 启用鉴权服务器
  auth-server-port: "8097"                        # 端口
  enable-auth-server-log: true                    # 启用日志
  auth-server-log-path: "./logs/auth-access.log"  # 日志路径
```

### 步骤 2: 重启服务（1 分钟）

```bash
docker restart go-emby2openlist

# 查看日志，确认启动成功
docker logs go-emby2openlist | grep "鉴权服务器"
# ✅ 应该看到：鉴权服务器启动在端口: 8097
```

### 步骤 3: 测试鉴权服务（1 分钟）

```bash
# ① 测试健康检查
curl http://localhost:8097/api/health
# ✅ {"status":"ok","service":"auth-server"}

# ② 测试鉴权接口（用你的真实 api_key）
curl -i "http://localhost:8097/api/auth?api_key=your_real_api_key"
# ✅ HTTP/1.1 200 OK

# ③ 查看统计信息
curl http://localhost:8097/api/stats
# ✅ {"total_requests":1,"success_requests":1,...}
```

### 步骤 4: 配置 Nginx（2 分钟）

**方式 A：快速配置（复制粘贴）**

```bash
# 1. 复制配置文件
sudo cp nginx/video-with-backend-auth.conf /etc/nginx/conf.d/

# 2. 修改 upstream 地址（如果 Nginx 和代理服务不在同一台机器）
sudo vi /etc/nginx/conf.d/video-with-backend-auth.conf
# 找到 upstream auth_backend，修改为实际 IP
# server go-emby2openlist:8097; → server 192.168.0.100:8097;

# 3. 测试配置
sudo nginx -t

# 4. 重载 Nginx
sudo nginx -s reload
```

**方式 B：手动修改现有配置**

在你现有的 Nginx 配置中添加：

```nginx
# 1. 在 http 块或 server 块外定义 upstream
upstream auth_backend {
    server go-emby2openlist:8097;  # 修改为实际地址
    keepalive 32;
}

# 2. 在 server 块内添加鉴权 location
server {
    # ... 现有配置 ...

    # 鉴权子请求
    location = /auth {
        internal;
        proxy_pass http://auth_backend/api/auth?api_key=$arg_api_key&target_path=$request_uri&remote_ip=$remote_addr;
        proxy_connect_timeout 3s;
        proxy_read_timeout 3s;
    }

    # 3. 在视频 location 中启用鉴权
    location /video/data {
        alias /mnt/google/;
        auth_request /auth;  # 👈 添加这一行

        # ... 其他配置 ...
    }
}
```

保存并重载：
```bash
sudo nginx -t && sudo nginx -s reload
```

---

## ✅ 验证配置

### 测试 1: 无效 API Key（应该失败）

```bash
curl -I "http://your-nginx-node/video/data/test.mp4?api_key=invalid"
# ❌ HTTP/1.1 403 Forbidden
```

### 测试 2: 有效 API Key（应该成功）

```bash
curl -I "http://your-nginx-node/video/data/test.mp4?api_key=your_real_key"
# ✅ HTTP/1.1 200 OK 或 HTTP/1.1 206 Partial Content
```

### 测试 3: 查看访问日志

```bash
tail -f ./logs/auth-access.log

# 或使用 jq 格式化
tail -f ./logs/auth-access.log | jq
```

你应该看到类似这样的日志：

```json
{
  "timestamp": "2025-12-06T10:30:45Z",
  "remote_ip": "192.168.1.100",
  "status": 200,
  "api_key": "abcd****efgh",
  "auth_result": "success",
  "duration": 15000000,
  "original_path": "/video/data/test.mp4"
}
```

---

## 📊 查看统计信息

```bash
# 查看统计（格式化输出）
curl http://localhost:8097/api/stats | jq

# 实时监控成功率
watch -n 5 'curl -s http://localhost:8097/api/stats | jq ".success_requests / .total_requests * 100"'

# 查看失败原因
curl -s http://localhost:8097/api/stats | jq '.fail_reasons'
```

---

## 🚀 可选：启用 Nginx 缓存（强烈推荐）

缓存可以将性能提升 **10 倍以上**！

在 Nginx 配置的 `http` 块中添加：

```nginx
# /etc/nginx/nginx.conf 的 http 块中
proxy_cache_path /var/cache/nginx/auth
    levels=1:2
    keys_zone=auth_cache:10m
    max_size=100m
    inactive=60m;
```

在 `location = /auth` 中添加：

```nginx
location = /auth {
    internal;
    proxy_pass http://auth_backend/api/auth?api_key=$arg_api_key;

    # 👇 添加缓存配置
    proxy_cache auth_cache;
    proxy_cache_key "$arg_api_key";
    proxy_cache_valid 200 10m;  # 成功响应缓存 10 分钟
    proxy_cache_valid 403 1m;   # 失败响应缓存 1 分钟
}
```

创建缓存目录并重载：

```bash
sudo mkdir -p /var/cache/nginx/auth
sudo chown -R nginx:nginx /var/cache/nginx/auth
sudo nginx -s reload
```

---

## 📝 日常运维

### 查看日志

```bash
# 实时查看
tail -f ./logs/auth-access.log | jq

# 查看失败记录
cat ./logs/auth-access.log | jq 'select(.auth_result == "failed")'

# 统计 Top 用户
cat ./logs/auth-access.log | jq -r '.api_key' | sort | uniq -c | sort -rn | head -10
```

### Docker Compose 配置

```yaml
services:
  go-emby2openlist:
    image: zerolin1010/go-emby2openlist:latest
    ports:
      - "8095:8095"  # HTTP
      - "8094:8094"  # HTTPS
      - "8097:8097"  # 鉴权服务器 👈 添加这一行
    volumes:
      - ./config.yml:/app/config.yml
      - ./logs:/app/logs  # 日志目录
```

---

## ❓ 常见问题

### Q1: 鉴权服务器启动失败？

**检查配置**：
```bash
docker logs go-emby2openlist | grep -i error

# 确认端口未被占用
netstat -tulnp | grep 8097
```

### Q2: Nginx 报错 502 Bad Gateway？

**检查网络连通性**：
```bash
# 从 Nginx 容器测试
docker exec nginx-container curl http://go-emby2openlist:8097/api/health

# 如果不通，检查 upstream 地址是否正确
```

### Q3: 日志文件过大？

**方法 1：关闭日志**
```yaml
auth:
  enable-auth-server-log: false
```

**方法 2：定期清理**
```bash
# 每周清理 30 天前的日志
find ./logs -name "auth-access.log.*" -mtime +30 -delete
```

### Q4: 性能不够？

**启用 Nginx 缓存**（见上文），可提升 10 倍性能！

---

## 🎓 下一步

- 📖 阅读 [完整文档](./AUTH_SERVER.md)
- 📊 学习 [日志分析技巧](./AUTH_SERVER.md#访问日志)
- ⚙️ 配置 [高级功能](./AUTH_SERVER.md#高级配置)

---

**配置完成！** 🎉

现在你的系统已经有了完整的鉴权和日志功能。
