package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"

	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/config"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/constant"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/emby"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/node"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/telegram"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/service/userkey"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/util/logs/colors"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/web"
	"github.com/AmbitiousJun/go-emby2openlist/v2/internal/web/webport"
	"github.com/gin-gonic/gin"
)

var ginMode = gin.DebugMode

func main() {
	go func() { http.ListenAndServe(":60360", nil) }()

	dataRoot := parseFlag()

	if err := config.ReadFromFile(filepath.Join(dataRoot, "config.yml")); err != nil {
		log.Fatal(err)
	}

	printBanner()

	// 初始化节点健康检查
	logs.Info("正在初始化节点健康检查模块...")
	healthChecker := node.NewHealthChecker(config.C.Nodes)
	go healthChecker.Start()

	// 初始化节点选择器
	nodeSelector := node.NewSelector(healthChecker)

	// 初始化用户 Key 缓存
	logs.Info("正在初始化用户 Key 缓存模块...")
	keyCache := userkey.NewCache(config.C.Auth.UserKeyCacheTTL)

	// 初始化重定向模块
	emby.InitRedirect(nodeSelector, keyCache)

	// 启动鉴权服务器（如果启用）
	if config.C.Auth.EnableAuthServer {
		logs.Info("正在启动鉴权服务器...")
		if err := web.ListenAuthServer(keyCache, healthChecker, nodeSelector); err != nil {
			logs.Error("鉴权服务器启动失败: %v", err)
		}
	}

	// 启动 Telegram Bot（如果启用）
	if config.C.Telegram.Enable {
		logs.Info("正在启动 Telegram Bot...")
		bot, err := telegram.NewBot(healthChecker)
		if err != nil {
			logs.Error("Telegram Bot 启动失败: %v", err)
		} else {
			go bot.Start()
			logs.Success("Telegram Bot 启动成功")
		}
	}

	logs.Info("正在启动主服务...")
	gin.SetMode(ginMode)
	if err := web.Listen(); err != nil {
		log.Fatal(colors.ToRed(err.Error()))
	}
}

// parseFlag 转换命令行参数
func parseFlag() (dataRoot string) {
	ph := flag.Int("p", 8095, "HTTP 服务监听端口")
	phs := flag.Int("ps", 8094, "HTTPS 服务监听端口")
	printVersion := flag.Bool("version", false, "查看程序版本")
	dr := flag.String("dr", ".", "程序数据根目录")
	flag.Parse()

	if *printVersion {
		fmt.Println(constant.CurrentVersion)
		os.Exit(0)
	}

	dataRoot = "."
	if *dr != dataRoot {
		stat, err := os.Stat(*dr)
		if err != nil || !stat.IsDir() {
			log.Fatalf("数据根目录 [%s] 不存在", *dr)
		}
		dataRoot = *dr
	}

	if *ph == *phs {
		log.Fatal("HTTP 和 HTTPS 端口冲突")
	}
	webport.HTTP = strconv.Itoa(*ph)
	webport.HTTPS = strconv.Itoa(*phs)
	return
}

func printBanner() {
	fmt.Printf(colors.ToYellow(`
                                 _           ___                        _ _     _   
                                | |         |__ \                      | (_)   | |  
  __ _  ___ ______ ___ _ __ ___ | |__  _   _   ) |___  _ __   ___ _ __ | |_ ___| |_ 
 / _| |/ _ \______/ _ \ '_ | _ \| '_ \| | | | / // _ \| '_ \ / _ \ '_ \| | / __| __|
| (_| | (_) |    |  __/ | | | | | |_) | |_| |/ /| (_) | |_) |  __/ | | | | \__ \ |_ 
 \__, |\___/      \___|_| |_| |_|_.__/ \__, |____\___/| .__/ \___|_| |_|_|_|___/\__|
  __/ |                                 __/ |         | |                           
 |___/                                 |___/          |_|                           

 Repository: %s
    Version: %s
`), constant.RepoAddr, constant.CurrentVersion)
}
