package authserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/userkey"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/https"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"

	"github.com/gin-gonic/gin"
)

// Server 鉴权服务器
type Server struct {
	cache       *userkey.Cache
	embyHost    string
	adminApiKey string
	logger      *AccessLogger
}

// NewServer 创建鉴权服务器
func NewServer(cache *userkey.Cache, cfg *config.Emby, logger *AccessLogger) *Server {
	return &Server{
		cache:       cache,
		embyHost:    cfg.Host,
		adminApiKey: cfg.AdminApiKey,
		logger:      logger,
	}
}

// HandleAuth 处理 Nginx auth_request 鉴权请求
// 返回 200 表示鉴权通过，返回 403 表示鉴权失败
func (s *Server) HandleAuth(c *gin.Context) {
	startTime := time.Now()

	// 1. 提取 api_key 参数
	apiKey := c.Query("api_key")
	if apiKey == "" {
		apiKey = c.GetHeader("X-Emby-Token")
	}
	if apiKey == "" {
		apiKey = extractTokenFromAuth(c.GetHeader("Authorization"))
	}

	if apiKey == "" {
		s.logAuthFailed(c, "missing_api_key", startTime)
		c.JSON(http.StatusForbidden, gin.H{"error": "Missing api_key"})
		return
	}

	// 2. 验证 api_key
	valid, err := s.validateApiKey(apiKey)
	if err != nil {
		logs.Error("验证 API Key 失败: %v", err)
		s.logAuthFailed(c, "validation_error", startTime)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Validation error"})
		return
	}

	if !valid {
		s.logAuthFailed(c, "invalid_api_key", startTime)
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid api_key"})
		return
	}

	// 3. 鉴权成功，记录日志
	s.logAuthSuccess(c, apiKey, startTime)
	c.Status(http.StatusOK)
}

// HandleAuthAndRedirect 处理鉴权并返回 302 重定向
// 这是一个更高级的接口，Nginx 可以直接使用
func (s *Server) HandleAuthAndRedirect(c *gin.Context) {
	startTime := time.Now()

	// 1. 提取参数
	apiKey := c.Query("api_key")
	targetPath := c.Query("target_path") // 例如：/video/data/movie.mp4
	nodeHost := c.Query("node_host")     // 例如：http://nginx-node

	if apiKey == "" || targetPath == "" || nodeHost == "" {
		s.logAuthFailed(c, "missing_parameters", startTime)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters"})
		return
	}

	// 2. 验证 api_key
	valid, err := s.validateApiKey(apiKey)
	if err != nil || !valid {
		s.logAuthFailed(c, "invalid_api_key", startTime)
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid api_key"})
		return
	}

	// 3. 构建重定向 URL
	redirectUrl := fmt.Sprintf("%s%s", nodeHost, targetPath)

	// 4. 记录访问日志
	s.logAccess(c, apiKey, redirectUrl, startTime)

	// 5. 返回 302 重定向
	c.Redirect(http.StatusTemporaryRedirect, redirectUrl)
}

// validateApiKey 验证 API Key 是否有效
func (s *Server) validateApiKey(apiKey string) (bool, error) {
	// 调用 Emby API 验证
	url := fmt.Sprintf("%s/emby/System/Info?api_key=%s", s.embyHost, apiKey)

	resp, err := https.Get(url).Timeout(5 * time.Second).Do()
	if err != nil {
		return false, fmt.Errorf("请求 Emby 失败: %v", err)
	}
	defer resp.Body.Close()

	// 401 表示无效
	if resp.StatusCode == http.StatusUnauthorized {
		return false, nil
	}

	return resp.StatusCode == http.StatusOK, nil
}

// logAuthSuccess 记录鉴权成功日志
func (s *Server) logAuthSuccess(c *gin.Context, apiKey string, startTime time.Time) {
	if s.logger == nil {
		return
	}

	duration := time.Since(startTime)
	log := AccessLog{
		Timestamp:    startTime,
		RemoteIP:     c.ClientIP(),
		Method:       c.Request.Method,
		URI:          c.Request.RequestURI,
		Status:       http.StatusOK,
		ApiKey:       maskApiKey(apiKey),
		UserAgent:    c.GetHeader("User-Agent"),
		Referer:      c.GetHeader("Referer"),
		Duration:     duration,
		AuthResult:   "success",
		ErrorReason:  "",
		OriginalPath: c.Query("target_path"),
	}

	s.logger.Log(log)
}

