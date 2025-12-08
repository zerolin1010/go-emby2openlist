package emby

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/node"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/userkey"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/web/cache"

	"github.com/gin-gonic/gin"
)

var (
	nodeSelector *node.Selector
	userKeyCache *userkey.Cache
)

// InitRedirect 初始化重定向模块
func InitRedirect(selector *node.Selector, keyCache *userkey.Cache) {
	nodeSelector = selector
	userKeyCache = keyCache
}

// Redirect2NginxLink 重定向到 Nginx 节点直链
func Redirect2NginxLink(c *gin.Context) {
	// 1. 解析请求的资源信息
	itemInfo, err := resolveItemInfo(c, RouteStream)
	if checkErr(c, err) {
		return
	}
	logs.Info("解析到的 itemInfo: %v", itemInfo)

	// 2. 获取 Emby 中的媒体路径
	embyPath, err := getEmbyFileLocalPath(itemInfo)
	if checkErr(c, err) {
		return
	}
	logs.Info("Emby 媒体路径: %s", embyPath)

	// 3. 如果是本地媒体，回源处理
	if strings.HasPrefix(embyPath, config.C.Emby.LocalMediaRoot) {
		logs.Info("本地媒体: %s, 回源处理", embyPath)
		ProxyOrigin(c)
		return
	}

	// 4. 转换为 Nginx 路径
	nginxPath, ok := config.C.Path.MapEmby2Nginx(embyPath)
	if !ok {
		checkErr(c, fmt.Errorf("无法映射 Emby 路径到 Nginx: %s", embyPath))
		return
	}
	logs.Info("Nginx 路径: %s", nginxPath)

	// 5. 获取用户 API Key (用于 Nginx 鉴权)
	userApiKey := userKeyCache.GetOrFetch(itemInfo.Id, itemInfo.ApiKey)

	// 6. 构建重定向 URL
	var redirectUrl string
	fixedProxyURL := getFixedProxyURL()
	if fixedProxyURL != "" {
		// 使用固定前置代理 URL（测试模式）
		redirectUrl = buildRedirectUrl(fixedProxyURL, nginxPath, userApiKey)
		logs.Info("[主服务] 使用固定前置代理: %s", fixedProxyURL)
	} else {
		// 使用节点选择器（原有逻辑）
		selectedNode := nodeSelector.SelectNode()
		if selectedNode == nil {
			checkErr(c, fmt.Errorf("没有可用的健康节点"))
			return
		}
		logs.Info("选择节点: %s (%s)", selectedNode.Name, selectedNode.Host)
		redirectUrl = buildRedirectUrl(selectedNode.Host, nginxPath, userApiKey)
	}
	logs.Success("重定向到: %s", redirectUrl)

	// 8. 设置缓存时间
	c.Header(cache.HeaderKeyExpired, cache.Duration(time.Minute*10))

	// 9. 返回 302 重定向
	c.Redirect(http.StatusTemporaryRedirect, redirectUrl)
}

// convertToNginxPath 将 Emby 路径转换为 Nginx 路径（已废弃，使用 config.C.Path.MapEmby2Nginx）
func convertToNginxPath(embyPath string) string {
	nginxPath, _ := config.C.Path.MapEmby2Nginx(embyPath)
	return nginxPath
}

// buildRedirectUrl 构建重定向 URL
func buildRedirectUrl(nodeHost, nginxPath, apiKey string) string {
	u, err := url.Parse(nodeHost)
	if err != nil {
		logs.Error("解析节点地址失败: %v", err)
		return ""
	}

	// 拼接路径
	u.Path = nginxPath

	// 添加鉴权参数
	if config.C.Auth.NginxAuthEnable && apiKey != "" {
		q := u.Query()
		q.Set("api_key", apiKey)
		u.RawQuery = q.Encode()
	}

	return u.String()
}

// ProxyOriginalResource 拦截 original 接口
func ProxyOriginalResource(c *gin.Context) {
	// 字幕请求直接代理回源
	if strings.Contains(strings.ToLower(c.Request.RequestURI), "subtitles") {
		ProxyOrigin(c)
		return
	}

	itemInfo, err := resolveItemInfo(c, RouteOriginal)
	if checkErr(c, err) {
		return
	}

	embyPath, err := getEmbyFileLocalPath(itemInfo)
	if checkErr(c, err) {
		return
	}

	// 如果是本地媒体, 代理回源
	if strings.HasPrefix(embyPath, config.C.Emby.LocalMediaRoot) {
		ProxyOrigin(c)
		return
	}

	// 重定向到 Nginx
	Redirect2NginxLink(c)
}

// checkErr 检查 err 是否为空
// 不为空则根据错误处理策略返回响应
//
// 返回 true 表示请求已经被处理
func checkErr(c *gin.Context, err error) bool {
	if err == nil || c == nil {
		return false
	}

	// 异常接口, 不缓存
	c.Header(cache.HeaderKeyExpired, "-1")

	// 采用拒绝策略, 直接返回错误
	if config.C.Emby.ProxyErrorStrategy == config.PeStrategyReject {
		logs.Error("代理接口失败: %v", err)
		c.String(http.StatusInternalServerError, "代理接口失败, 请检查日志")
		return true
	}

	logs.Error("代理接口失败: %v, 回源处理", err)
	ProxyOrigin(c)
	return true
}

// getFixedProxyURL 获取固定前置代理 URL（测试模式）
func getFixedProxyURL() string {
	if config.C == nil || config.C.Auth == nil {
		return ""
	}
	return config.C.Auth.FixedProxyURL
}
