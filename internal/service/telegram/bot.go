package telegram

import (
	"fmt"
	"strings"
	"time"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/node"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot Telegram 机器人
type Bot struct {
	api           *tgbotapi.BotAPI
	healthChecker *node.HealthChecker
	nodeManager   *NodeManager
}

// NewBot 创建 Telegram Bot
func NewBot(healthChecker *node.HealthChecker) (*Bot, error) {
	if !config.C.Telegram.Enable {
		return nil, fmt.Errorf("Telegram Bot 未启用")
	}

	api, err := tgbotapi.NewBotAPI(config.C.Telegram.BotToken)
	if err != nil {
		return nil, fmt.Errorf("创建 Telegram Bot 失败: %v", err)
	}

	logs.Info("[Telegram] Bot 已连接: @%s", api.Self.UserName)

	bot := &Bot{
		api:           api,
		healthChecker: healthChecker,
		nodeManager:   NewNodeManager(healthChecker),
	}

	return bot, nil
}

// Start 启动机器人
func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	logs.Info("[Telegram] 开始监听消息...")

	for update := range updates {
		if update.Message == nil {
			continue
		}

		// 检查权限
		if !b.isAdmin(update.Message.From.ID) {
			b.reply(update.Message.Chat.ID, "❌ 无权限访问")
			continue
		}

		// 处理命令
		b.handleCommand(update.Message)
	}
}

// isAdmin 检查用户是否是管理员
func (b *Bot) isAdmin(userID int64) bool {
	for _, adminID := range config.C.Telegram.AdminUserID {
		if adminID == userID {
			return true
		}
	}
	return false
}

// handleCommand 处理命令
func (b *Bot) handleCommand(message *tgbotapi.Message) {
	command := message.Command()
	args := strings.Fields(message.CommandArguments())

	switch command {
	case "start", "help":
		b.handleHelp(message.Chat.ID)
	case "list":
		b.handleList(message.Chat.ID)
	case "add":
		b.handleAdd(message.Chat.ID, args)
	case "del", "delete":
		b.handleDelete(message.Chat.ID, args)
	case "batchadd":
		b.handleBatchAdd(message.Chat.ID, args)
	case "batchdel", "batchdelete":
		b.handleBatchDelete(message.Chat.ID, args)
	case "enable":
		b.handleEnable(message.Chat.ID, args)
	case "disable":
		b.handleDisable(message.Chat.ID, args)
	case "status":
		b.handleStatus(message.Chat.ID)
	default:
		b.reply(message.Chat.ID, "❓ 未知命令，请使用 /help 查看帮助")
	}
}

// handleHelp 帮助命令
func (b *Bot) handleHelp(chatID int64) {
	help := `🤖 *CDN 节点管理 Bot*

📋 *可用命令：*

*基础操作：*
• /list - 列出所有节点
• /status - 查看节点健康状态

*单节点操作：*
• /add <host> [weight] - 添加节点（自动命名）
  例如: /add http://1.2.3.4:80
  或: /add http://1.2.3.4:80 100
• /del <name> - 删除节点
• /enable <name> - 启用节点
• /disable <name> - 禁用节点

*批量操作：*
• /batchadd <host1> <host2> ... - 批量添加节点
  例如: /batchadd http://1.2.3.4:80 http://5.6.7.8:80:50
• /batchdel <name1> <name2> ... - 批量删除节点
  例如: /batchdel node1 node2

💡 *提示：*
- 节点会自动命名（格式：node-{IP简写}-{序号}）
- 节点必须支持健康检查接口 (GET /gtm-health)
- 权重范围: 1-100，默认 100
- 可在host后加:weight指定权重
- 权重越高，被选中的概率越大`

	b.replyMarkdown(chatID, help)
}

// handleList 列出所有节点
func (b *Bot) handleList(chatID int64) {
	nodes := b.nodeManager.ListNodes()

	if len(nodes) == 0 {
		b.reply(chatID, "📭 当前没有配置任何节点")
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 *节点列表：*\n\n")

	for i, node := range nodes {
		status := "✅ 启用"
		if !node.Enabled {
			status = "⛔ 禁用"
		}

		sb.WriteString(fmt.Sprintf(
			"%d. *%s*\n   • Host: `%s`\n   • 权重: %d\n   • 状态: %s\n\n",
			i+1, node.Name, node.Host, node.Weight, status,
		))
	}

	b.replyMarkdown(chatID, sb.String())
}

// handleAdd 添加节点（支持自动命名）
func (b *Bot) handleAdd(chatID int64, args []string) {
	if len(args) < 1 {
		b.reply(chatID, "❌ 参数错误\n用法: /add <host> [weight]\n例如: /add http://1.2.3.4:80\n或: /add http://1.2.3.4:80 100")
		return
	}

	host := args[0]
	weight := 100

	// 解析权重（可选）
	if len(args) >= 2 {
		fmt.Sscanf(args[1], "%d", &weight)
	}

	// 验证权重
	if weight < 1 || weight > 100 {
		b.reply(chatID, "❌ 权重必须在 1-100 之间")
		return
	}

	// 添加节点（名称自动生成）
	newNode := config.Node{
		Name:    "", // 空字符串，Manager 会自动生成
		Host:    host,
		Weight:  weight,
		Enabled: true,
	}

	if err := b.nodeManager.AddNode(newNode); err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 添加节点失败: %v", err))
		return
	}

	// 获取生成的节点名称
	nodes := b.nodeManager.ListNodes()
	var addedName string
	for _, node := range nodes {
		if node.Host == host {
			addedName = node.Name
			break
		}
	}

	b.reply(chatID, fmt.Sprintf("✅ 节点添加成功\n• 名称: %s\n• 主机: %s\n• 权重: %d\n正在进行健康检查...", addedName, host, weight))
}

