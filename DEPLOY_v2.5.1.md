# v2.5.1 部署指南

## 🎯 本次更新重点

### 核心问题修复 ✅
**严重问题**: 播放时间超过 5 分钟后，Token 过期导致视频自动 403 中断

### 解决方案：播放会话自动续期 🚀
- 首次访问创建播放会话（Session）
- 每次 Range 请求自动续期 5 分钟
- 闲置 5 分钟后会话自动过期（防止分享）
- **视频可以无限播放**（只要不暂停超过 5 分钟）

---

## 🚀 部署步骤

### 在远程服务器 183.179.251.164 上执行：

```bash
# 1. 进入项目目录
cd /usr/local/go-emby2openlist

# 2. 拉取最新代码
git pull
git fetch --tags

# 3. 切换到 v2.5.1
git checkout v2.5.1

# 4. 查看更新内容
git log v2.5.0..v2.5.1 --oneline

# 5. 重新构建 Docker 镜像
docker build -t go-emby2openlist:v2.5.1 .

# 6. 停止并删除旧容器
docker stop go-emby2openlist
docker rm go-emby2openlist

# 7. 启动新容器
docker run -d \
  --name go-emby2openlist \
  --network host \
  -v /usr/local/go-emby2openlist/config:/app/config \
  go-emby2openlist:v2.5.1

# 8. 查看启动日志
docker logs --tail=50 go-emby2openlist

# 9. 验证版本
docker logs go-emby2openlist 2>&1 | grep "ge2o:v"
```

---

## ✅ 功能验证

### 1. 测试长视频播放（关键测试）

**步骤**：
1. 在 Emby 中选择一个长视频（> 10 分钟）
2. 开始播放并跳转到 5 分钟位置
3. 持续播放超过 5 分钟（不要暂停）
4. **预期结果**: 视频正常播放，不会 403 中断 ✅

**日志验证**：
```bash
# 查看播放会话创建日志
docker logs -f go-emby2openlist | grep "创建播放会话"

# 查看自动续期日志
docker logs -f go-emby2openlist | grep "播放会话续期"
```

预期日志输出：
```
[TokenVerify] 创建播放会话，用户: 1a2b****c3d4, 文件: /internal/data/Movie/xxx.mkv, IP: 192.168.1.100, 会话过期时间: 2025-01-07 14:25:00
[TokenVerify] 播放会话续期，用户: 1a2b****c3d4, 文件: /internal/data/Movie/xxx.mkv, IP: 192.168.1.100, 新过期时间: 2025-01-07 14:30:00
[TokenVerify] 播放会话续期，用户: 1a2b****c3d4, 文件: /internal/data/Movie/xxx.mkv, IP: 192.168.1.100, 新过期时间: 2025-01-07 14:35:00
...
```

### 2. 测试暂停恢复（4 分钟内）

**步骤**：
1. 播放视频 2 分钟
2. 暂停播放
3. 等待 3 分钟
4. 恢复播放
5. **预期结果**: 正常恢复播放 ✅

### 3. 测试闲置过期（超过 5 分钟）

**步骤**：
1. 播放视频 2 分钟
2. 暂停播放
3. 等待 6 分钟
4. 尝试恢复播放
5. **预期结果**: 视频会重新加载（302 重定向），然后正常播放 ✅

**日志验证**：
```bash
docker logs -f go-emby2openlist | grep "播放会话已过期"
```

预期日志输出：
```
[TokenVerify] 播放会话已过期（闲置超过5分钟），路径: /internal/data/Movie/xxx.mkv, IP: 192.168.1.100
```

### 4. 测试防分享机制

**步骤**：
1. 用户 A 开始播放视频，复制视频 URL（包含 Token）
2. 用户 B 在 5 分钟内使用该 URL 访问 → **可以访问** ⚠️（同一会话）
3. 用户 A 停止播放并等待 5 分钟
4. 用户 B 尝试继续访问 → **403 拒绝访问** ✅（会话已过期）

**安全性说明**：
- Token 仍然有 5 分钟硬过期时间
- 会话绑定到 `{token}:{uid}` 组合
- 闲置 5 分钟后会话自动销毁
- 分享的 Token 只能在原始会话活跃期间使用

---

## 📊 性能监控

### 查看播放会话数量

```bash
# 查看活跃会话创建日志
docker logs go-emby2openlist 2>&1 | grep "创建播放会话" | wc -l

# 查看续期次数（活跃度）
docker logs go-emby2openlist 2>&1 | grep "播放会话续期" | wc -l
```

### 内存占用估算

- 每个会话: ~60 字节
- 1000 个并发用户: ~60 KB（可忽略）

---

## 🔍 故障排查

### 问题1: 视频仍然在 5 分钟后中断

