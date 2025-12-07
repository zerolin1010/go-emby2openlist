package videoauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/node"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/userkey"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/https"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"
	"github.com/gin-gonic/gin"
)

// VideoAuthService 视频鉴权服务
type VideoAuthService struct {
	cache           *userkey.Cache
	embyHost        string
	adminApiKey     string
	secretKey       string
	tokenTTL        time.Duration
	uidCache        *userkey.Cache     // UID 到 api_key 的映射缓存
	playingSessions *userkey.Cache     // 播放会话跟踪（token -> 最后活跃时间）
	healthChecker   *node.HealthChecker // 节点健康检查器（用于故障转移）
	nodeSelector    *node.Selector     // 节点选择器（用于选择新节点）
}

// NewVideoAuthService 创建视频鉴权服务
func NewVideoAuthService(cache *userkey.Cache, cfg *config.Emby, healthChecker *node.HealthChecker, nodeSelector *node.Selector) *VideoAuthService {
	return &VideoAuthService{
		cache:           cache,
		embyHost:        cfg.Host,
		adminApiKey:     cfg.AdminApiKey,
		secretKey:       "go-emby2openlist-secret-2024", // TODO: 从配置读取
		tokenTTL:        5 * time.Minute,                // 临时 URL 有效期 5 分钟
		uidCache:        userkey.NewCache(10 * time.Minute), // UID 缓存有效期 10 分钟
		playingSessions: userkey.NewCache(30 * time.Minute), // 播放会话缓存 30 分钟
		healthChecker:   healthChecker,                      // 节点健康检查器
		nodeSelector:    nodeSelector,                       // 节点选择器
	}
}

// HandleVideoAuth 处理视频鉴权请求（返回 302 重定向）
func (s *VideoAuthService) HandleVideoAuth(c *gin.Context) {
	startTime := time.Now()

	// 1. 提取 api_key
	apiKey := c.Query("api_key")
	if apiKey == "" {
		apiKey = c.GetHeader("X-Emby-Token")
	}
	if apiKey == "" {
		logs.Warn("[VideoAuth] 缺少 api_key，路径: %s, IP: %s", c.Request.URL.Path, c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"error": "Missing api_key"})
		return
	}

	// 2. 验证 api_key（使用缓存）
	valid, err := s.validateApiKey(apiKey)
	if err != nil {
		logs.Error("[VideoAuth] 验证 API Key 失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Validation error"})
		return
	}

	if !valid {
		logs.Warn("[VideoAuth] 无效的 api_key，路径: %s, IP: %s", c.Request.URL.Path, c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid api_key"})
		return
	}

	// 3. 提取视频路径
	// 请求路径: /api/video-auth/data/Movie/xxx.mkv
	// 目标路径: /internal/data/Movie/xxx.mkv
	videoPath := strings.Replace(c.Request.URL.Path, "/api/video-auth/", "/internal/", 1)

	// 4. 生成临时签名 URL
	expiresAt := time.Now().Add(s.tokenTTL).Unix()
	token := s.generateToken(videoPath, apiKey, expiresAt)
	uid := s.encryptUID(apiKey) // 加密用户标识并缓存映射

	// 5. 记录访问日志
	logs.Info("[VideoAuth] 鉴权通过，生成临时 URL，用户: %s, 文件: %s, IP: %s, 耗时: %v",
		maskApiKey(apiKey), videoPath, c.ClientIP(), time.Since(startTime))

	// 6. 构建重定向 URL
	redirectURL := fmt.Sprintf("%s?token=%s&expires=%d&uid=%s",
		videoPath, token, expiresAt, uid)

	// 7. 返回 302 重定向
	c.Redirect(http.StatusFound, redirectURL)
}

