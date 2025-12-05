<div align="center">
  <img height="150px" src="./assets/logo.png"></img>
</div>

<h1 align="center">go-emby2openlist</h1>

<div align="center">
  <a href="https://github.com/zerolin1010/go-emby2openlist/releases/latest"><img src="https://img.shields.io/github/v/tag/zerolin1010/go-emby2openlist"></img></a>
  <a href="https://hub.docker.com/r/zerolin1010/go-emby2openlist/tags"><img src="https://img.shields.io/docker/image-size/zerolin1010/go-emby2openlist/latest"></img></a>
  <a href="https://hub.docker.com/r/zerolin1010/go-emby2openlist/tags"><img src="https://img.shields.io/docker/pulls/zerolin1010/go-emby2openlist"></img></a>
  <a href="https://github.com/zerolin1010/go-emby2openlist/releases/latest"><img src="https://img.shields.io/github/downloads/zerolin1010/go-emby2openlist/total"></img></a>
  <img src="https://img.shields.io/github/stars/zerolin1010/go-emby2openlist"></img>
  <img src="https://img.shields.io/github/license/zerolin1010/go-emby2openlist"></img>
</div>

<div align="center">
  Emby 反向代理服务 - 本地 Nginx 多节点 CDN 模式
</div>

---

## 📢 重要更新 v2.4.1

**🎉 项目已从 OpenList 网盘模式改造为本地 Nginx 多节点 CDN 模式！**

### ✨ 新架构特性

- ✅ **多节点 CDN 支持** - 类似 CDN 的多节点架构
- ✅ **自动健康检查** - 实时监控节点状态，自动故障转移
- ✅ **加权负载均衡** - 智能分配请求到不同节点
- ✅ **后端鉴权服务器** - 集中式鉴权，支持 Nginx auth_request（v2.4.0 新功能）
- ✅ **访问日志和统计** - JSON 格式日志，实时统计 API（v2.4.0 新功能）
- ✅ **Telegram Bot 管理** - 远程管理节点，动态添加/删除
- ✅ **302 重定向** - 客户端直连 Nginx，不消耗代理服务器带宽

### 📚 迁移指南

如果你从旧版本升级，请查看：[MIGRATION_GUIDE.md](./MIGRATION_GUIDE.md)

---

## 🎯 核心功能

### 1. 多节点健康检查

- 自动定期检查所有节点状态
- 可配置失败/成功阈值
- 节点故障自动切换
- 支持节点禁用/启用

### 2. 智能负载均衡

- 加权随机选择算法
- 自动排除不健康节点
- 支持动态调整权重
- 并发安全，支持高并发场景

### 3. Telegram Bot 管理

通过 Telegram 远程管理 CDN 节点：

- `/list` - 查看所有节点
- `/status` - 实时健康状态
- `/add` - 动态添加节点
- `/del` - 删除节点
- `/enable` / `/disable` - 启用/禁用节点

详细使用：[Telegram Bot 文档](./docs/TELEGRAM_BOT.md)

### 4. 后端鉴权服务器（v2.4.0 新增）

可选的集中式鉴权服务，用于 Nginx `auth_request` 集成：

- **端口**: 8097（可配置）
- **功能**: API Key 验证，访问日志，实时统计
- **性能**: 支持 API Key 缓存，异步日志写入
- **安全**: API Key 自动脱敏记录

**使用场景**：
- Nginx 收到播放请求 → 调用后端 `/api/auth` 验证
- 后端验证通过 → Nginx 返回 302 重定向
- 所有鉴权由后端集中处理，Nginx 专注文件服务

详细文档：
- 📖 [后端鉴权架构](./docs/BACKEND_AUTH_ARCHITECTURE.md)
- 📖 [鉴权服务器使用指南](./docs/AUTH_SERVER.md)
- 📖 [5分钟快速开始](./docs/AUTH_SERVER_QUICKSTART.md)
- 📖 [Nginx 鉴权方案对比](./docs/NGINX_AUTH.md)

### 5. 其他功能

- ✅ Strm 直链播放
- ✅ WebSocket 代理
- ✅ 客户端防转码
- ✅ 响应缓存中间件
- ✅ 字幕缓存（30天）
- ✅ CORS 跨域支持
- ✅ Range 请求支持（视频拖拽）

---

## 📋 工作原理

### 传统模式（消耗服务器带宽）

```
客户端 → Emby 服务器 → 读取本地视频 → 返回客户端
       ↓
    消耗服务器上传带宽
```

### 新模式（302 重定向，不消耗带宽）

```
1. 客户端 → go-emby2openlist → Emby API → 获取视频路径
2. go-emby2openlist → 路径映射 → 选择健康节点
3. 返回 302 重定向 → 客户端直连 Nginx 节点
4. 客户端 ← Nginx 节点 ← 本地视频文件

✅ 代理服务器只处理控制请求，不转发视频流
✅ 带宽消耗转移到 Nginx CDN 节点
```

