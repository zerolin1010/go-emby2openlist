package config

import "time"

// Auth 鉴权配置
type Auth struct {
	UserKeyCacheTTL time.Duration `yaml:"user-key-cache-ttl"`
	NginxAuthEnable bool          `yaml:"nginx-auth-enable"`
}
