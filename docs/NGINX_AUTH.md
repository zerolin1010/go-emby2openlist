# Nginx 鉴权配置指南

> 解决 302 重定向后如何防止 Nginx 节点被直接访问的问题

---

## 📋 问题说明

当前架构：客户端 → 代理服务器 → **302 重定向** → Nginx 节点 → 视频文件

**安全问题**：
- ❌ 如果知道 Nginx 节点地址，可以绕过代理直接访问
- ❌ 任何人都能下载 `http://nginx-node/video/data/movie.mp4`
- ❌ 无法统计用户访问和限制带宽

**解决方案**：在 Nginx 层增加鉴权，验证 URL 中的 `api_key` 参数。

---

## 🔒 三种鉴权方案对比

| 方案 | 安全性 | 性能 | 复杂度 | 推荐度 |
|------|--------|------|--------|--------|
| **方案 1：URL 参数鉴权** | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐ | ✅ 推荐 |
| **方案 2：Emby API 验证** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | 高安全需求 |
| **方案 3：JWT Token** | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | 企业级 |

---

## 方案 1：URL 参数鉴权（推荐）

### 工作原理

```
客户端请求视频
    ↓
代理服务器鉴权（ApiKeyChecker 中间件）
    ↓
验证通过 → 构建 302 URL（带 api_key）
    ↓
302: http://nginx-node/video/data/movie.mp4?api_key=xxx123
    ↓
客户端访问 Nginx
    ↓
Nginx 检查 $arg_api_key 是否存在
    ├─ 存在 → 返回视频 ✅
    └─ 不存在 → 返回 403 ❌
```

### 配置步骤

#### 1. 启用代理服务器的 Nginx 鉴权

编辑 `config.yml`：

```yaml
auth:
  # 启用后，302 URL 会自动携带 api_key 参数
  nginx-auth-enable: true

  # 用户 api_key 缓存时间（减少 Emby 请求）
  user-key-cache-ttl: 24h
```

重启服务：
```bash
docker restart go-emby2openlist
```

#### 2. 配置 Nginx 鉴权

使用配置文件：`nginx/video-with-auth.conf`

**核心配置**：

```nginx
location /video/data {
    alias /mnt/google/;

    # 必须有 api_key 参数
    if ($arg_api_key = "") {
        return 403 '{"error":"Missing api_key"}';
    }

    # 其他配置...
    sendfile on;
    tcp_nopush on;
}
```

**部署**：

```bash
# 复制配置文件
sudo cp nginx/video-with-auth.conf /etc/nginx/conf.d/

# 测试配置
sudo nginx -t

# 重载配置
sudo nginx -s reload
```

### 测试验证

```bash
# ❌ 无 api_key - 应返回 403
curl -I http://nginx-node/video/data/test.mp4

# ✅ 有 api_key - 应返回 200 或 206
curl -I "http://nginx-node/video/data/test.mp4?api_key=xxx123"
```

### 优点

- ✅ **简单高效**：只需检查参数是否存在
- ✅ **性能极佳**：Nginx 内置功能，无额外请求
- ✅ **兼容性好**：所有 Nginx 版本都支持
- ✅ **足够安全**：结合代理服务器鉴权，双层防护

### 缺点

- ⚠️ api_key 暴露在 URL 中（可被日志记录）
- ⚠️ 如果泄露，可以重放攻击

### 安全建议

1. **不记录敏感日志**：

```nginx
location /video/ {
    # 关闭访问日志（避免 api_key 泄露）
    access_log off;

    # 或使用自定义日志格式（不记录 query string）
    # log_format video_log '$remote_addr - [$time_local] "$request_method $uri"';
    # access_log /var/log/nginx/video.log video_log;
}
```

2. **启用 HTTPS**：防止中间人窃取 api_key

```nginx
server {
    listen 443 ssl http2;
    ssl_certificate /path/to/cert.crt;
    ssl_certificate_key /path/to/cert.key;
}
```

3. **定期轮换 Key**：Emby 管理员定期重新生成 API Key

---

## 方案 2：Emby API 验证（高安全）

### 工作原理

```
客户端访问 Nginx（带 api_key）
    ↓
Nginx 使用 auth_request 子请求
    ↓
调用 Emby API: GET /emby/System/Info?api_key=xxx
    ├─ 200 → api_key 有效 → 返回视频 ✅
    └─ 401 → api_key 无效 → 返回 403 ❌
```

