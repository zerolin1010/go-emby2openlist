package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	configFilePath string
	saveMutex      sync.Mutex
)

// SetConfigPath 设置配置文件路径
func SetConfigPath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("获取配置文件绝对路径失败: %v", err)
	}
	configFilePath = absPath
	return nil
}

// GetConfigPath 获取配置文件路径
func GetConfigPath() string {
	return configFilePath
}

// SaveToFile 保存配置到文件
func SaveToFile() error {
	saveMutex.Lock()
	defer saveMutex.Unlock()

	if configFilePath == "" {
		return fmt.Errorf("配置文件路径未设置")
	}

	if C == nil {
		return fmt.Errorf("配置对象为空")
	}

	// 序列化配置
	data, err := yaml.Marshal(C)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	// Docker 挂载文件不能使用 rename（会报 device or resource busy）
	// 直接覆盖写入（虽然不是原子操作，但在单进程环境下安全）
	if err := os.WriteFile(configFilePath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}
