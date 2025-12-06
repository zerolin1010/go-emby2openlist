# 视频鉴权测试指南 - 方案1（应用层签名）

## 架构说明

方案1 使用应用层签名的临时 URL 来实现视频鉴权，提供以下安全特性：

1. **HMAC-SHA256 签名** - 防止 URL 伪造
2. **5分钟过期时间** - 限制 URL 分享窗口
3. **用户追踪（UID）** - 记录谁访问和下载了什么文件
4. **完整日志** - 访问日志 + 下载日志

## 工作流程

```
1. 用户请求视频
   GET http://node-ip:7777/video/data/Movie/xxx.mkv?api_key=YOUR_API_KEY

2. Nginx 代理到鉴权服务
   → http://go-emby2openlist:8097/api/video-auth/data/Movie/xxx.mkv?api_key=YOUR_API_KEY

3. 鉴权服务验证 api_key
   - 检查缓存
   - 如果缓存未命中，调用 Emby API 验证
   - 验证通过后生成签名

4. 返回 302 重定向（带签名参数）
   Location: /internal/data/Movie/xxx.mkv?token=abc123&expires=1234567890&uid=def456

5. Nginx 拦截 /internal/ 路径
   → 调用 auth_request /auth-verify

6. 鉴权服务验证 token
   - 检查是否过期
   - 验证 HMAC 签名
   - 从 UID 解密出 api_key
   - 记录下载日志

7. 验证通过，Nginx 提供文件内容
```

## 部署步骤

### 1. 确保配置文件正确

检查 `config.yml`:

```yaml
auth:
  user-key-cache-ttl: 24h
  nginx-auth-enable: true
  enable-auth-server: true
  auth-server-port: "8097"
  enable-auth-server-log: true
  auth-server-log-path: "./logs/auth-access.log"
```

### 2. 部署 Nginx 配置

```bash
# 复制新的 Nginx 配置
cp nginx/video-gateway-SIGNED-URL.conf /etc/nginx/sites-available/video-gateway.conf

# 测试配置
nginx -t

# 重载 Nginx
nginx -s reload
```

### 3. 重启 Go 服务

```bash
# Docker 方式
docker-compose restart go-emby2openlist

# 或手动重启
systemctl restart go-emby2openlist
```

## 测试步骤

### 测试 1: 健康检查

```bash
# 测试健康检查端点（端口 80）
curl -v http://183.179.251.164:80/gtm-health
# 预期: 200 OK

# 测试鉴权服务健康检查（端口 8097）
curl http://127.0.0.1:8097/api/health
# 预期: {"status":"ok","service":"auth-server"}
```

### 测试 2: 视频鉴权流程

```bash
# 使用有效的 api_key 请求视频
curl -v "http://183.179.251.164:7777/video/data/Movie/动画电影/罗小黑战记%20(2019)/罗小黑战记%20(2019)%20-%202160p.H265.DDP%205.1.HDR.mkv?api_key=YOUR_API_KEY"

# 预期响应:
# 1. 首先看到 302 重定向
# 2. Location 头包含 /internal/ 路径和 token/expires/uid 参数
# 3. 如果跟随重定向，最终得到视频文件内容
```

### 测试 3: 无效 api_key

```bash
curl -v "http://183.179.251.164:7777/video/data/Movie/test.mkv?api_key=invalid_key"

# 预期: 403 Forbidden
# {"error":"Invalid api_key"}
```

### 测试 4: 缺少 api_key

```bash
curl -v "http://183.179.251.164:7777/video/data/Movie/test.mkv"

# 预期: 403 Forbidden
# {"error":"Missing api_key"}
```

### 测试 5: 直接访问 /internal/ 路径（应该失败）

```bash
curl -v "http://183.179.251.164:7777/internal/data/Movie/test.mkv"

# 预期: 403 Forbidden
# 因为没有有效的 token 参数
```

### 测试 6: Token 过期

```bash
# 1. 先正常请求获取签名 URL
curl -v "http://183.179.251.164:7777/video/data/Movie/test.mkv?api_key=YOUR_API_KEY" 2>&1 | grep Location

# 2. 复制 Location 中的 URL
# 3. 等待 6 分钟（超过 5 分钟的 TTL）
# 4. 再次访问该 URL

curl -v "http://183.179.251.164:7777/internal/data/Movie/test.mkv?token=xxx&expires=xxx&uid=xxx"

# 预期: 403 Forbidden
# Token 已过期
```

## 查看日志

### Go 应用日志

