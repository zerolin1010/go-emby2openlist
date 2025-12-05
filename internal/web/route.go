package web

import (
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/constant"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/emby"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"

	"github.com/gin-gonic/gin"
)

// rules 预定义路由拦截规则, 以及相应的处理器
//
// 每个规则为一个切片, 参数分别是: 正则表达式, 处理器
var rules [][2]any

func initRulePatterns() {
	logs.Info("正在初始化路由规则...")
	rules = compileRules([][2]any{
		// websocket
		{constant.Reg_Socket, emby.ProxySocket()},

		// PlaybackInfo 接口
		{constant.Reg_PlaybackInfo, emby.TransferPlaybackInfo},

		// 播放进度
		{constant.Reg_PlayingStopped, emby.PlayingStoppedHelper},
		{constant.Reg_PlayingProgress, emby.PlayingProgressHelper},

		// Items 接口
		{constant.Reg_UserItems, emby.LoadCacheItems},

		// 剧集排序
		{constant.Reg_ShowEpisodes, emby.ResortEpisodes},

		// 字幕
		{constant.Reg_VideoSubtitles, emby.ProxySubtitles},

		// 资源重定向到 Nginx 直链 (核心)
		{constant.Reg_ResourceStream, emby.Redirect2NginxLink},
		{constant.Reg_ResourceOriginal, emby.ProxyOriginalResource},

		// 下载接口
		{constant.Reg_ItemDownload, emby.Redirect2NginxLink},
		{constant.Reg_ItemSyncDownload, emby.HandleSyncDownload},

		// 图片
		{constant.Reg_Images, emby.HandleImages},

		// 根路径
		{constant.Reg_Root, emby.ProxyRoot},

		// 其余资源代理回源
		{constant.Reg_All, emby.ProxyOrigin},
	})
	logs.Success("路由规则初始化完成")
}

// initRoutes 初始化路由
func initRoutes(r *gin.Engine) {
	r.Any("/*vars", globalDftHandler)
}