### 配置步骤

#### 1. 检查 Nginx 模块

```bash
nginx -V 2>&1 | grep -o auth_request

# 如果没有输出，需要重新编译 Nginx 或安装模块
```

**Ubuntu/Debian 安装**：
```bash
sudo apt install nginx-extras
```

**或编译安装**：
```bash
./configure --with-http_auth_request_module
make && sudo make install
```

#### 2. 启用代理服务器配置

```yaml
auth:
  nginx-auth-enable: true  # 302 URL 带 api_key
  user-key-cache-ttl: 24h
```

#### 3. 使用 Nginx 配置文件

配置文件：`nginx/video-with-emby-auth.conf`

**核心配置**：

```nginx
# 定义 Emby 上游服务器
upstream emby_backend {
    server emby-server:8096;
    keepalive 32;
}

server {
    listen 80;

    # 鉴权子请求
    location = /auth {
        internal;  # 只允许内部调用

        # 调用 Emby API 验证
        proxy_pass http://emby_backend/emby/System/Info?api_key=$arg_api_key;
        proxy_pass_request_body off;
        proxy_set_header Content-Length "";
    }

    # 视频服务
    location /video/data {
        alias /mnt/google/;

        # 启用鉴权
        auth_request /auth;

        # 其他配置...
    }
}
```

**部署**：
```bash
sudo cp nginx/video-with-emby-auth.conf /etc/nginx/conf.d/
sudo nginx -t
sudo nginx -s reload
```

### 测试验证

```bash
# ❌ 无效 api_key
curl -I "http://nginx-node/video/data/test.mp4?api_key=invalid"
# 应返回 403

# ✅ 有效 api_key
curl -I "http://nginx-node/video/data/test.mp4?api_key=valid_key"
# 应返回 200/206
```

### 优点

- ✅ **最高安全性**：每次请求都验证 Emby
- ✅ **实时验证**：Key 撤销立即生效
- ✅ **精确控制**：可基于 Emby 用户权限管理

### 缺点

- ⚠️ **性能开销**：每次视频请求都调用 Emby API（~50ms）
- ⚠️ **Emby 压力**：大量并发会增加 Emby 负载
- ⚠️ **依赖性**：Emby 不可用时 Nginx 也无法服务

### 优化建议

1. **启用 Nginx 缓存**：

```nginx
# 缓存鉴权结果
proxy_cache_path /var/cache/nginx/auth levels=1:2 keys_zone=auth_cache:10m max_size=100m inactive=60m;

location = /auth {
    internal;
    proxy_pass http://emby_backend/emby/System/Info?api_key=$arg_api_key;

    # 启用缓存
    proxy_cache auth_cache;
    proxy_cache_key "$arg_api_key";
    proxy_cache_valid 200 60m;  # 成功响应缓存 60 分钟
    proxy_cache_valid 401 1m;   # 失败响应缓存 1 分钟
}
```

2. **使用 Lua 缓存**：

```nginx
location /video/data {
    access_by_lua_block {
        local cache = ngx.shared.auth_cache
        local api_key = ngx.var.arg_api_key
        local cached = cache:get(api_key)

        if cached == "valid" then
            return  -- 缓存命中，通过
        end

        -- 调用 Emby 验证
        local res = ngx.location.capture("/auth")
        if res.status == 200 then
            cache:set(api_key, "valid", 3600)  -- 缓存 1 小时
            return
        else
            ngx.exit(403)
        end
    }
}
```

---

## 方案 3：JWT Token（企业级）

### 工作原理

```
客户端请求视频
    ↓
代理服务器生成 JWT Token（包含 userId, exp）
    ↓
302: http://nginx-node/video/data/movie.mp4?token=eyJhbGc...
    ↓
Nginx 验证 JWT 签名和过期时间
    ├─ 有效 → 返回视频 ✅
    └─ 无效/过期 → 返回 403 ❌
```

### 优点

- ✅ **无状态验证**：不需要查询数据库
- ✅ **防重放攻击**：Token 包含过期时间
- ✅ **携带信息**：可包含用户 ID、权限等
- ✅ **高性能**：Nginx 本地验证，无外部请求

### 实现方式

需要修改 go-emby2openlist 代码，暂不详述。

---

## 推荐配置

### 场景 1：家庭/小型部署

**推荐**：方案 1（URL 参数鉴权）

