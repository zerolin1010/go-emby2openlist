package config

import "time"

// Auth 鉴权配置
type Auth struct {
	UserKeyCacheTTL time.Duration `yaml:"user-key-cache-ttl"`
	NginxAuthEnable bool          `yaml:"nginx-auth-enable"`

	// 鉴权服务器配置
	EnableAuthServer    bool   `yaml:"enable-auth-server"`     // 是否启用鉴权服务器
	AuthServerPort      string `yaml:"auth-server-port"`       // 鉴权服务器端口
	EnableAuthServerLog bool   `yaml:"enable-auth-server-log"` // 是否启用访问日志
	AuthServerLogPath   string `yaml:"auth-server-log-path"`   // 访问日志路径

	// 固定前置代理配置（测试用）
	FixedProxyURL string `yaml:"fixed-proxy-url"` // 固定的前置代理完整 URL，如 "http://cdn.example.com:7777" 或 "https://cdn.example.com"
}
