# 🚀 快速开始指南

本指南将帮助你在 5 分钟内完成 go-emby2openlist 的部署。

---

## 📋 前置要求

- ✅ Docker 和 Docker Compose 已安装
- ✅ Emby 服务器已部署（知道访问地址和管理员 API Key）
- ✅ 至少一台 Nginx 服务器（用于提供视频文件）

---

## 🎯 步骤 1: 下载配置文件

```bash
# 创建工作目录
mkdir go-emby2openlist && cd go-emby2openlist

# 下载配置示例
wget https://raw.githubusercontent.com/zerolin1010/go-emby2openlist/main/config-example.yml -O config.yml
wget https://raw.githubusercontent.com/zerolin1010/go-emby2openlist/main/docker-compose-example.yml -O docker-compose.yml
```

---

## 🎯 步骤 2: 修改配置

编辑 `config.yml`，填写你的配置：

```yaml
# Emby 服务器配置
emby:
  host: http://your-emby-server:8096        # 改为你的 Emby 地址
  admin-api-key: "your-admin-api-key"       # 改为你的管理员 API Key
  mount-path: /media                        # Emby 媒体挂载路径

# 节点配置
nodes:
  health-check:
    interval: 30
    timeout: 5
    fail-threshold: 3
    success-threshold: 2
  list:
    - name: "node-1"
      host: "http://nginx-server:80"        # 改为你的 Nginx 地址
      weight: 100
      enabled: true

# 路径映射（Emby路径 → Nginx路径）
path:
  emby2nginx:
    - /media/data:/video/data               # 根据实际情况修改
```

**如何获取 Emby Admin API Key**：
1. 登录 Emby 后台
2. 设置 → API Keys → 创建新 API Key
3. 复制 API Key

---

## 🎯 步骤 3: 启动服务

```bash
# 启动容器
docker-compose up -d

# 查看日志
docker logs -f go-emby2openlist
```

**正常启动日志示例**：
```
正在初始化节点健康检查模块...
正在初始化用户 Key 缓存模块...
正在启动主服务...
[GIN] Listening on :8095
```

---

## 🎯 步骤 4: 验证服务

### 4.1 检查主服务

```bash
curl http://localhost:8095
```

应该返回类似 Emby 的响应。

### 4.2 检查节点健康

```bash
docker logs go-emby2openlist 2>&1 | grep "健康检查"
```

应该看到节点健康检查的日志。

### 4.3 测试 302 重定向

```bash
# 替换为你的 Emby Item ID 和 API Key
curl -I "http://localhost:8095/emby/Items/{ItemId}/Download?api_key={YourApiKey}"
```

应该返回 `HTTP/1.1 302 Found`，Location 指向 Nginx 节点。

---

## 🎯 步骤 5: 配置 Nginx（每个节点）

在你的 Nginx 服务器上创建配置：

```nginx
# /etc/nginx/sites-available/emby-video

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
        alias /mnt/media/;  # 你的媒体存储路径

        # Range 请求支持（视频拖拽）
        add_header 'Accept-Ranges' bytes always;

        # CORS 支持
        add_header 'Access-Control-Allow-Origin' '*' always;
        add_header 'Access-Control-Allow-Methods' 'GET, OPTIONS' always;
        add_header 'Access-Control-Allow-Headers' 'Range' always;

        # 性能优化
        sendfile on;
        tcp_nopush on;
        tcp_nodelay on;
        directio 512;
        output_buffers 8 256k;
    }
}
```

**启用配置**：

```bash
sudo ln -s /etc/nginx/sites-available/emby-video /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

---

## 🎯 步骤 6: 配置 Emby 客户端

### Web 客户端

在浏览器中访问：`http://your-server-ip:8095`

### 移动客户端

1. 打开 Emby 客户端
2. 添加服务器：`http://your-server-ip:8095`
3. 输入用户名和密码登录

---

## ✅ 验证完整流程

播放一个视频，然后：

1. **查看 go-emby2openlist 日志**：
   ```bash
   docker logs -f go-emby2openlist
   ```
   应该看到 302 重定向日志

2. **查看 Nginx 日志**：
   ```bash
   tail -f /var/log/nginx/access.log
   ```
   应该看到客户端直接请求 Nginx

3. **检查网络流量**：
   go-emby2openlist 服务器的流量应该很小（只有控制请求），大部分流量在 Nginx

---

## 🆘 常见问题

### 问题 1: 节点显示不健康

**检查健康接口**：
```bash
curl -v -H "Host: gtm-health" http://nginx-server-ip/gtm-health
```

应该返回 200 OK。

**解决方案**：
- 确保 Nginx 配置了健康检查接口
- 检查防火墙是否允许访问

### 问题 2: 302 重定向后 404

**检查路径映射**：
```yaml
# config.yml
path:
  emby2nginx:
    - /media/data:/video/data

# Emby 实际路径
/media/data/movies/test.mp4

# Nginx 应该能访问
http://nginx/video/data/movies/test.mp4
```

**解决方案**：
- 确保路径映射正确
- 确保 Nginx alias 配置正确

### 问题 3: 视频无法拖拽

**检查 Range 支持**：
```bash
curl -I -H "Range: bytes=0-1023" http://nginx/video/data/test.mp4
```

应该返回 `HTTP/1.1 206 Partial Content`。

**解决方案**：
- 确保 Nginx 配置了 `Accept-Ranges: bytes`
- 检查 `sendfile` 和 `directio` 配置

---

## 🎉 恭喜！

你已经成功部署了 go-emby2openlist！

### 下一步

- 📖 [启用后端鉴权服务器](./docs/AUTH_SERVER_QUICKSTART.md)
- 📖 [配置 Telegram Bot 管理](./docs/TELEGRAM_BOT.md)
- 📖 [查看完整架构文档](./docs/ARCHITECTURE.md)
- 📖 [Nginx 鉴权方案对比](./docs/NGINX_AUTH.md)

### 需要帮助？

- 📝 [提交 Issue](https://github.com/zerolin1010/go-emby2openlist/issues)
- 📖 [完整文档](./README.md)
- 🔍 [故障排查指南](./docs/TESTING_GUIDE.md)

---

<div align="center">
  Made with ❤️ by zerolin1010
</div>