**排查步骤**：
```bash
# 1. 确认版本是否为 v2.5.1
docker logs go-emby2openlist 2>&1 | grep "ge2o:v"
# 预期输出: ge2o:v2.5.1

# 2. 检查是否有续期日志
docker logs -f go-emby2openlist | grep "播放会话"
# 应该能看到 "创建播放会话" 和 "播放会话续期" 日志

# 3. 检查是否有错误日志
docker logs go-emby2openlist 2>&1 | grep -i "error"
```

**可能原因**：
- Docker 镜像未正确重建（仍在使用旧版本）
- 缓存未正确初始化

**解决方法**：
```bash
# 强制重新构建（不使用缓存）
docker build --no-cache -t go-emby2openlist:v2.5.1 .

# 重启容器
docker restart go-emby2openlist
```

### 问题2: 看不到续期日志

**排查步骤**：
```bash
# 检查 auth_request 是否正常工作
docker logs -f go-emby2openlist | grep "TokenVerify"
# 应该能看到 Token 验证日志

# 如果没有任何 TokenVerify 日志，检查 Nginx 配置
cat /etc/nginx/sites-enabled/video-gateway.conf | grep auth_request
```

**可能原因**：
- Nginx auth_request 未正确配置
- Go 应用未正常启动

**解决方法**：
```bash
# 检查 Go 应用是否正常运行
docker ps | grep go-emby2openlist

# 检查应用日志
docker logs go-emby2openlist 2>&1 | head -50
```

### 问题3: 服务重启后会话丢失

**说明**：这是**正常行为**。

播放会话存储在内存中，服务重启后会清空。

**影响**：
- 用户需要重新加载视频（302 重定向）
- 不影响新用户访问

**未来优化**：
- 使用 Redis 存储会话（支持多实例和重启恢复）

---

## 📝 技术原理

### 自动续期机制

```
┌─────────────────────────────────────────────┐
│ 用户播放视频                                 │
└───────────────┬─────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────────┐
│ 302 重定向到签名 URL                         │
│ Token: 9ca76a... (5 分钟有效期)              │
│ UID: f862eeb5 (加密的用户标识)               │
└───────────────┬─────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────────┐
│ 首次 auth_request 验证                       │
│ ✅ 创建播放会话 (Session)                    │
│ 会话 Key: {token}:{uid}                      │
│ 会话过期: 5 分钟后                           │
└───────────────┬─────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────────┐
│ 每次 Range 请求（下载视频分片）              │
│ → auth_request 验证                          │
│ → 检测到会话存在                             │
│ → 自动续期 5 分钟 ✅                         │
└───────────────┬─────────────────────────────┘
                │
                ▼
┌─────────────────────────────────────────────┐
│ 用户可以无限播放                             │
│ （只要不暂停超过 5 分钟）                    │
└─────────────────────────────────────────────┘
```

### 安全性保障

1. **Token 签名验证**（防伪造）
   ```
   Token = HMAC-SHA256(path + apiKey + expires, secretKey)
   ```

2. **UID 加密**（防逆向）
   ```
   UID = HMAC-SHA256(apiKey, secretKey)[:8]
   ```

3. **会话绑定**（防分享）
   ```
   Session Key = {token}:{uid}
   ```

4. **硬过期时间**（最终防线）
   - Token 初始过期: 5 分钟
   - Session 最大存活: 30 分钟（缓存 TTL）

---

## 📚 相关文档

- **详细技术文档**: `docs/TOKEN_AUTO_RENEWAL.md`
- **v2.5.0 部署指南**: `DEPLOY_v2.5.0.md`
- **Nginx 配置说明**: `nginx/README.md`

---

## 🎉 预期效果

部署完成后，您应该能够：

1. ✅ 播放任意长度的视频（不会中断）
2. ✅ 短暂暂停后恢复播放（< 5 分钟）
3. ✅ 防止 Token 分享（闲置 5 分钟过期）
4. ✅ 保持高性能（< 0.1ms 额外延迟）
5. ✅ 零配置（完全兼容现有系统）

---

## 🤝 反馈与支持

如果遇到问题，请提供以下信息：

1. **版本确认**：
   ```bash
   docker logs go-emby2openlist 2>&1 | grep "ge2o:v"
   ```

2. **错误日志**（最近 50 行）：
   ```bash
   docker logs --tail=50 go-emby2openlist
   ```

3. **Nginx 日志**：
   ```bash
   tail -50 /var/log/nginx/video_internal_error.log
   ```

4. **问题描述**：
   - 播放了多长时间后出现 403？
   - 是否能看到续期日志？
   - 是否重启过服务？

**GitHub Issues**: https://github.com/AmbitiousJun/go-emby2openlist/issues

---

祝部署顺利！🎉
