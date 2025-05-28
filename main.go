package main

import (
	"log"
	"os"
	"strconv"
	"tailscale-go-proxy/internal/cache"
	"tailscale-go-proxy/internal/config"
	"tailscale-go-proxy/internal/headscale"
	"tailscale-go-proxy/internal/proxy"
	"tailscale-go-proxy/internal/register"

	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化缓存
	nodeCache := cache.NewNodeCache()

	// 启动 HTTP 代理服务（协程）
	go func() {
		if err := proxy.StartHTTPProxy(":8080"); err != nil {
			log.Fatalf("HTTP/HTTPS 代理启动失败: %v", err)
		}
	}()

	// 初始化 Gin
	r := gin.Default()

	// 注册 API 路由
	r.POST("/register", func(c *gin.Context) {
		register.HandleRegister(c, nodeCache, cfg)
	})
	r.GET("/nodes", func(c *gin.Context) {
		cache.HandleGetNodes(c, nodeCache)
	})

	// 从环境变量获取注册 key
	regKey := os.Getenv("HEADSCALE_REG_KEY")
	if regKey == "" {
		log.Fatal("请设置环境变量 HEADSCALE_REG_KEY 作为注册 key")
	}
	if err := headscale.RegisterNodeByDockerExec("flink", regKey); err != nil {
		log.Fatalf("自动注册节点失败: %v", err)
	}
	log.Println("节点注册成功")

	// 代理服务路由（仅骨架，实际监听见 proxy 包）
	// proxy.StartHTTPProxy(cfg, nodeCache)
	addr := ":1080" // 监听本地 1080 端口
	if err := proxy.StartSocks5Proxy(addr); err != nil {
		log.Fatalf("SOCKS5 代理启动失败: %v", err)
	}

	// 启动管理 API
	addr = ":" + strconv.Itoa(cfg.ManageAPIPort)
	log.Printf("管理 API 启动于 %s", addr)
	r.Run(addr)
}
