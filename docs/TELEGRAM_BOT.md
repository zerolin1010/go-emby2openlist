# Telegram Bot 使用指南

## 📱 功能介绍

Telegram Bot 可以让你通过 Telegram 聊天来管理 CDN 节点，无需登录服务器或修改配置文件。

### 主要功能

- ✅ 查看所有节点列表
- ✅ 查看节点健康状态（实时）
- ✅ 添加新节点
- ✅ 删除节点
- ✅ 启用/禁用节点
- 🔐 权限控制（仅管理员可操作）

---

## 🚀 快速开始

### 第一步：创建 Telegram Bot

1. 在 Telegram 中找到 [@BotFather](https://t.me/BotFather)
2. 发送 `/newbot` 命令
3. 按照提示设置 Bot 名称和用户名
4. 复制 BotFather 返回的 **Bot Token**（格式: `123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11`）

### 第二步：获取你的用户 ID

1. 在 Telegram 中找到 [@userinfobot](https://t.me/userinfobot)
2. 发送任意消息
3. Bot 会返回你的用户 ID（一串数字，例如: `123456789`）

### 第三步：配置 config.yml

编辑 `config.yml` 文件，添加 Telegram 配置：

```yaml
telegram:
  # 启用 Telegram Bot
  enable: true

  # Bot Token (从 @BotFather 获取)
  bot-token: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"

  # 管理员用户 ID 列表（可以添加多个管理员）
  admin-users:
    - 123456789
    - 987654321

  # 使用轮询模式（默认）
  webhook-mode: false
```

### 第四步：启动服务

```bash
# 重启 Go 服务以应用新配置
docker-compose restart

# 或者如果是直接运行
./go-emby2openlist
```

启动后，日志中会显示：

```
[INFO] [Telegram] Bot 已连接: @your_bot_username
[INFO] [Telegram] 开始监听消息...
```

### 第五步：开始使用

1. 在 Telegram 中搜索你的 Bot 用户名（例如: `@your_bot_username`）
2. 点击 `Start` 或发送 `/start` 命令
3. Bot 会回复帮助信息

---

## 📚 命令列表

### `/start` 或 `/help`
显示帮助信息和所有可用命令

**示例：**
```
/help
```

---

### `/list`
列出所有已配置的节点

**示例：**
```
/list
```

**响应示例：**
```
📋 节点列表：

1. node-1
   • Host: http://1.2.3.4:80
   • 权重: 100
   • 状态: ✅ 启用

2. node-2
   • Host: http://5.6.7.8:80
   • 权重: 80
   • 状态: ⛔ 禁用
```

---

### `/status`
查看所有节点的实时健康状态

**示例：**
```
/status
```

**响应示例：**
```
🏥 节点健康状态：

1. node-1
   • Host: http://1.2.3.4:80
   • 权重: 100
   • 状态: ✅ 健康

2. node-2
   • Host: http://5.6.7.8:80
   • 权重: 80
   • 状态: ❌ 不健康

3. node-3
   • Host: http://9.10.11.12:80
   • 权重: 60
   • 状态: ⛔ 已禁用

📊 统计：
• 总节点数: 3
• 健康节点: 1
• 更新时间: 2025-01-15 10:30:45
```

---

### `/add`
添加新节点

**语法：**
```
/add <节点名称> <节点地址> <权重>
```

**参数说明：**
- `节点名称`: 节点的唯一标识（不能重复）
- `节点地址`: 完整的 HTTP/HTTPS 地址
- `权重`: 1-100 之间的整数，权重越高被选中概率越大

**示例：**
```
/add node-4 http://13.14.15.16:80 90
```

**响应：**
```
✅ 节点 node-4 添加成功
正在进行健康检查...
```

**注意事项：**
- 节点必须支持健康检查接口 `GET /gtm-health`（Host: gtm-health）
- 添加后会自动开始健康检查
- 新节点默认为启用状态

---

### `/del` 或 `/delete`
删除节点

**语法：**
```
/del <节点名称>
```

**示例：**
```
/del node-4
```

**响应：**
```
✅ 节点 node-4 已删除
```

**警告：** 删除操作不可恢复！

---

### `/enable`
启用已禁用的节点

**语法：**
```
/enable <节点名称>
```

**示例：**
```
/enable node-2
```

**响应：**
```
✅ 节点 node-2 已启用
```

启用后节点会重新参与负载均衡和健康检查。

---

### `/disable`
禁用节点（暂停使用但不删除）

**语法：**
```
/disable <节点名称>
```

**示例：**
```
/disable node-2
```

**响应：**
```
✅ 节点 node-2 已禁用
```

禁用后节点会停止参与负载均衡，但配置保留。

---

## 🔐 权限说明

### 管理员权限

只有在 `config.yml` 中配置的 `admin-users` 列表中的用户才能使用 Bot。

非管理员用户会收到：
```
❌ 无权限访问
```

### 添加多个管理员

在 `config.yml` 中添加多个用户 ID：

```yaml
telegram:
  admin-users:
    - 123456789  # 管理员 1
    - 987654321  # 管理员 2
    - 555666777  # 管理员 3
```

---

## ⚙️ 高级配置

### Webhook 模式（可选）

如果你的服务器有公网 IP 和域名，可以使用 Webhook 模式代替轮询模式，性能更好。

```yaml
telegram:
  enable: true
  bot-token: "your-token"
  admin-users:
    - 123456789

  # 启用 Webhook 模式
  webhook-mode: true
  webhook-url: "https://your-domain.com/telegram-webhook"
```

**注意：** Webhook 模式需要 HTTPS。

---

## 🧪 测试步骤

### 1. 测试 Bot 连接

启动服务后，查看日志：

```bash
docker logs -f go-emby2openlist
```

应该看到：
```
[INFO] [Telegram] Bot 已连接: @your_bot_username
[INFO] [Telegram] 开始监听消息...
```

### 2. 测试权限验证

使用非管理员账号发送 `/start`，应该收到：
```
❌ 无权限访问
```

### 3. 测试基本命令

以管理员身份发送：

```
/help
/list
/status
```

### 4. 测试节点管理

```bash
# 添加测试节点
/add test-node http://127.0.0.1:80 50

# 查看节点
/list

# 查看状态
/status

# 禁用节点
/disable test-node

# 启用节点
/enable test-node

# 删除节点
/del test-node
```

---

## 🆘 故障排查

### 问题 1: Bot 无响应

**症状：** 发送命令后 Bot 没有任何回复

**排查步骤：**

1. 检查服务日志
```bash
docker logs -f go-emby2openlist | grep Telegram
```

2. 检查 Bot Token 是否正确
```yaml
telegram:
  bot-token: "正确的token"  # 确保没有多余空格
```

3. 检查网络连接
```bash
curl -X GET "https://api.telegram.org/bot<YOUR-BOT-TOKEN>/getMe"
```

---

### 问题 2: 提示无权限

**症状：** 发送命令后收到 `❌ 无权限访问`

**原因：** 你的用户 ID 不在 admin-users 列表中

**解决方案：**

1. 获取你的用户 ID（通过 @userinfobot）
2. 添加到 config.yml:
```yaml
telegram:
  admin-users:
    - 你的用户ID
```
3. 重启服务

---

### 问题 3: 添加节点后显示不健康

**原因：** 节点可能没有正确配置健康检查接口

**检查方法：**

```bash
curl -v -H "Host: gtm-health" http://<节点IP>/gtm-health
```

应该返回：
```
HTTP/1.1 200 OK
OK
```

参考 `nginx/video.conf` 配置健康检查接口。

---

### 问题 4: Bot Token 无效

**症状：** 启动时报错 `创建 Telegram Bot 失败`

**解决方案：**

1. 检查 Token 格式是否正确（应该包含冒号）
2. 向 @BotFather 发送 `/mybots` 检查 Bot 是否存在
3. 如果 Token 泄露，使用 @BotFather 的 `/revoke` 重新生成

---

## 💡 使用技巧

### 1. 快速添加多个节点

准备好节点信息后，可以连续发送多个添加命令：

```
/add node-1 http://1.2.3.4:80 100
/add node-2 http://5.6.7.8:80 80
/add node-3 http://9.10.11.12:80 60
```

### 2. 定期检查节点状态

建议每天使用 `/status` 检查节点健康状态。

### 3. 节点维护流程

维护节点时的推荐操作顺序：

```bash
# 1. 禁用节点（停止新请求）
/disable node-1

# 2. 等待现有连接结束（约1-2分钟）

# 3. 进行服务器维护

# 4. 维护完成后启用节点
/enable node-1

# 5. 检查节点是否恢复健康
/status
```

---

## 📝 安全建议

1. **保护 Bot Token**
   - 不要将 Token 提交到公开的 Git 仓库
   - 定期更换 Token

2. **限制管理员数量**
   - 只添加可信任的用户到 admin-users

3. **使用 Webhook 模式**
   - 如果有条件，使用 Webhook 模式更安全

4. **定期审计**
   - 检查 admin-users 列表
   - 查看 Bot 操作日志

---

## 🔗 相关链接

- [Telegram Bot API 文档](https://core.telegram.org/bots/api)
- [BotFather](https://t.me/BotFather) - 创建和管理 Bot
- [userinfobot](https://t.me/userinfobot) - 获取用户 ID
- [项目主文档](../README.md)
- [Nginx 配置说明](../nginx/README.md)

---

**更新时间：** 2025-01-15
**版本：** v2.3.2+telegram