---

## 🚀 快速开始

### 前置要求

1. ✅ 已部署 Emby 服务器
2. ✅ 至少一台 Nginx 服务器（用于提供视频文件）
3. ✅ 视频文件存储在本地磁盘
4. ✅ 服务器已安装 Docker

### 安装步骤

#### 方式 1: 使用 Docker Compose（推荐）

1. **创建配置文件**

```bash
mkdir go-emby2openlist && cd go-emby2openlist
wget https://raw.githubusercontent.com/zerolin1010/go-emby2openlist/main/config-example.yml -O config.yml
```

2. **修改配置**

编辑 `config.yml`，配置你的 Emby 服务器和 Nginx 节点：

```yaml
emby:
  host: http://your-emby-server:8096
  admin-api-key: "your-admin-api-key"
  mount-path: /media

nodes:
  health-check:
    interval: 30
    timeout: 5
    fail-threshold: 3
    success-threshold: 2
  list:
    - name: "node-1"
      host: "http://nginx-server-1:80"
      weight: 100
      enabled: true

path:
  emby2nginx:
    - /media/data:/video/data
    - /media/series:/video/series
```

3. **创建 docker-compose.yml**

```yaml
version: '3.1'
services:
  go-emby2openlist:
    image: zerolin1010/go-emby2openlist:latest
    container_name: go-emby2openlist
    restart: always
    volumes:
      - ./config.yml:/app/config.yml
      - ./logs:/app/logs              # 可选：日志目录（如果启用鉴权服务器日志）
      # - ./ssl:/app/ssl              # 可选：SSL 证书（如果启用 HTTPS）
    ports:
      - 8095:8095                     # HTTP 服务（必需）
      - 8094:8094                     # HTTPS 服务（可选，需要配置 SSL）
      - 8097:8097                     # 鉴权服务器（可选，如果启用后端鉴权）
    environment:
      - TZ=Asia/Shanghai
      - GIN_MODE=release              # 生产模式（可选）
```

**端口说明**：
- `8095`: 主 HTTP 服务（必需）- Emby 客户端连接此端口
- `8094`: HTTPS 服务（可选）- 需要在 config.yml 中配置 SSL
- `8097`: 鉴权服务器（可选）- 仅在启用 `enable-auth-server: true` 时需要

**卷挂载说明**：
- `./config.yml`: 配置文件（必需）
- `./logs`: 日志目录（可选）- 仅在启用 `enable-auth-server-log: true` 时需要
- `./ssl`: SSL 证书（可选）- 仅在启用 HTTPS 时需要

4. **启动服务**

```bash
docker-compose up -d
```

5. **查看日志**

```bash
docker logs -f go-emby2openlist
```

6. **验证服务**

```bash
# 检查主服务（HTTP）
curl http://localhost:8095

# 检查鉴权服务器（如果启用）
curl http://localhost:8097/api/health
# 应返回: {"service":"auth-server","status":"ok"}

# 查看鉴权统计（如果启用）
curl http://localhost:8097/api/stats
# 应返回: {"success_count":0,"failure_count":0,"last_update":"..."}
```

#### 方式 2: 直接使用 Docker（不使用 Compose）

```bash
docker pull zerolin1010/go-emby2openlist:latest

docker run -d \
  --name go-emby2openlist \
  --restart always \
  -v $(pwd)/config.yml:/app/config.yml \
  -v $(pwd)/logs:/app/logs \
  -p 8095:8095 \
  -p 8094:8094 \
  -p 8097:8097 \
  -e TZ=Asia/Shanghai \
  -e GIN_MODE=release \
  zerolin1010/go-emby2openlist:latest
```

#### 方式 3: 从源码编译

```bash
# 克隆仓库
git clone https://github.com/zerolin1010/go-emby2openlist.git
cd go-emby2openlist

# 编译
go build -o go-emby2openlist

# 运行
./go-emby2openlist
```

---

## 🔐 后端鉴权服务器配置（可选）

### 为什么需要鉴权服务器？

如果您需要以下功能，建议启用鉴权服务器：

- ✅ **集中式鉴权** - 所有节点的鉴权由后端统一处理
- ✅ **详细访问日志** - JSON 格式，记录每次访问（API Key 自动脱敏）
- ✅ **实时统计** - 查看成功/失败次数，监控系统使用情况
- ✅ **Nginx auth_request** - Nginx 通过后端验证，无需在配置中硬编码 API Key

### 快速启用（3 步）

#### 1. 修改 config.yml

