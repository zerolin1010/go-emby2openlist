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

## 📢 重要更新 v2.3.3

**🎉 项目已从 OpenList 网盘模式改造为本地 Nginx 多节点 CDN 模式！**

### ✨ 新架构特性

- ✅ **多节点 CDN 支持** - 类似 CDN 的多节点架构
- ✅ **自动健康检查** - 实时监控节点状态，自动故障转移
- ✅ **加权负载均衡** - 智能分配请求到不同节点
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

### 4. 其他功能

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
wget https://raw.githubusercontent.com/zerolin1010/go-emby2openlist/v2.3.3/config-example.yml -O config.yml
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
    ports:
      - 8095:8095
      - 8094:8094
    environment:
      - TZ=Asia/Shanghai
```

4. **启动服务**

```bash
docker-compose up -d
```

5. **查看日志**

```bash
docker logs -f go-emby2openlist
```

#### 方式 2: 使用现有镜像

```bash
docker pull zerolin1010/go-emby2openlist:latest

docker run -d \
  --name go-emby2openlist \
  --restart always \
  -v $(pwd)/config.yml:/app/config.yml \
  -p 8095:8095 \
  -p 8094:8094 \
  zerolin1010/go-emby2openlist:latest
```

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

- 📖 [迁移指南](./MIGRATION_GUIDE.md) - 从 OpenList 迁移到 Nginx 模式
- 📖 [测试指南](./docs/TESTING_GUIDE.md) - 完整测试步骤
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
