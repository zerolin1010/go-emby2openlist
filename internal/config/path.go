package config

import (
	"fmt"
	"strings"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"
)

type Path struct {
	// Emby2Nginx Emby 的路径前缀映射到 Nginx 的路径前缀, 两个路径使用 : 符号隔开
	Emby2Nginx []string `yaml:"emby2nginx"`

	// emby2NginxArr 根据 Emby2Nginx 转换成路径键值对数组
	emby2NginxArr [][2]string
}

func (p *Path) Init() error {
	p.emby2NginxArr = make([][2]string, 0, len(p.Emby2Nginx))
	for _, e2n := range p.Emby2Nginx {
		arr := strings.Split(e2n, ":")
		if len(arr) != 2 {
			return fmt.Errorf("path.emby2nginx 配置错误, %s 无法根据 ':' 进行分割", e2n)
		}
		p.emby2NginxArr = append(p.emby2NginxArr, [2]string{arr[0], arr[1]})
	}
	return nil
}

// MapEmby2Nginx 将 emby 路径映射成 nginx 路径
func (p *Path) MapEmby2Nginx(embyPath string) (string, bool) {
	for _, cfg := range p.emby2NginxArr {
		ep, np := cfg[0], cfg[1]
		// 完全匹配或者是路径分隔符后的前缀
		if embyPath == ep || strings.HasPrefix(embyPath, ep+"/") {
			logs.Tip("命中 emby2nginx 路径映射: %s => %s (如命中错误, 请将正确的映射配置前移)", ep, np)
			return strings.Replace(embyPath, ep, np, 1), true
		}
	}
	return "", false
}