**原因**：
- 简单易用，配置 5 分钟搞定
- 性能最佳，无额外开销
- 已有代理服务器鉴权，双层防护足够

**配置**：
```yaml
# config.yml
auth:
  nginx-auth-enable: true
  user-key-cache-ttl: 24h
```

```nginx
# Nginx
if ($arg_api_key = "") {
    return 403;
}
```

### 场景 2：多用户/公共服务

**推荐**：方案 2（Emby API 验证） + Nginx 缓存

**原因**：
- 实时验证用户权限
- Key 撤销立即生效
- 配合缓存，性能可接受

**配置**：
```nginx
proxy_cache_path /var/cache/nginx/auth keys_zone=auth_cache:10m;
location = /auth {
    proxy_cache auth_cache;
    proxy_cache_valid 200 60m;
}
```

### 场景 3：企业级/高安全

**推荐**：方案 3（JWT Token）

**原因**：
- 最高安全性和灵活性
- 支持细粒度权限控制
- 可审计、可撤销

---

## 常见问题

### Q1: 是否一定要配置 Nginx 鉴权？

**A**: 取决于你的使用场景：

- **内网环境 + 信任所有用户** → 可以不配置
- **公网暴露 + 多用户** → 强烈建议配置
- **有敏感内容** → 必须配置

### Q2: 方案 1 的 api_key 会被日志记录吗？

**A**: 默认会，但可以关闭：

```nginx
location /video/ {
    access_log off;  # 完全关闭日志
}
```

或自定义日志格式（不记录 query string）：

```nginx
log_format video_log '$remote_addr - [$time_local] "$request_method $uri" $status';
access_log /var/log/nginx/video.log video_log;
```

### Q3: 如何防止 api_key 泄露后被滥用？

**A**: 多层防护：

1. **启用 HTTPS**（必须）
2. **定期轮换 Key**（Emby 管理面板）
3. **监控异常访问**（Nginx 日志分析）
4. **限流保护**（nginx limit_req）

```nginx
limit_req_zone $arg_api_key zone=api_limit:10m rate=10r/s;

location /video/ {
    limit_req zone=api_limit burst=20;
}
```

### Q4: 方案 2 会影响性能吗？

**A**: 未缓存时有影响，建议配置：

- 启用 Nginx 缓存：60 分钟
- Emby 响应时间：~50ms
- 缓存命中率：> 95%
- 平均延迟：< 5ms

### Q5: 如何测试鉴权是否生效？

**测试步骤**：

```bash
# 1. 无 api_key（应该 403）
curl -I http://nginx-node/video/data/test.mp4

# 2. 有无效 api_key（应该 403）
curl -I "http://nginx-node/video/data/test.mp4?api_key=invalid"

# 3. 有效 api_key（应该 200/206）
curl -I "http://nginx-node/video/data/test.mp4?api_key=your_valid_key"

# 4. 查看 Nginx 日志
sudo tail -f /var/log/nginx/error.log
```

---

## 部署清单

### ✅ 方案 1 部署

- [ ] 修改 `config.yml`：`nginx-auth-enable: true`
- [ ] 重启代理服务：`docker restart go-emby2openlist`
- [ ] 复制 `nginx/video-with-auth.conf` 到 Nginx
- [ ] 测试配置：`nginx -t`
- [ ] 重载 Nginx：`nginx -s reload`
- [ ] 验证鉴权：`curl` 测试
- [ ] 检查日志：确认 403/200 响应

### ✅ 方案 2 部署

- [ ] 检查 Nginx 是否有 `auth_request` 模块
- [ ] 修改 `config.yml`：`nginx-auth-enable: true`
- [ ] 重启代理服务
- [ ] 复制 `nginx/video-with-emby-auth.conf`
- [ ] 修改 `emby-server` 地址
- [ ] 配置缓存目录：`mkdir -p /var/cache/nginx/auth`
- [ ] 测试配置：`nginx -t`
- [ ] 重载 Nginx：`nginx -s reload`
- [ ] 验证 Emby 连通性
- [ ] 测试鉴权流程

---

## 相关文档

- [ARCHITECTURE.md](./ARCHITECTURE.md) - 完整架构设计
- [README.md](../README.md) - 快速开始
- [nginx/README.md](../nginx/README.md) - Nginx 配置说明

---

**文档版本**: v1.0
**最后更新**: 2025-12-06
**项目版本**: v2.3.3
