# Nginx 404 故障排查指南

## 问题描述

访问视频 URL 返回 404 错误：
```
http://183.179.251.164:8081/video/data/Movie/动画电影/罗小黑战记 (2019)/罗小黑战记 (2019) - 2160p.H265.DDP 5.1.HDR.mkv?api_key=xxx
```

---

## 排查步骤

### 1️⃣ 检查 Nginx 配置是否生效

```bash
# SSH 登录到源站服务器
ssh root@183.179.251.164

# 检查配置文件是否链接到 sites-enabled
ls -la /etc/nginx/sites-enabled/ | grep video-gateway

# 如果没有，手动创建软链接
sudo ln -s /etc/nginx/sites-available/video-gateway.conf /etc/nginx/sites-enabled/

# 测试 Nginx 配置语法
sudo nginx -t

# 如果语法正确，重新加载 Nginx
sudo nginx -s reload
# 或
sudo systemctl reload nginx
```

### 2️⃣ 检查 Nginx 是否监听 8081 端口

```bash
# 检查端口监听
sudo netstat -tlnp | grep 8081
# 或
sudo ss -tlnp | grep 8081

# 期望输出：
# tcp  0  0  0.0.0.0:8081  0.0.0.0:*  LISTEN  <nginx进程ID>/nginx
```

如果没有监听 8081，说明配置未生效，需要重启 Nginx：

```bash
sudo systemctl restart nginx
```

### 3️⃣ 检查文件路径是否存在

```bash
# 检查媒体目录是否存在
ls -la /media/data/Movie/

# 检查具体文件路径（使用 Tab 补全避免中文路径问题）
ls -lh "/media/data/Movie/动画电影/"

# 或者使用通配符查找
find /media/data/Movie/ -name "*罗小黑*" -type f

# 检查完整路径
stat "/media/data/Movie/动画电影/罗小黑战记 (2019)/罗小黑战记 (2019) - 2160p.H265.DDP 5.1.HDR.mkv"
```

**期望输出**：
```
  File: /media/data/Movie/动画电影/罗小黑战记 (2019)/罗小黑战记 (2019) - 2160p.H265.DDP 5.1.HDR.mkv
  Size: 12345678900
Access: (0644/-rw-r--r--)  Uid: ( 1000/  user)   Gid: ( 1000/  user)
```

### 4️⃣ 检查文件权限

```bash
# 检查 Nginx 运行用户
ps aux | grep nginx | grep -v grep

# 期望输出示例：
# www-data  1234  ... nginx: worker process

# 检查文件权限（确保 Nginx 用户可读）
namei -l /media/data/Movie/动画电影/罗小黑战记\ \(2019\)/罗小黑战记\ \(2019\)\ -\ 2160p.H265.DDP\ 5.1.HDR.mkv
```

**如果权限不足**：

```bash
# 方案 1: 修改文件所有者（推荐）
sudo chown -R www-data:www-data /media/data/

# 方案 2: 添加读取权限
sudo chmod -R o+rX /media/data/
```

### 5️⃣ 检查鉴权服务是否正常

Nginx 配置使用了 `auth_request /auth`，需要确保后端鉴权服务正常：

```bash
# 检查 go-emby2openlist 是否运行
docker ps | grep go-emby2openlist

# 检查鉴权服务是否监听 8097 端口
sudo netstat -tlnp | grep 8097

# 手动测试鉴权接口
curl -v "http://127.0.0.1:8097/api/auth?api_key=5c762c8479344405ace0c24324b6dc40&target_path=/video/data/test.mkv&remote_ip=127.0.0.1"
```

**期望输出**：
```
< HTTP/1.1 200 OK
...
```

**如果返回 403**：
- api_key 无效或已过期
- 鉴权服务配置错误

**如果无法连接**：
- go-emby2openlist 未启动
- 端口未监听

### 6️⃣ 查看 Nginx 错误日志

```bash
# 查看总错误日志
sudo tail -50 /var/log/nginx/error.log

# 查看 video_data 专用错误日志
sudo tail -50 /var/log/nginx/video_data_error.log

# 实时监控日志
sudo tail -f /var/log/nginx/video_data_error.log &
# 然后访问视频 URL，观察日志输出
```

**常见错误信息**：

| 错误信息 | 原因 | 解决方案 |
|---------|------|---------|
| `open() "/media/data/..." failed (2: No such file or directory)` | 文件不存在 | 检查文件路径 |
| `open() "/media/data/..." failed (13: Permission denied)` | 权限不足 | 修改文件权限 |
| `auth request unexpected status: 403` | 鉴权失败 | 检查 api_key 和鉴权服务 |
| `upstream prematurely closed connection` | 后端服务异常 | 检查 go-emby2openlist 日志 |

### 7️⃣ 查看 Nginx 访问日志

```bash
# 查看访问日志
sudo tail -50 /var/log/nginx/video_data_access.log | grep -E "404|403"

# 示例输出：
# 183.179.251.164 - - [06/Dec/2025:14:30:45 +0800] "GET /video/data/Movie/... HTTP/1.1" 404 169 "-" "Mozilla/5.0..."
```

### 8️⃣ 测试不带鉴权的访问

临时禁用鉴权，测试是否是鉴权问题：

```bash
# 编辑 Nginx 配置
sudo nano /etc/nginx/sites-available/video-gateway.conf

# 注释掉 auth_request 行（第 88 行）
# location /video/data {
#     alias /media/data/;
#     # auth_request /auth;  ← 注释掉这行
#     ...
# }

# 重新加载配置
sudo nginx -t && sudo nginx -s reload

# 测试访问（不带 api_key）
curl -I "http://127.0.0.1:8081/video/data/Movie/动画电影/罗小黑战记%20(2019)/罗小黑战记%20(2019)%20-%202160p.H265.DDP%205.1.HDR.mkv"
```

