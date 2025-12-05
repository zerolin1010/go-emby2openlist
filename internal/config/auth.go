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
}