// logAuthFailed 记录鉴权失败日志
func (s *Server) logAuthFailed(c *gin.Context, reason string, startTime time.Time) {
	if s.logger == nil {
		return
	}

	duration := time.Since(startTime)
	log := AccessLog{
		Timestamp:    startTime,
		RemoteIP:     c.ClientIP(),
		Method:       c.Request.Method,
		URI:          c.Request.RequestURI,
		Status:       http.StatusForbidden,
		ApiKey:       maskApiKey(c.Query("api_key")),
		UserAgent:    c.GetHeader("User-Agent"),
		Referer:      c.GetHeader("Referer"),
		Duration:     duration,
		AuthResult:   "failed",
		ErrorReason:  reason,
		OriginalPath: c.Query("target_path"),
	}

	s.logger.Log(log)
}

// logAccess 记录访问日志（302 重定向）
func (s *Server) logAccess(c *gin.Context, apiKey, redirectUrl string, startTime time.Time) {
	if s.logger == nil {
		return
	}

	duration := time.Since(startTime)
	log := AccessLog{
		Timestamp:    startTime,
		RemoteIP:     c.ClientIP(),
		Method:       c.Request.Method,
		URI:          c.Request.RequestURI,
		Status:       http.StatusTemporaryRedirect,
		ApiKey:       maskApiKey(apiKey),
		UserAgent:    c.GetHeader("User-Agent"),
		Referer:      c.GetHeader("Referer"),
		Duration:     duration,
		AuthResult:   "success",
		ErrorReason:  "",
		RedirectURL:  redirectUrl,
		OriginalPath: c.Query("target_path"),
	}

	s.logger.Log(log)
}

// HandleStats 处理统计信息查询
func (s *Server) HandleStats(c *gin.Context) {
	if s.logger == nil {
		c.JSON(http.StatusOK, gin.H{"error": "Logger not initialized"})
		return
	}

	stats := s.logger.GetStats()
	c.JSON(http.StatusOK, stats)
}

// extractTokenFromAuth 从 Authorization 头提取 token
func extractTokenFromAuth(auth string) string {
	if auth == "" {
		return ""
	}

	// 格式：MediaBrowser Token="xxx"
	if strings.Contains(auth, "Token=") {
		parts := strings.Split(auth, "Token=")
		if len(parts) == 2 {
			token := strings.Trim(parts[1], "\"")
			return token
		}
	}

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

// Stats 统计信息
type Stats struct {
	TotalRequests   int64              `json:"total_requests"`
	SuccessRequests int64              `json:"success_requests"`
	FailedRequests  int64              `json:"failed_requests"`
	FailReasons     map[string]int64   `json:"fail_reasons"`
	LastHourStats   HourlyStats        `json:"last_hour_stats"`
	TopUsers        []UserStats        `json:"top_users"`
	AverageDuration time.Duration      `json:"average_duration"`
	mu              sync.RWMutex       `json:"-"`
}

// HourlyStats 每小时统计
type HourlyStats struct {
	Requests int64 `json:"requests"`
	Success  int64 `json:"success"`
	Failed   int64 `json:"failed"`
}

// UserStats 用户统计
type UserStats struct {
	ApiKey   string `json:"api_key"`
	Requests int64  `json:"requests"`
	LastSeen time.Time `json:"last_seen"`
}

// AccessLog 访问日志
type AccessLog struct {
	Timestamp    time.Time     `json:"timestamp"`
	RemoteIP     string        `json:"remote_ip"`
	Method       string        `json:"method"`
	URI          string        `json:"uri"`
	Status       int           `json:"status"`
	ApiKey       string        `json:"api_key"`       // 已脱敏
	UserAgent    string        `json:"user_agent"`
	Referer      string        `json:"referer"`
	Duration     time.Duration `json:"duration"`
	AuthResult   string        `json:"auth_result"`   // success/failed
	ErrorReason  string        `json:"error_reason"`  // 失败原因
	RedirectURL  string        `json:"redirect_url"`  // 302 重定向地址
	OriginalPath string        `json:"original_path"` // 原始请求路径
}

// ToJSON 转换为 JSON 字符串
func (l AccessLog) ToJSON() string {
	data, _ := json.Marshal(l)
	return string(data)
}
