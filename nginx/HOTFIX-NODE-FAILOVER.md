# Nginx 节点故障转移修复指南

## 问题描述

当通过 Telegram 手动禁用节点后，系统没有正确触发故障转移，客户端仍然继续访问被禁用的节点。

## 根本原因

Nginx 配置中的 `X-Node-Host` 头使用了错误的变量：

```nginx
# ❌ 错误配置
proxy_set_header X-Node-Host "$host:$server_port";
```

**问题**：`$host` 获取的是请求中的 Host 头（可能是域名或客户端使用的地址），而不是 Nginx 节点的真实服务器地址。这导致认证服务器无法正确识别是哪个节点发来的请求，从而无法判断该节点是否被禁用。

## 修复方案

将 `$host` 改为 `$server_addr`：

```nginx
# ✅ 正确配置
proxy_set_header X-Node-Host "$server_addr:$server_port";
```

**说明**：
- `$server_addr` - Nginx 服务器接收请求的本地IP地址（真实的服务器IP）
- `$server_port` - Nginx 监听的端口号

这样认证服务器就能准确识别请求来自哪个节点，从而正确判断节点是否被禁用。

## 应用步骤

### 1. 定位需要修改的文件

在每个 Nginx 节点服务器上找到配置文件（通常在以下位置之一）：
- `/etc/nginx/conf.d/video-gateway.conf`
- `/etc/nginx/sites-enabled/video-gateway.conf`
- 或您自定义的配置文件位置

### 2. 修改配置文件

找到 `location = /auth-verify` 区块，将：

```nginx
proxy_set_header X-Node-Host "$host:$server_port";
```

修改为：

```nginx
# 修复：使用 $server_addr 获取真实的服务器IP地址
proxy_set_header X-Node-Host "$server_addr:$server_port";
```

**完整示例**（修改后的配置）：

```nginx
location = /auth-verify {
    internal;

    set $video_path $request_uri;
    if ($video_path ~ "^([^?]+)") {
        set $video_path $1;
    }

    proxy_pass http://127.0.0.1:8097/api/verify-token?token=$token&expires=$expires&uid=$uid&path=$video_path;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header Host $host;
    proxy_set_header X-Node-Host "$server_addr:$server_port";  # ← 这里修改
    proxy_set_header X-Original-URI $request_uri;
    proxy_set_header X-Real-IP $remote_addr;
}
```

### 3. 验证配置语法

```bash
nginx -t
```

### 4. 重载 Nginx 配置

```bash
nginx -s reload
# 或者
systemctl reload nginx
```

### 5. 验证修复

修改后，可以通过以下方式验证：

1. **查看认证服务器日志**：
   ```bash
   docker logs go-emby2openlist | grep "VideoAuth.*检查节点健康"
   ```

2. **通过 Telegram 禁用一个节点**，观察日志应该显示：
   ```
   [WARN] [VideoAuth] 匹配到被禁用的节点: node-xxx, 返回不健康以触发故障转移
   [INFO] [TokenVerify] 故障转移到新节点: node-yyy
   ```

3. **测试播放**：禁用节点后，正在播放的视频应该自动切换到其他健康节点。

## 批量部署脚本

如果您有多个 Nginx 节点需要修改，可以使用以下脚本（请根据实际情况调整）：

```bash
#!/bin/bash
# 批量修复 Nginx 配置

NGINX_CONF="/etc/nginx/conf.d/video-gateway.conf"

# 备份原配置
cp $NGINX_CONF ${NGINX_CONF}.backup.$(date +%Y%m%d_%H%M%S)

# 执行替换
sed -i 's/proxy_set_header X-Node-Host "\$host:\$server_port";/proxy_set_header X-Node-Host "\$server_addr:\$server_port";/g' $NGINX_CONF

# 验证语法
if nginx -t; then
    echo "配置语法检查通过"
    nginx -s reload
    echo "Nginx 已重载"
else
    echo "配置语法错误，请检查！"
    # 恢复备份
    cp ${NGINX_CONF}.backup.* $NGINX_CONF
fi
```

## 注意事项

1. **端口映射问题**：如果您的 Nginx 节点使用了端口映射（例如 Docker 容器内监听 7777，但对外暴露 46621），`$server_port` 会是容器内的端口（7777）。在这种情况下，您可能需要：

   - 方案A：直接硬编码节点地址（推荐）
     ```nginx
     proxy_set_header X-Node-Host "120.79.200.215:46621";  # 使用节点的公网IP和端口
     ```

   - 方案B：使用环境变量（需要在 Nginx 启动时传入）
     ```nginx
     proxy_set_header X-Node-Host "$PUBLIC_IP:$PUBLIC_PORT";
     ```

2. **如果使用 Docker 部署 Nginx**：确保容器能够正确获取宿主机的IP地址。

3. **修改后需要等待**：
   - 已建立的播放会话会继续使用旧节点（最长5分钟过期）
   - 新的播放请求会立即使用新的故障转移逻辑

## 已修复的配置文件

本仓库中已修复的配置文件：
- ✅ `nginx/video-gateway-URL-DECODE-FIX.conf` - 已修复（第211行）
- ➖ `nginx/video-gateway-SIMPLE.conf` - 不需要修改（未使用 auth_request）

## 参考文档

- [Nginx 核心变量文档](http://nginx.org/en/docs/varindex.html)
- `$server_addr` - 接收请求的服务器地址
- `$server_port` - 接收请求的服务器端口
- `$host` - 请求行中的主机名，或Host请求头字段中的主机名

---

**修复日期**: 2025-12-07
**影响版本**: v2.6.3 及更早版本
**修复人员**: Claude Code
