# Nginx 配置说明

本目录包含用于配合 go-emby2openlist 项目的 Nginx 配置示例。

## 文件说明

- `video.conf` - 视频服务配置文件示例

## 安装步骤

### 1. 复制配置文件

```bash
# 将配置文件复制到 Nginx 配置目录
sudo cp video.conf /etc/nginx/conf.d/video.conf
```

### 2. 修改配置

根据你的实际环境修改以下配置项：

```nginx
# 修改视频文件根目录
root /data/media;  # 改为你的实际路径

# 修改 alias 路径
location /video/ {
    alias /data/media/;  # 改为你的实际路径
}
```

### 3. 测试配置

```bash
# 测试配置文件语法
sudo nginx -t
```

### 4. 重载 Nginx

```bash
# 如果测试通过，重载配置
sudo nginx -s reload
```

## 健康检查测试

使用以下命令测试健康检查接口：

```bash
# 标准请求方式
curl -v -H "Host: gtm-health" http://<服务器IP>/gtm-health

# 应该返回 200 OK
```

## 目录结构要求

确保你的媒体目录结构符合以下格式：

```
/data/media/
├── movie/               # 电影
│   └── Movie Name (2024)/
│       └── Movie.Name.2024.mp4
├── series/              # 电视剧
│   └── Series Name/
│       └── Season 01/
│           └── S01E01.mp4
└── music/               # 音乐
    └── Artist/
        └── Album/
            └── track.mp3
```

## Range 请求支持

配置已默认启用 Range 请求支持，客户端可以：

- 拖拽视频进度条
- 断点续传
- 分段下载

## CORS 跨域支持

配置已包含完整的 CORS 支持，允许：

- Web 播放器跨域访问
- 所有常见的 Emby 请求头
- OPTIONS 预检请求

**⚠️ 生产环境建议：**

将 `*` 改为具体的域名：

```nginx
set $cors_origin 'https://your-domain.com';
```

## 性能优化

配置已包含以下优化：

- `sendfile on` - 零拷贝文件传输
- `tcp_nopush on` - 减少网络包数量
- `directio 512` - 大文件直接 IO
- `output_buffers 1 1m` - 输出缓冲优化

## 故障排查

### 1. 403 Forbidden

检查文件权限：

```bash
sudo chmod -R 755 /data/media
sudo chown -R nginx:nginx /data/media  # CentOS
# 或
sudo chown -R www-data:www-data /data/media  # Ubuntu/Debian
```

### 2. Range 请求不工作

确保：

- 文件存在且可读
- 未启用 gzip 压缩
- 返回了 `Accept-Ranges: bytes` 响应头

### 3. CORS 错误

检查：

- CORS 头是否正确添加
- OPTIONS 请求是否返回 204
- 浏览器控制台的具体错误信息

## 日志位置

- 访问日志: `/var/log/nginx/access.log`
- 错误日志: `/var/log/nginx/error.log`

## 端口说明

- `80` - HTTP 服务端口
- 健康检查也使用 `80` 端口，通过 `Host` 头区分

## 更多帮助

如有问题，请查看项目主 README 或提交 Issue。
