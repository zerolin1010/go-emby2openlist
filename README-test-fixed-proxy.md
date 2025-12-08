# 测试分支：固定前置代理模式

## 与主分支的区别

- **302 重定向**：返回完整 URL `http://cdn.xxx:7777/internal/...`（而非相对路径）
- **移除功能**：节点健康检查、故障转移
- **保留安全**：HMAC-SHA256、Token 验证、会话续期等核心鉴权机制**完全相同**

## 部署流程

```bash
# 1. 切换分支
cd /root/go-emby2openlist
docker compose down
git checkout test-fixed-proxy
git pull origin test-fixed-proxy

# 2. 修改配置文件
vim config.yml
```

**在 `config.yml` 的 `auth` 部分添加：**

```yaml
auth:
  user-key-cache-ttl: 10m
  nginx-auth-enable: true
  enable-auth-server: true
  auth-server-port: "8097"
  enable-auth-server-log: true
  auth-server-log-path: "./logs/auth.log"

  # 新增：固定前置代理 URL（根据实际情况选择 http 或 https）
  fixed-proxy-url: "http://183.179.251.164:7777"
  # 或 fixed-proxy-url: "https://cdn.example.com"
```

```bash
# 3. 重新构建并启动
docker compose build
docker compose up -d

# 4. 查看日志验证
docker logs -f go-emby2openlist | grep "固定前置代理"
```

## 验证成功

播放视频时日志应显示：
```
[INFO] [VideoAuth] 使用固定前置代理: http://183.179.251.164:7777
```

## 回滚到主分支

```bash
cd /root/go-emby2openlist
docker compose down
git checkout main
git pull origin main
docker compose build
docker compose up -d
```

## Docker 网络模式

**当前默认 Host 模式**（性能最好，可访问 127.0.0.1）：
```yaml
network_mode: "host"
```

**可选 Bridge 模式**（需修改 docker-compose.yml 取消注释 ports 部分）：
- 需将 `config.yml` 中 Emby Host 改为宿主机 IP（非 127.0.0.1）
- 可修改端口避免冲突：`"18095:8095"`
