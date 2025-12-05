package web

import (
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/authserver"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/userkey"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"

	"github.com/gin-gonic/gin"
)

var (
	authServerInstance *authserver.Server
	accessLogger       *authserver.AccessLogger
)

// ListenAuthServer 启动鉴权服务器
func ListenAuthServer(cache *userkey.Cache) error {
	if !config.C.Auth.EnableAuthServer {
		logs.Info("鉴权服务器未启用")
		return nil
	}

	// 初始化访问日志记录器
	var err error
	accessLogger, err = authserver.NewAccessLogger(
		config.C.Auth.AuthServerLogPath,
		config.C.Auth.EnableAuthServerLog,
	)
	if err != nil {
		logs.Error("初始化访问日志记录器失败: %v", err)
		return err
	}

	// 初始化鉴权服务器
	authServerInstance = authserver.NewServer(cache, &config.C.Emby, accessLogger)

	// 创建 Gin 引擎
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(CustomLogger("8097")) // 鉴权服务端口

	// 注册路由
	api := r.Group("/api")
	{
		// 鉴权接口（供 Nginx auth_request 使用）
		api.GET("/auth", authServerInstance.HandleAuth)

		// 鉴权并重定向接口（可选）
		api.GET("/auth-redirect", authServerInstance.HandleAuthAndRedirect)

		// 统计接口
		api.GET("/stats", authServerInstance.HandleStats)

		// 健康检查
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status": "ok",
				"service": "auth-server",
			})
		})
	}

	// 启动服务
	port := config.C.Auth.AuthServerPort
	logs.Success("鉴权服务器启动在端口: %s", port)

	errChan := make(chan error, 1)
	go func() {
		err := r.Run("0.0.0.0:" + port)
		errChan <- err
	}()

	return nil
}

// CloseAuthServer 关闭鉴权服务器
func CloseAuthServer() error {
	if accessLogger != nil {
		return accessLogger.Close()
	}
	return nil
}