```bash
# Docker 方式
docker-compose logs -f go-emby2openlist | grep -E "(VideoAuth|TokenVerify)"

# 预期看到:
# [VideoAuth] 鉴权通过，生成临时 URL，用户: 5c76****dc40, 文件: /internal/data/Movie/xxx.mkv
# [TokenVerify] Token 验证通过，用户: 5c76****dc40, 文件: /internal/data/Movie/xxx.mkv
```

### Nginx 访问日志

```bash
# 公开路径访问日志（用户请求）
tail -f /var/log/nginx/video_public_access.log

# 内部路径访问日志（实际下载）
tail -f /var/log/nginx/video_internal_access.log
```

### 鉴权服务专用日志

```bash
# 查看鉴权访问日志
tail -f ./logs/auth-access.log
```

## 日志分析

### 识别和封禁滥用用户

1. **查看下载频率最高的用户**:

```bash
# 从 Go 日志中提取用户 ID
docker-compose logs go-emby2openlist | grep TokenVerify | awk '{print $8}' | sort | uniq -c | sort -rn

# 输出示例:
# 1543 5c76****dc40  (此用户下载了 1543 次)
#  234 a1b2****ef56
#   45 7890****abcd
```

2. **查看某个用户下载了哪些文件**:

```bash
docker-compose logs go-emby2openlist | grep "TokenVerify" | grep "5c76****dc40"

# 输出示例:
# [TokenVerify] Token 验证通过，用户: 5c76****dc40, 文件: /internal/data/Movie/xxx.mkv, IP: 1.2.3.4
```

3. **封禁用户**:

如果发现滥用，可以通过以下方式封禁：
- 在 Emby 中删除或禁用该用户
- 或在 Nginx 中添加 IP 黑名单

## 性能监控

### 查看鉴权统计

```bash
curl http://127.0.0.1:8097/api/stats

# 输出示例:
{
  "total_requests": 12345,
  "success_requests": 12000,
  "failed_requests": 345,
  "fail_reasons": {
    "invalid_api_key": 200,
    "missing_api_key": 145
  },
  "average_duration": 1500000  # 纳秒
}
```

## 故障排查

### 问题 1: 403 Forbidden，但 api_key 正确

**可能原因**:
- api_key 缓存未生效，每次都调用 Emby API 超时
- Emby 服务不可达

**排查**:
```bash
# 检查 Emby 连通性
curl "http://YOUR_EMBY_HOST:8096/emby/System/Info?api_key=YOUR_API_KEY"

# 检查 Go 日志
docker-compose logs go-emby2openlist | grep -E "(VideoAuth|验证 API Key)"
```

### 问题 2: 302 重定向后依然 403

**可能原因**:
- Token 验证失败
- UID 缓存过期

**排查**:
```bash
# 检查 token 验证日志
docker-compose logs go-emby2openlist | grep TokenVerify

# 查看是否有 "无效的 UID" 或 "Token 签名无效" 错误
```

### 问题 3: Nginx 错误 "unknown variable"

**可能原因**:
- Nginx 配置中使用了未定义的变量

**排查**:
```bash
# 测试 Nginx 配置
nginx -t

# 查看 Nginx 错误日志
tail -f /var/log/nginx/error.log
```

## 安全建议

1. **定期轮换 secretKey**: 修改 `internal/service/videoauth/auth.go` 中的 `secretKey`

2. **调整 Token TTL**: 根据实际需求调整 `tokenTTL`（当前 5 分钟）

3. **启用 HTTPS**: 在生产环境中使用 HTTPS 防止中间人攻击

4. **限制请求频率**: 在 Nginx 中添加 `limit_req` 防止暴力破解

5. **监控异常流量**: 定期检查日志，识别异常访问模式

## 性能优化

1. **API Key 缓存**: 已实现 24 小时 TTL 缓存

2. **UID 缓存**: 已实现 10 分钟 TTL 缓存

3. **Nginx sendfile**: 已启用 `sendfile on` 加速文件传输

4. **禁用 gzip**: 视频文件已压缩，禁用 gzip 节省 CPU

## 总结

方案1 提供了强大的安全性和完整的审计日志，能够：

✅ 防止 URL 伪造（HMAC 签名）
✅ 限制 URL 分享（5 分钟过期）
✅ 追踪用户行为（UID + 日志）
✅ 支持用户封禁（基于日志分析）
✅ 高性能（缓存 + sendfile）

适用于需要严格控制视频访问权限的场景。