**期望输出**：
```
HTTP/1.1 200 OK
Accept-Ranges: bytes
Content-Length: 12345678900
Content-Type: video/x-matroska
```

如果返回 200，说明问题在鉴权服务；如果仍然 404，说明是文件路径或权限问题。

### 9️⃣ 检查 URL 编码

中文路径需要正确的 URL 编码：

```bash
# 正确的编码测试
curl -I "http://127.0.0.1:8081/video/data/Movie/%E5%8A%A8%E7%94%BB%E7%94%B5%E5%BD%B1/%E7%BD%97%E5%B0%8F%E9%BB%91%E6%88%98%E8%AE%B0%20%282019%29/%E7%BD%97%E5%B0%8F%E9%BB%91%E6%88%98%E8%AE%B0%20%282019%29%20-%202160p.H265.DDP%205.1.HDR.mkv?api_key=5c762c8479344405ace0c24324b6dc40"
```

### 🔟 检查 go-emby2openlist 日志

```bash
# 查看 Docker 容器日志
docker logs -f --tail=100 go-emby2openlist

# 如果不是 Docker 部署
tail -f /path/to/go-emby2openlist/logs/*.log
```

关注是否有鉴权相关的错误日志。

---

## 快速诊断命令

一键执行所有检查：

```bash
#!/bin/bash
echo "=== 1. Nginx 配置检查 ==="
sudo nginx -t

echo -e "\n=== 2. 端口监听检查 ==="
sudo netstat -tlnp | grep -E "8081|8097"

echo -e "\n=== 3. 文件路径检查 ==="
ls -lh /media/data/Movie/ | head -10

echo -e "\n=== 4. Nginx 错误日志（最近10行） ==="
sudo tail -10 /var/log/nginx/video_data_error.log

echo -e "\n=== 5. Nginx 访问日志（最近10行） ==="
sudo tail -10 /var/log/nginx/video_data_access.log

echo -e "\n=== 6. 鉴权服务检查 ==="
curl -s -o /dev/null -w "Status: %{http_code}\n" "http://127.0.0.1:8097/api/auth?api_key=5c762c8479344405ace0c24324b6dc40&target_path=/test&remote_ip=127.0.0.1"

echo -e "\n=== 7. Docker 容器状态 ==="
docker ps | grep go-emby2openlist
```

保存为 `diagnose.sh`，执行：

```bash
chmod +x diagnose.sh
sudo ./diagnose.sh
```

---

## 常见问题解决方案

### 问题 1: 配置文件未生效

**症状**：修改配置后仍然 404

**解决**：
```bash
sudo ln -s /etc/nginx/sites-available/video-gateway.conf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl restart nginx
```

### 问题 2: 文件路径不存在

**症状**：日志显示 `No such file or directory`

**解决**：
1. 检查文件是否真实存在
2. 检查路径大小写是否正确
3. 检查是否有多余的空格或特殊字符

### 问题 3: 权限不足

**症状**：日志显示 `Permission denied`

**解决**：
```bash
# 临时方案（不推荐生产环境）
sudo chmod -R 755 /media/data/

# 推荐方案
sudo chown -R www-data:www-data /media/data/
sudo chmod -R 644 /media/data/**/*.mkv
sudo chmod -R 755 /media/data/**/
```

### 问题 4: 鉴权服务异常

**症状**：日志显示 `auth request unexpected status: 403`

**解决**：
1. 检查 go-emby2openlist 是否运行
2. 检查 api_key 是否有效
3. 查看 go-emby2openlist 日志

```bash
docker logs go-emby2openlist | grep -E "ERROR|WARN|鉴权"
```

### 问题 5: URL 中有特殊字符

**症状**：浏览器访问正常，curl 访问 404

**解决**：
使用正确的 URL 编码，空格 → `%20`，中文 → UTF-8 编码

---

## 验证修复

修复后，执行以下测试：

```bash
# 1. 健康检查
curl http://183.179.251.164:80/gtm-health
# 期望: OK

# 2. 鉴权测试
curl -I "http://183.179.251.164:8081/video/data/Movie/%E5%8A%A8%E7%94%BB%E7%94%B5%E5%BD%B1/%E7%BD%97%E5%B0%8F%E9%BB%91%E6%88%98%E8%AE%B0%20%282019%29/%E7%BD%97%E5%B0%8F%E9%BB%91%E6%88%98%E8%AE%B0%20%282019%29%20-%202160p.H265.DDP%205.1.HDR.mkv?api_key=5c762c8479344405ace0c24324b6dc40"
# 期望: HTTP/1.1 200 OK

# 3. Range 请求测试（模拟视频拖拽）
curl -I -H "Range: bytes=0-1023" "http://183.179.251.164:8081/video/data/Movie/%E5%8A%A8%E7%94%BB%E7%94%B5%E5%BD%B1/%E7%BD%97%E5%B0%8F%E9%BB%91%E6%88%98%E8%AE%B0%20%282019%29/%E7%BD%97%E5%B0%8F%E9%BB%91%E6%88%98%E8%AE%B0%20%282019%29%20-%202160p.H265.DDP%205.1.HDR.mkv?api_key=5c762c8479344405ace0c24324b6dc40"
# 期望: HTTP/1.1 206 Partial Content
```

---

**文档版本**: v1.0
**最后更新**: 2025-12-06
