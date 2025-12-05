package config

// Nodes 节点配置
type Nodes struct {
	HealthCheck HealthCheck `yaml:"health-check"`
	List        []Node      `yaml:"list"`
}

// HealthCheck 健康检查配置
type HealthCheck struct {
	Interval         int `yaml:"interval"`          // 检查间隔(秒)
	Timeout          int `yaml:"timeout"`           // 超时时间(秒)
	FailThreshold    int `yaml:"fail-threshold"`    // 失败阈值
	SuccessThreshold int `yaml:"success-threshold"` // 成功阈值
}

// Node 单个节点配置
type Node struct {
	Name    string `yaml:"name"`
	Host    string `yaml:"host"`
	Weight  int    `yaml:"weight"`
	Enabled bool   `yaml:"enabled"`
}
