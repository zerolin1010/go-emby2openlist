package config

// Telegram Bot 配置
type Telegram struct {
	Enable      bool     `yaml:"enable"`       // 是否启用 Telegram Bot
	BotToken    string   `yaml:"bot-token"`    // Bot Token (从 @BotFather 获取)
	AdminUserID []int64  `yaml:"admin-users"`  // 管理员用户 ID 列表
	WebhookMode bool     `yaml:"webhook-mode"` // 是否使用 Webhook 模式（默认使用轮询）
	WebhookURL  string   `yaml:"webhook-url"`  // Webhook URL (仅 webhook 模式需要)
}