```yaml
auth:
  user-key-cache-ttl: 24h
  nginx-auth-enable: true

  # 启用鉴权服务器
  enable-auth-server: true           # 改为 true
  auth-server-port: "8097"
  enable-auth-server-log: true
  auth-server-log-path: "./logs/auth-access.log"
```

#### 2. 更新 docker-compose.yml

确保暴露 8097 端口和挂载日志目录：

```yaml
ports:
  - 8095:8095
  - 8094:8094
  - 8097:8097    # 鉴权服务器端口
volumes:
  - ./config.yml:/app/config.yml
  - ./logs:/app/logs    # 日志目录
```

#### 3. 重启服务

```bash
docker-compose down
docker-compose up -d
```

#### 4. 验证鉴权服务器

```bash
# 健康检查
curl http://localhost:8097/api/health

# 测试鉴权（替换 YOUR_API_KEY）
curl "http://localhost:8097/api/auth?api_key=YOUR_API_KEY"

# 查看统计
curl http://localhost:8097/api/stats

# 查看日志
tail -f logs/auth-access.log
```

### Nginx 集成

修改 Nginx 配置，使用 `auth_request` 调用后端鉴权：

```nginx
upstream auth_backend {
    server go-emby2openlist:8097;
    keepalive 32;
}

# 鉴权子请求
location = /auth {
    internal;
    proxy_pass http://auth_backend/api/auth?api_key=$arg_api_key;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
}

# 视频服务
location /video/data {
    auth_request /auth;    # 使用后端鉴权
    auth_request_set $auth_status $upstream_status;

    alias /mnt/media/;
    add_header X-Auth-Status $auth_status;
}
```

完整配置示例：[nginx/video-with-backend-auth.conf](./nginx/video-with-backend-auth.conf)

### API 接口

鉴权服务器提供 3 个 API：

| 接口 | 方法 | 说明 |
|-----|------|------|
| `/api/auth` | GET | 验证 API Key（Nginx auth_request） |
| `/api/stats` | GET | 获取统计信息 |
| `/api/health` | GET | 健康检查 |

详细文档：[AUTH_SERVER.md](./docs/AUTH_SERVER.md)

---

## 🔧 Nginx 配置

### 1. 安装 Nginx

```bash
# Ubuntu/Debian
sudo apt update && sudo apt install nginx

# CentOS/RHEL
sudo yum install nginx
```

### 2. 配置 Nginx

参考配置示例：[nginx/video.conf](./nginx/video.conf)

```nginx
# 健康检查接口
server {
    listen 80;
    server_name gtm-health;

    location = /gtm-health {
        access_log off;
        return 200 'OK';
        add_header Content-Type text/plain;
    }
}

# 视频服务
server {
    listen 80 default_server;

    location /video/ {
        alias /data/media/;

        # Range 请求支持
        add_header 'Accept-Ranges' bytes always;

        # CORS 支持
        add_header 'Access-Control-Allow-Origin' '*' always;

        # 性能优化
        sendfile on;
        tcp_nopush on;
        directio 512;
    }
}
```

详细配置：[Nginx 配置文档](./nginx/README.md)

---

## 📊 配置说明

### 核心配置项

#### Emby 配置

```yaml
emby:
  host: http://emby-server:8096        # Emby 访问地址
  admin-api-key: "your-admin-api-key"  # 管理员 API Key
  mount-path: /media                   # 媒体挂载路径
```

#### 节点配置

```yaml
nodes:
  health-check:
    interval: 30              # 检查间隔（秒）
    timeout: 5                # 超时时间（秒）
    fail-threshold: 3         # 失败阈值
    success-threshold: 2      # 成功阈值

  list:
    - name: "node-1"
      host: "http://1.2.3.4:80"
      weight: 100              # 权重（1-100）
      enabled: true
```

#### 路径映射

```yaml
path:
  emby2nginx:
    - /media/data:/video/data      # Emby路径:Nginx路径
    - /media/series:/video/series
```

#### 鉴权配置

```yaml
auth:
  user-key-cache-ttl: 24h           # API Key 缓存时间
  nginx-auth-enable: true           # 启用 Nginx 鉴权检查

  # 后端鉴权服务器（可选，v2.4.0 新增）
  enable-auth-server: false         # 是否启用鉴权服务器
  auth-server-port: "8097"          # 鉴权服务器端口
  enable-auth-server-log: true      # 是否记录访问日志
  auth-server-log-path: "./logs/auth-access.log"  # 日志文件路径
```

**何时启用鉴权服务器**：
- ✅ 需要 Nginx 通过 `auth_request` 验证请求
- ✅ 需要详细的访问日志（JSON 格式）
- ✅ 需要实时统计 API（成功/失败次数）
- ❌ 不需要 - 如果使用 URL 参数鉴权或 Emby API 鉴权

