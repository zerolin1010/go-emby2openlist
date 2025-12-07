# 404 错误排查指南

## 问题描述
播放某些库的视频时出现 404 错误，例如：
```
http://8.138.199.183:46621/internal/data1/TVshow/...
状态码: 404 Not Found
```

---

## 🔍 排查步骤

### 步骤 1：确认文件在服务器上存在

在服务器（183.179.251.164）上执行：

```bash
# 检查文件是否存在
ls -lh "/mnt/google1/TVshow/国产剧/毒舌家庭 (2025) {tmdbid=273135}/Season 01/毒舌家庭 S01E01 2160p.WEB-DL.H265.AAC-HHWEB.mp4"

# 如果找不到，检查实际的挂载点
df -h | grep google
mount | grep google
```

**预期结果**：
- 文件应该存在于 `/mnt/google1` 目录下
- 如果不存在，说明磁盘未挂载或路径错误

---

### 步骤 2：检查 Nginx 配置

```bash
# 查看 Nginx 配置中 data1 的路径映射
cat /etc/nginx/sites-available/video-gateway.conf | grep -A 2 "data1"
```

**预期结果**：
```nginx
if ($media_type = 'data1') {
    set $root_path '/mnt/google1';
}
```

**如果不正确**：
```bash
# 更新配置
cd /usr/local/go-emby2openlist
cp nginx/video-gateway-URL-DECODE-FIX.conf /etc/nginx/sites-available/video-gateway.conf

# 测试并重新加载
nginx -t && nginx -s reload
```

---

### 步骤 3：检查 Docker 容器挂载

Docker 容器**不需要**挂载媒体目录，因为：
- Nginx 直接访问宿主机的 `/mnt/google1`
- Go 应用只负责鉴权，不直接访问文件

但如果您的架构不同，可以检查：

```bash
# 检查容器挂载
docker inspect go-emby2openlist | grep -A 10 Mounts
```

---

### 步骤 4：检查 config.yml 路径映射

```bash
# 查看配置文件
cat /usr/local/go-emby2openlist/config/config.yml | grep -A 10 "path:"
```

**关键配置**：
```yaml
path:
  emby2nginx:
    - /media/data:/video/data       # Emby 路径 -> Nginx 路径
    - /media/data1:/video/data1     # 必须包含 data1 映射
    - /media/data2:/video/data2
    # ... 其他映射
```

**重要说明**：
- **左边**（/media/data1）是 **Emby 容器内的路径**
- **右边**（/video/data1）是 Nginx 中间路径（最终映射到 `/internal/data1`）

**如果配置错误**：
```bash
# 编辑配置文件
vi /usr/local/go-emby2openlist/config/config.yml

# 重启容器
docker restart go-emby2openlist
```

---

### 步骤 5：验证 302 重定向

```bash
# 测试 302 重定向是否正确
curl -I "http://localhost:8095/videos/123456/stream.mkv?api_key=YOUR_API_KEY" 2>&1 | grep Location
```

**预期结果**：
```
Location: http://8.138.199.183:46621/internal/data1/TVshow/...?token=xxx&expires=xxx&uid=xxx
```

**如果重定向到错误的路径**：
- 检查 Emby 中该文件的实际路径
- 确认 config.yml 中的路径映射正确

---

### 步骤 6：检查 Nginx 错误日志

```bash
# 查看 Nginx 错误日志
tail -50 /var/log/nginx/video_internal_error.log
```

**常见错误**：

#### 错误 1：文件不存在
```
open() "/mnt/google1/TVshow/..." failed (2: No such file or directory)
```

**解决方法**：
- 检查磁盘挂载
- 检查路径映射是否正确

#### 错误 2：权限被拒绝
```
open() "/mnt/google1/TVshow/..." failed (13: Permission denied)
```

**解决方法**：
```bash
# 检查文件权限
ls -la /mnt/google1/TVshow/

# 给 Nginx 读取权限
chmod -R 755 /mnt/google1

# 或者将 Nginx 添加到对应的用户组
usermod -aG <group> nginx
```

#### 错误 3：URL 编码问题
```
open() "/mnt/google1/TVshow/%e5%9b%bd%e4%ba%a7%e5%89%a7/..." failed
```

这**不是错误**，Nginx 会自动解码 URL。如果出现 404，说明解码后的路径仍然不存在。

---

## 🎯 常见问题解决

### 问题 1：只有 data1/data2/... 出现 404，data 正常

**原因**：config.yml 中缺少对应的路径映射

**解决方法**：
```bash
# 编辑配置文件
vi /usr/local/go-emby2openlist/config/config.yml

# 添加缺失的映射
path:
  emby2nginx:
    - /media/data:/video/data
    - /media/data1:/video/data1    # 添加这一行
    - /media/data2:/video/data2    # 添加这一行

# 重启容器
docker restart go-emby2openlist
```

---

### 问题 2：Emby 中看到文件，但播放 404