// HandleVerifyToken 验证临时令牌（Nginx auth_request 调用）
func (s *VideoAuthService) HandleVerifyToken(c *gin.Context) {
	// 1. 提取参数
	token := c.Query("token")
	expiresStr := c.Query("expires")
	uid := c.Query("uid")
	path := c.Query("path") // Nginx 传递的原始路径

	if token == "" || expiresStr == "" || uid == "" || path == "" {
		logs.Warn("[TokenVerify] 缺少必需参数，IP: %s", c.ClientIP())
		c.Status(http.StatusForbidden)
		return
	}

	// 1.5. 检查故障转移重试次数（防止无限重定向）
	retryCount := 0
	if retryStr := c.Query("_retry"); retryStr != "" {
		fmt.Sscanf(retryStr, "%d", &retryCount)
	}
	const maxRetries = 3
	if retryCount >= maxRetries {
		logs.Error("[TokenVerify] 故障转移重试次数超限 (%d 次)，拒绝访问，路径: %s", retryCount, path)
		c.Status(http.StatusServiceUnavailable)
		return
	}

	// 2. 解析过期时间
	var expiresAt int64
	fmt.Sscanf(expiresStr, "%d", &expiresAt)

	now := time.Now()
	currentUnix := now.Unix()

	// 3. 检查是否过期
	if currentUnix > expiresAt {
		logs.Warn("[TokenVerify] Token 已过期，路径: %s, IP: %s", path, c.ClientIP())
		c.Status(http.StatusForbidden)
		return
	}

	// 4. 解密 uid 获取 api_key
	apiKey := s.decryptUID(uid)
	if apiKey == "" {
		logs.Warn("[TokenVerify] 无效的 UID，路径: %s, IP: %s", path, c.ClientIP())
		c.Status(http.StatusForbidden)
		return
	}

	// 5. 验证签名
	expectedToken := s.generateToken(path, apiKey, expiresAt)
	if token != expectedToken {
		logs.Warn("[TokenVerify] Token 签名无效，路径: %s, IP: %s", path, c.ClientIP())
		c.Status(http.StatusForbidden)
		return
	}

	// 6. 自动续期逻辑（基于播放会话）+ 节点健康检查
	sessionKey := fmt.Sprintf("%s:%s", token, uid)

	// 检查当前节点是否健康（实现自动故障转移）
	if s.healthChecker != nil && s.nodeSelector != nil {
		// 优先使用 Nginx 传递的 X-Node-Host 头（包含端口号）
		// 如果不存在，降级使用 Host 头
		requestHost := c.GetHeader("X-Node-Host")
		if requestHost == "" {
			requestHost = c.Request.Host
		}
		isHealthy := s.isNodeHealthy(requestHost)
		if !isHealthy {
			// 节点不健康 → 307 重定向到新节点 → 无缝故障转移
			logs.Warn("[TokenVerify] 节点不健康，执行故障转移: %s, 路径: %s, IP: %s",
				requestHost, path, c.ClientIP())

			// 选择新的健康节点
			newNode := s.nodeSelector.SelectNode()
			if newNode == nil {
				logs.Error("[TokenVerify] 没有可用的健康节点，拒绝访问")
				s.playingSessions.Delete(sessionKey)
				c.Status(http.StatusServiceUnavailable)
				return
			}

			// 重新生成签名 URL（指向新节点）
			newRedirectURL := s.buildFailoverURL(newNode.Host, path, apiKey, retryCount+1)
			logs.Info("[TokenVerify] 故障转移到新节点: %s (%s), 重试次数: %d, 新 URL: %s",
				newNode.Name, newNode.Host, retryCount+1, newRedirectURL)

			// 返回 307 临时重定向（保留 POST/Range 等方法）
			c.Redirect(http.StatusTemporaryRedirect, newRedirectURL)
			return
		}
	}

	// 检查是否存在活跃的播放会话
	if sessionExpiresStr, ok := s.playingSessions.Get(sessionKey); ok {
		// 会话存在，使用会话的过期时间代替 URL 中的过期时间
		var sessionExpires int64
		fmt.Sscanf(sessionExpiresStr, "%d", &sessionExpires)

		// 如果会话未过期，允许访问并续期
		if currentUnix <= sessionExpires {
			// 每次请求都续期会话（延长 5 分钟）
			newSessionExpires := currentUnix + int64(s.tokenTTL.Seconds())
			s.playingSessions.Set(sessionKey, fmt.Sprintf("%d", newSessionExpires))

			logs.Info("[TokenVerify] 播放会话续期，用户: %s, 文件: %s, IP: %s, 新过期时间: %s",
				maskApiKey(apiKey), path, c.ClientIP(),
				time.Unix(newSessionExpires, 0).Format("2006-01-02 15:04:05"))

			// 验证通过
			c.Status(http.StatusOK)
			return
		}

		// 会话已过期，删除会话
		s.playingSessions.Delete(sessionKey)
		logs.Warn("[TokenVerify] 播放会话已过期（闲置超过5分钟），路径: %s, IP: %s", path, c.ClientIP())
		c.Status(http.StatusForbidden)
		return
	}

	// 7. 首次访问，创建播放会话
	// 如果 Token 本身未过期，创建新会话（续期 5 分钟）
	sessionExpires := currentUnix + int64(s.tokenTTL.Seconds())
	s.playingSessions.Set(sessionKey, fmt.Sprintf("%d", sessionExpires))

	logs.Info("[TokenVerify] 创建播放会话，用户: %s, 文件: %s, IP: %s, 会话过期时间: %s",
		maskApiKey(apiKey), path, c.ClientIP(),
		time.Unix(sessionExpires, 0).Format("2006-01-02 15:04:05"))

	// 8. 返回 200 表示验证通过
	c.Status(http.StatusOK)
}

