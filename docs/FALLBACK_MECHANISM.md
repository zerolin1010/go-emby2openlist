# 回退机制说明文档

## 📋 概述

go-emby2openlist 具有完善的回退机制，确保即使所有 Nginx 节点不可用时，用户仍然可以正常播放视频。

---

## 🔄 回退机制工作流程

### 正常流程（有健康节点）

```
1. 客户端请求视频
   → http://go-emby2openlist:8095/emby/Items/12345/Download

2. 解析请求，获取 Emby 媒体路径
   → /media/data/movies/test.mp4

3. 检查是否为本地媒体
   ✅ 是本地媒体 → 直接回源到 Emby（ProxyOrigin）
   ❌ 非本地媒体 → 继续

4. 映射到 Nginx 路径
   → /video/data/movies/test.mp4

5. 选择健康节点
   ✅ 有健康节点 → 返回 302 重定向
   ❌ 无健康节点 → 触发回退机制

6. 返回 302 重定向
   → Location: http://nginx-ip/video/data/movies/test.mp4?api_key=xxx

7. 客户端直连 Nginx 节点
```

### 回退流程（无健康节点）

```
1. 选择节点失败
   SelectNode() 返回 nil（没有健康节点）

2. 触发 checkErr() 函数
   checkErr(c, fmt.Errorf("没有可用的健康节点"))

3. 根据配置策略处理

   策略 A: origin（默认，推荐）
   ├─ 记录错误日志：代理接口失败: 没有可用的健康节点, 回源处理
   ├─ 调用 ProxyOrigin(c)
   ├─ 请求直接代理到 Emby 服务器
   └─ 用户可以正常播放（消耗 Emby 服务器带宽）

   策略 B: reject（不推荐）
   ├─ 记录错误日志：代理接口失败: 没有可用的健康节点
   ├─ 返回 500 错误
   └─ 用户无法播放
```

---

## ⚙️ 配置说明

### 配置文件：config.yml

```yaml
emby:
  host: http://localhost:8096
  admin-api-key: "your-api-key"

  # ⭐ 代理错误策略（回退机制）
  proxy-error-strategy: origin  # 默认值

  # 可选值:
  # - origin: 回源到 Emby（推荐）- 保证服务可用性
  # - reject: 拒绝请求 - 返回 500 错误
```

### 策略对比

| 策略 | 行为 | 优点 | 缺点 | 适用场景 |
|-----|------|------|------|---------|
| **origin** | 回源到 Emby | 用户体验好，始终可播放 | Emby 带宽消耗增加 | 生产环境（推荐） |
| **reject** | 返回 500 错误 | 节省 Emby 带宽 | 用户无法播放 | 测试环境 |

---

## 🔍 回退触发场景

### 1. 所有节点不健康

```
场景：所有 Nginx 节点都无法访问

原因：
- 网络故障
- Nginx 服务停止
- 健康检查接口返回非 200

触发条件：
- nodeSelector.SelectNode() 返回 nil

行为：
- 策略 origin → 回源到 Emby
- 策略 reject → 返回 500 错误
```

### 2. 路径映射失败

```
场景：Emby 路径无法映射到 Nginx 路径

原因：
- config.yml 中未配置该路径映射

示例：
Emby 路径：/media/unknown/test.mp4
配置映射：
  - /media/data:/video/data
  - /media/data1:/video/data1
结果：无法找到匹配的映射

触发条件：
- config.C.Path.MapEmby2Nginx() 返回 false

行为：
- 记录错误：无法映射 Emby 路径到 Nginx
- 调用 checkErr() → 根据策略处理
```

### 3. 获取 Emby 文件路径失败

```
场景：无法从 Emby API 获取文件路径

原因：
- Emby API 返回错误
- Item 不存在
- 网络问题

触发条件：
- getEmbyFileLocalPath() 返回错误

行为：
- 调用 checkErr() → 根据策略处理
```

---

## 📊 监控和日志

### 正常日志（有健康节点）

```
2025-12-06 13:45:23 [INFO] 解析到的 itemInfo: {...}
2025-12-06 13:45:23 [INFO] Emby 媒体路径: /media/data/movies/test.mp4
2025-12-06 13:45:23 [INFO] Nginx 路径: /video/data/movies/test.mp4
2025-12-06 13:45:23 [INFO] 选择节点: node-1 (http://192.168.1.100:80)
2025-12-06 13:45:23 [SUCCESS] 重定向到: http://192.168.1.100/video/data/movies/test.mp4?api_key=xxx
```

### 回退日志（无健康节点）

```
2025-12-06 13:45:23 [INFO] 解析到的 itemInfo: {...}
2025-12-06 13:45:23 [INFO] Emby 媒体路径: /media/data/movies/test.mp4
2025-12-06 13:45:23 [INFO] Nginx 路径: /video/data/movies/test.mp4
2025-12-06 13:45:23 [ERROR] 代理接口失败: 没有可用的健康节点, 回源处理
2025-12-06 13:45:23 [INFO] 代理请求到: http://localhost:8096/emby/Items/12345/Download
```

### 监控指标

可以通过日志统计回退频率：

```bash
# 统计回退次数
grep "没有可用的健康节点, 回源处理" logs/*.log | wc -l

# 查看最近的回退事件
grep "没有可用的健康节点" logs/*.log | tail -20

# 监控实时日志
tail -f logs/*.log | grep -E "回源处理|没有可用的健康节点"
```