// handleDelete 删除节点
func (b *Bot) handleDelete(chatID int64, args []string) {
	if len(args) < 1 {
		b.reply(chatID, "❌ 参数错误\n用法: /del <name>")
		return
	}

	name := args[0]

	if err := b.nodeManager.DeleteNode(name); err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 删除节点失败: %v", err))
		return
	}

	b.reply(chatID, fmt.Sprintf("✅ 节点 %s 已删除", name))
}

// handleEnable 启用节点
func (b *Bot) handleEnable(chatID int64, args []string) {
	if len(args) < 1 {
		b.reply(chatID, "❌ 参数错误\n用法: /enable <name>")
		return
	}

	name := args[0]

	if err := b.nodeManager.EnableNode(name, true); err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 启用节点失败: %v", err))
		return
	}

	b.reply(chatID, fmt.Sprintf("✅ 节点 %s 已启用", name))
}

// handleDisable 禁用节点
func (b *Bot) handleDisable(chatID int64, args []string) {
	if len(args) < 1 {
		b.reply(chatID, "❌ 参数错误\n用法: /disable <name>")
		return
	}

	name := args[0]

	if err := b.nodeManager.EnableNode(name, false); err != nil {
		b.reply(chatID, fmt.Sprintf("❌ 禁用节点失败: %v", err))
		return
	}

	b.reply(chatID, fmt.Sprintf("✅ 节点 %s 已禁用", name))
}

// handleStatus 查看节点状态
func (b *Bot) handleStatus(chatID int64) {
	allNodes := b.healthChecker.GetAllNodes()
	healthyNodes := b.healthChecker.GetHealthyNodes()

	if len(allNodes) == 0 {
		b.reply(chatID, "📭 当前没有配置任何节点")
		return
	}

	var sb strings.Builder
	sb.WriteString("🏥 *节点健康状态：*\n\n")

	healthyMap := make(map[string]bool)
	for _, node := range healthyNodes {
		healthyMap[node.GetName()] = true
	}

	for i, node := range allNodes {
		healthIcon := "❌ 不健康"
		if healthyMap[node.GetName()] {
			healthIcon = "✅ 健康"
		}

		if !node.IsEnabled() {
			healthIcon = "⛔ 已禁用"
		}

		sb.WriteString(fmt.Sprintf(
			"%d. *%s*\n   • Host: `%s`\n   • 权重: %d\n   • 状态: %s\n\n",
			i+1, node.GetName(), node.GetHost(), node.GetWeight(), healthIcon,
		))
	}

	sb.WriteString(fmt.Sprintf(
		"📊 *统计：*\n• 总节点数: %d\n• 健康节点: %d\n• 更新时间: %s",
		len(allNodes), len(healthyNodes), time.Now().Format("2006-01-02 15:04:05"),
	))

	b.replyMarkdown(chatID, sb.String())
}

// handleBatchAdd 批量添加节点
func (b *Bot) handleBatchAdd(chatID int64, args []string) {
	if len(args) < 1 {
		b.reply(chatID, "❌ 参数错误\n用法: /batchadd <host1> <host2> ...\n例如: /batchadd http://1.2.3.4:80 http://5.6.7.8:80:50")
		return
	}

	successCount, failedHosts, err := b.nodeManager.BatchAddNodes(args)

	var sb strings.Builder
	if successCount > 0 {
		sb.WriteString(fmt.Sprintf("✅ 成功添加 %d 个节点\n\n", successCount))
	}

	if len(failedHosts) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ 失败 %d 个节点:\n", len(failedHosts)))
		for _, host := range failedHosts {
			sb.WriteString(fmt.Sprintf("• %s\n", host))
		}
	}

	if err != nil && successCount == 0 {
		b.reply(chatID, fmt.Sprintf("❌ 批量添加失败: %v", err))
		return
	}

	sb.WriteString("\n正在进行健康检查...")
	b.reply(chatID, sb.String())
}

// handleBatchDelete 批量删除节点
func (b *Bot) handleBatchDelete(chatID int64, args []string) {
	if len(args) < 1 {
		b.reply(chatID, "❌ 参数错误\n用法: /batchdel <name1> <name2> ...\n例如: /batchdel 8.138.199.183 47.92.114.104")
		return
	}

	deletedCount, failedNames, err := b.nodeManager.BatchDeleteNodes(args)

	var sb strings.Builder
	if deletedCount > 0 {
		sb.WriteString(fmt.Sprintf("✅ 成功删除 %d 个节点\n\n", deletedCount))
	}

	if len(failedNames) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ 失败 %d 个节点（不存在）:\n", len(failedNames)))
		for _, name := range failedNames {
			sb.WriteString(fmt.Sprintf("• %s\n", name))
		}
	}

	if err != nil && deletedCount == 0 {
		b.reply(chatID, fmt.Sprintf("❌ 批量删除失败: %v", err))
		return
	}

	b.reply(chatID, sb.String())
}

// reply 发送普通消息
func (b *Bot) reply(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		logs.Error("[Telegram] 发送消息失败: %v", err)
	}
}

// replyMarkdown 发送 Markdown 格式消息
func (b *Bot) replyMarkdown(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	if _, err := b.api.Send(msg); err != nil {
		logs.Error("[Telegram] 发送消息失败: %v", err)
	}
}