**原因**：Emby 路径和实际服务器路径不匹配

**诊断步骤**：

1. **在 Emby 中查看文件路径**：
   - 打开 Emby Web 界面
   - 进入 "控制台" → "媒体库" → 点击具体视频
   - 查看 "路径" 字段，例如：`/media/data1/TVshow/...`

2. **在服务器上查找文件**：
   ```bash
   # 根据 Emby 显示的路径查找
   find /mnt -name "毒舌家庭*" -type f 2>/dev/null
   ```

3. **确认路径映射**：
   - Emby 中：`/media/data1/TVshow/...`
   - 服务器上：`/mnt/google1/TVshow/...`
   - 映射配置：`/media/data1:/video/data1`

4. **验证映射是否正确**：
   ```bash
   # 在 Go 应用日志中查看映射结果
   docker logs go-emby2openlist 2>&1 | grep "Nginx 路径"
   ```

---

### 问题 3：所有库都 404

**原因**：Nginx 配置未生效或路径完全错误

**解决方法**：

```bash
# 1. 检查 Nginx 配置是否正确加载
nginx -t

# 2. 重新部署 Nginx 配置
cd /usr/local/go-emby2openlist
cp nginx/video-gateway-URL-DECODE-FIX.conf /etc/nginx/sites-available/video-gateway.conf

# 3. 重新加载 Nginx
nginx -s reload

# 4. 检查 Nginx 是否正常运行
systemctl status nginx
curl -I http://localhost:7777
```

---

### 问题 4：中文文件名 404

**原因**：URL 编码问题（v2.5.0 已修复）

**确认修复**：
```bash
# 检查 Nginx 配置是否使用 root + rewrite
cat /etc/nginx/sites-available/video-gateway.conf | grep -A 5 "rewrite.*internal"
```

**应该看到**：
```nginx
# 使用 root 指令（会自动解码 URL）
rewrite ^/internal/data1(.*)$ $1 break;
root $root_path;
```

**如果仍然使用 alias**：
```nginx
# ❌ 错误（不会解码）
alias $root_path/$file_path;
```

需要更新到最新配置。

---

## 📊 完整诊断脚本

将以下内容保存为 `diagnose_404.sh` 并执行：

```bash
#!/bin/bash

echo "====== 404 错误诊断脚本 ======"
echo ""

# 1. 检查挂载点
echo "1. 检查磁盘挂载:"
df -h | grep -E "google|mnt"
echo ""

# 2. 检查 Nginx 配置
echo "2. 检查 Nginx data1 配置:"
grep -A 2 "data1" /etc/nginx/sites-available/video-gateway.conf | head -10
echo ""

# 3. 检查 config.yml
echo "3. 检查路径映射配置:"
cat /usr/local/go-emby2openlist/config/config.yml | grep -A 15 "path:"
echo ""

# 4. 检查 Nginx 错误日志（最近10行）
echo "4. 最近的 Nginx 错误日志:"
tail -10 /var/log/nginx/video_internal_error.log
echo ""

# 5. 检查文件权限
echo "5. 检查媒体目录权限:"
ls -ld /mnt/google*
echo ""

# 6. 测试文件是否存在
echo "6. 测试示例文件（请替换为实际路径）:"
# ls -lh "/mnt/google1/TVshow/国产剧/毒舌家庭*"
echo "请手动执行: ls -lh \"/mnt/google1/TVshow/...\""
echo ""

echo "====== 诊断完成 ======"
```

**执行方式**：
```bash
chmod +x diagnose_404.sh
./diagnose_404.sh
```

---

## 🚀 快速修复模板

### 修复 1：添加 data1 路径映射

```bash
# 1. 编辑配置
vi /usr/local/go-emby2openlist/config/config.yml

# 2. 在 path.emby2nginx 下添加:
- /media/data1:/video/data1

# 3. 重启容器
docker restart go-emby2openlist

# 4. 等待5秒
sleep 5

# 5. 测试播放
```

---

### 修复 2：更新 Nginx 配置

```bash
cd /usr/local/go-emby2openlist
git pull
cp nginx/video-gateway-URL-DECODE-FIX.conf /etc/nginx/sites-available/video-gateway.conf
nginx -t && nginx -s reload
```

---

## 📞 仍然无法解决？

提供以下信息：

1. **Nginx 错误日志**：
   ```bash
   tail -50 /var/log/nginx/video_internal_error.log
   ```

2. **Go 应用日志**：
   ```bash
   docker logs --tail=100 go-emby2openlist | grep "Nginx 路径"
   ```

3. **实际文件路径**：
   ```bash
   find /mnt -name "*毒舌家庭*" -type f
   ```

4. **Emby 中的路径**：
   - 在 Emby 控制台中查看该视频的完整路径

5. **config.yml 配置**：
   ```bash
   cat /usr/local/go-emby2openlist/config/config.yml | grep -A 20 "path:"
   ```