---

## 🎯 最佳实践

### 1. 生产环境配置（推荐）

```yaml
emby:
  host: http://localhost:8096
  admin-api-key: "your-api-key"
  proxy-error-strategy: origin  # ⭐ 使用 origin 策略

nodes:
  health-check:
    interval: 30          # 30秒检查一次
    timeout: 5            # 5秒超时
    fail-threshold: 3     # 连续3次失败才标记为不健康
    success-threshold: 2  # 连续2次成功恢复健康
  list:
    - name: "node-1"
      host: "http://192.168.1.100:80"
      weight: 100
      enabled: true
    - name: "node-2"      # ⭐ 配置多个节点提高可用性
      host: "http://192.168.1.101:80"
      weight: 80
      enabled: true
```

**优势**：
- 多节点冗余
- 自动故障转移
- 节点故障时回源保证服务可用

### 2. 测试环境配置

```yaml
emby:
  proxy-error-strategy: reject  # 测试时使用 reject

nodes:
  health-check:
    interval: 10   # 更频繁的检查
    timeout: 2
    fail-threshold: 2
    success-threshold: 1
```

**用途**：
- 快速发现节点问题
- 避免回源掩盖节点故障
- 便于调试

### 3. 高可用配置

```yaml
nodes:
  health-check:
    interval: 15             # 更频繁的健康检查
    timeout: 3
    fail-threshold: 2        # 更快的故障检测
    success-threshold: 1     # 更快的恢复
  list:
    # 多个节点，不同地理位置
    - name: "node-asia"
      host: "http://asia-node:80"
      weight: 100
      enabled: true
    - name: "node-europe"
      host: "http://europe-node:80"
      weight: 80
      enabled: true
    - name: "node-us"
      host: "http://us-node:80"
      weight: 60
      enabled: true
```

---

## 🔧 故障排查

### 问题 1: 频繁回退

**症状**：
```
[ERROR] 代理接口失败: 没有可用的健康节点, 回源处理
```

**排查步骤**：

1. **检查节点健康**：
```bash
docker logs go-emby2openlist 2>&1 | grep "健康检查"
```

2. **手动测试节点**：
```bash
curl -H "Host: gtm-health" http://node-ip/gtm-health
```

3. **检查网络连通性**：
```bash
ping node-ip
telnet node-ip 80
```

4. **查看 Nginx 日志**：
```bash
tail -f /var/log/nginx/error.log
```

### 问题 2: 回退后性能下降

**原因**：回源到 Emby，消耗 Emby 服务器带宽

**解决方案**：

1. **修复 Nginx 节点**
2. **增加节点数量**
3. **优化健康检查参数**：
```yaml
nodes:
  health-check:
    interval: 20          # 减少检查频率
    fail-threshold: 5     # 增加失败容忍度
```

### 问题 3: 本地媒体总是回源

**症状**：
```
[INFO] 本地媒体: /local/path/test.mp4, 回源处理
```

**说明**：这是**正常行为**，不是回退

**配置**：
```yaml
emby:
  local-media-root: "/local"  # 本地媒体根目录
```

本地媒体会直接回源，不经过 302 重定向。

---

## 📈 性能影响

### 正常模式（302 重定向）

- **Emby 带宽消耗**：仅控制请求（< 1KB）
- **Nginx 带宽消耗**：视频文件（GB 级）
- **延迟**：302 重定向 < 5ms

### 回退模式（回源）

- **Emby 带宽消耗**：视频文件（GB 级）⚠️
- **延迟**：取决于 Emby 服务器性能

### 对比

| 指标 | 正常模式 | 回退模式 |
|-----|---------|---------|
| Emby 带宽 | < 1KB | 整个视频文件 |
| Nginx 带宽 | 整个视频文件 | 0 |
| 播放延迟 | 低 | 取决于 Emby |
| 服务器负载 | 低 | 高 |

---

## ✅ 验证回退机制

### 测试步骤

1. **启动服务**：
```bash
docker-compose up -d
```

2. **正常播放测试**：
- 访问 Emby，播放视频
- 观察日志，应该看到 302 重定向

3. **触发回退测试**：
```bash
# 停止所有 Nginx 节点
systemctl stop nginx  # 在所有节点上执行

# 等待健康检查标记节点为不健康（约 90 秒）
# fail-threshold: 3 × interval: 30s = 90s
```

4. **验证回退**：
- 再次播放视频
- 应该能正常播放（通过 Emby）
- 日志显示：`代理接口失败: 没有可用的健康节点, 回源处理`

5. **恢复测试**：
```bash
# 启动 Nginx 节点
systemctl start nginx

# 等待健康检查恢复（约 60 秒）
# success-threshold: 2 × interval: 30s = 60s
```

6. **验证恢复**：
- 播放视频
- 应该恢复 302 重定向

---

## 🎉 总结

✅ **go-emby2openlist 具有完善的回退机制**

- 默认策略：`origin`（回源到 Emby）
- 保证服务可用性：节点故障时自动回退
- 灵活配置：可根据需求选择策略
- 自动恢复：节点恢复后自动切换回 302 模式

**推荐配置**：
- 生产环境：`proxy-error-strategy: origin`
- 配置多个 Nginx 节点提高可用性
- 合理设置健康检查参数平衡响应速度和稳定性

---

**文档版本**: v2.4.2
**最后更新**: 2025-12-06
**作者**: Claude AI Assistant
