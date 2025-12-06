# Nginx 视频服务配置文件说明

## 📁 配置文件清单

### 🎯 正式配置（推荐使用）

#### `video-gateway-URL-DECODE-FIX.conf` ✅ **当前使用**
**方案1: 应用层签名临时 URL（完整版）**

**特性**：
- ✅ HMAC-SHA256 签名防伪造  
- ✅ 5分钟过期时间防分享
- ✅ UID用户追踪（支持封禁）
- ✅ 完整的访问和下载日志
- ✅ URL自动解码（支持中文文件名）
- ✅ CORS跨域支持
- ✅ auth_request token验证

---

### 📚 备用配置（参考）

#### `video-gateway-SIMPLE.conf`
**简化方案：仅做文件服务**
- 纯文件服务（无鉴权）
- 适合内网测试环境

⚠️ **注意**: 生产环境不推荐使用

---

## 🚀 快速部署

```bash
cd /usr/local/go-emby2openlist
cp nginx/video-gateway-URL-DECODE-FIX.conf /etc/nginx/sites-available/video-gateway.conf
ln -sf /etc/nginx/sites-available/video-gateway.conf /etc/nginx/sites-enabled/
nginx -t && nginx -s reload
```

---

## 📝 已删除的历史版本

以下12个配置文件已删除：
- video-custom-port-46621.conf
- video-custom-with-auth.conf
- video-custom.conf
- video-gateway-CORRECT.conf
- video-gateway-OPTIMIZED.conf
- video-gateway-SIGNED-URL-FIXED.conf
- video-gateway-SIGNED-URL.conf
- video-gateway-TEST-NO-AUTH.conf
- video-with-auth.conf
- video-with-backend-auth.conf
- video-with-emby-auth.conf
- video.conf