参考文档：
- [后端鉴权架构说明](./docs/BACKEND_AUTH_ARCHITECTURE.md)
- [Nginx 鉴权方案对比](./docs/NGINX_AUTH.md)

#### Telegram Bot（可选）

```yaml
telegram:
  enable: true
  bot-token: "your-bot-token"
  admin-users:
    - 123456789
```

完整配置参考：[config-example.yml](./config-example.yml)

---

## 🧪 测试验证

所有核心功能已通过单元测试：

- ✅ 编译测试 - 通过
- ✅ 路径映射 - 8/8 测试通过
- ✅ 健康检查 - 5/5 测试通过
- ✅ 节点选择 - 6/6 测试通过（权重分布精度 ±0.2%）

查看完整测试报告：[TEST_REPORT.md](./TEST_REPORT.md)

---

## 📚 文档

### 架构和设计
- 📖 [完整架构设计](./docs/ARCHITECTURE.md) - 系统架构详解，视频流机制
- 📖 [后端鉴权架构](./docs/BACKEND_AUTH_ARCHITECTURE.md) - 后端鉴权服务器架构说明

### 鉴权相关
- 📖 [鉴权服务器使用指南](./docs/AUTH_SERVER.md) - 完整的 API 文档和配置
- 📖 [5分钟快速开始](./docs/AUTH_SERVER_QUICKSTART.md) - 快速配置鉴权服务器
- 📖 [Nginx 鉴权方案对比](./docs/NGINX_AUTH.md) - 3 种鉴权方案的性能对比

### 配置和测试
- 📖 [迁移指南](./MIGRATION_GUIDE.md) - 从 OpenList 迁移到 Nginx 模式
- 📖 [测试指南](./docs/TESTING_GUIDE.md) - 完整测试步骤
- 📖 [测试报告 v2.4.0](./TEST_REPORT_V2.4.0.md) - 最新版本测试结果
- 📖 [Telegram Bot 文档](./docs/TELEGRAM_BOT.md) - Bot 使用说明
- 📖 [Nginx 配置](./nginx/README.md) - Nginx 详细配置

---

## 🔄 版本更新

### Docker Compose 更新

```bash
docker-compose down
docker-compose pull
docker-compose up -d
```

### 查看版本

```bash
docker exec go-emby2openlist ./main --version
```

---

## 🆘 故障排查

### 问题 1: 所有节点都不健康

**检查健康接口**：
```bash
curl -v -H "Host: gtm-health" http://your-node-ip/gtm-health
```

应该返回：
```
HTTP/1.1 200 OK
OK
```

### 问题 2: 302 重定向后 404

**检查路径映射**：
```yaml
# Emby 路径
/media/data/movie/test.mp4

# 配置映射
path:
  emby2nginx:
    - /media/data:/video/data

# Nginx 实际路径
/mnt/disk/movie/test.mp4  # 宿主机路径

# Nginx 配置
location /video/data {
    alias /mnt/disk/;
}
```

### 问题 3: 视频无法拖拽

**检查 Nginx Range 支持**：
```bash
curl -I -H "Range: bytes=0-1023" http://your-node-ip/video/data/test.mp4
```

应该返回：
```
HTTP/1.1 206 Partial Content
Accept-Ranges: bytes
Content-Range: bytes 0-1023/...
```

更多问题：[故障排查文档](./docs/TESTING_GUIDE.md#故障排查工具)

---

## 🌟 技术栈

- **语言**: Go 1.24+
- **框架**: Gin (HTTP)
- **容器**: Docker + Multi-stage builds
- **CI/CD**: GitHub Actions
- **镜像**: 23.9MB (Alpine-based)
- **平台**: linux/amd64, linux/arm64, linux/arm/v7, linux/386

---

## 📊 性能指标

- **镜像大小**: 23.9MB
- **302 响应延迟**: < 5ms
- **节点选择精度**: ±0.2%
- **并发支持**: 1000+ 并发请求
- **健康检查间隔**: 可配置（默认 30s）

---

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 提交 Pull Request

---

## 📜 开源协议

本项目采用 [MIT License](./LICENSE) 开源协议。

---

## 🔗 相关链接

- **GitHub**: https://github.com/zerolin1010/go-emby2openlist
- **Docker Hub**: https://hub.docker.com/r/zerolin1010/go-emby2openlist
- **问题反馈**: https://github.com/zerolin1010/go-emby2openlist/issues

---

## ⭐ Star History

<a href="https://star-history.com/#zerolin1010/go-emby2openlist&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=zerolin1010/go-emby2openlist&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=zerolin1010/go-emby2openlist&type=Date" />
   <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=zerolin1010/go-emby2openlist&type=Date" />
 </picture>
</a>

---

<div align="center">
  Made with ❤️ by zerolin1010
</div>