// validateApiKey 验证 API Key（使用缓存）
func (s *VideoAuthService) validateApiKey(apiKey string) (bool, error) {
	// 1. 先检查缓存
	if s.cache != nil {
		if _, ok := s.cache.Get(apiKey); ok {
			return true, nil
		}
	}

	// 2. 缓存未命中，调用 Emby API 验证
	url := fmt.Sprintf("%s/emby/System/Info?api_key=%s", s.embyHost, apiKey)

	resp, err := https.Get(url).Do()
	if err != nil {
		return false, fmt.Errorf("请求 Emby 失败: %v", err)
	}
	defer resp.Body.Close()

	// 401 表示无效
	if resp.StatusCode == http.StatusUnauthorized {
		return false, nil
	}

	// 3. 验证成功，加入缓存
	if resp.StatusCode == http.StatusOK && s.cache != nil {
		s.cache.Set(apiKey, apiKey)
	}

	return resp.StatusCode == http.StatusOK, nil
}

// isNodeHealthy 检查节点是否健康（基于请求的 Host）
func (s *VideoAuthService) isNodeHealthy(requestHost string) bool {
	if s.healthChecker == nil {
		return true // 健康检查器未初始化，默认认为健康
	}

	// 获取所有节点
	allNodes := s.healthChecker.GetAllNodes()

	// 提取请求的 IP 地址（忽略端口号）
	requestIP := requestHost
	if host, _, err := net.SplitHostPort(requestHost); err == nil {
		requestIP = host
	}

	// 遍历查找匹配的节点
	for _, nodeStatus := range allNodes {
		// 解析节点的 Host
		nodeURL, err := url.Parse(nodeStatus.GetHost())
		if err != nil {
			continue
		}

		// 提取节点的 IP 地址（忽略端口号）
		nodeIP := nodeURL.Hostname()

		// 比较 IP 地址（忽略端口号）
		if nodeIP == requestIP || nodeURL.Host == requestHost || nodeStatus.GetHost() == requestHost {
			// 找到匹配的节点，返回健康状态
			isHealthy := nodeStatus.IsHealthy()
			if !isHealthy {
				logs.Warn("[VideoAuth] 节点不健康: %s (%s)", nodeStatus.GetName(), nodeStatus.GetHost())
			}
			return isHealthy
		}
	}

	// 未找到匹配的节点，可能是直接访问 Emby，认为健康
	logs.Info("[VideoAuth] 未找到匹配的节点: %s, 认为健康", requestHost)
	return true
}

// generateToken 生成临时签名
func (s *VideoAuthService) generateToken(path, apiKey string, expiresAt int64) string {
	data := fmt.Sprintf("%s:%s:%d", path, apiKey, expiresAt)
	hash := hmac.New(sha256.New, []byte(s.secretKey))
	hash.Write([]byte(data))
	return hex.EncodeToString(hash.Sum(nil))[:16] // 取前 16 位
}

// encryptUID 加密用户标识并缓存映射关系
func (s *VideoAuthService) encryptUID(apiKey string) string {
	hash := hmac.New(sha256.New, []byte(s.secretKey))
	hash.Write([]byte(apiKey))
	uid := hex.EncodeToString(hash.Sum(nil))[:8] // 取前 8 位作为标识

	// 缓存 UID -> api_key 映射，用于后续解密
	s.uidCache.Set(uid, apiKey)

	return uid
}

// decryptUID 解密用户标识（从缓存查找）
func (s *VideoAuthService) decryptUID(uid string) string {
	// 从缓存中查找 UID 对应的 api_key
	if apiKey, ok := s.uidCache.Get(uid); ok {
		return apiKey
	}
	// 缓存未命中，可能是 UID 过期或无效
	return ""
}

// maskApiKey 隐藏 API Key 的部分内容
func maskApiKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
}

// buildFailoverURL 构建故障转移 URL
func (s *VideoAuthService) buildFailoverURL(nodeHost, internalPath, apiKey string, retryCount int) string {
	// 1. 解析节点地址
	u, err := url.Parse(nodeHost)
	if err != nil {
		logs.Error("[Failover] 解析节点地址失败: %v", err)
		return ""
	}

	// 2. 将 /internal/dataX/... 转换为 /video/dataX/...
	publicPath := strings.Replace(internalPath, "/internal/", "/video/", 1)

	// 3. 设置路径
	u.Path = publicPath

	// 4. 添加 api_key 和 _retry 参数
	q := u.Query()
	q.Set("api_key", apiKey)
	if retryCount > 0 {
		q.Set("_retry", fmt.Sprintf("%d", retryCount))
	}
	u.RawQuery = q.Encode()

	return u.String()
}
