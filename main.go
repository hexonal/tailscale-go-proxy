package main

import (
	"database/sql"
	"log"
	"strconv"
	"tailscale-go-proxy/internal/cache"
	"tailscale-go-proxy/internal/config"
	"tailscale-go-proxy/internal/proxy"
	"tailscale-go-proxy/internal/register"

	_ "github.com/lib/pq"

	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化数据库连接
	db, err := sql.Open("postgres", "host=pg user=xxx password=xxx dbname=headscale sslmode=disable")
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer db.Close()

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
		register.HandleRegister(c, db)
	})
	r.GET("/nodes", func(c *gin.Context) {
		cache.HandleGetNodes(c, cache.NewNodeCache())
	})

	// 代理服务路由（仅骨架，实际监听见 proxy 包）
	addr := ":1080" // 监听本地 1080 端口
	if err := proxy.StartSocks5Proxy(addr); err != nil {
		log.Fatalf("SOCKS5 代理启动失败: %v", err)
	}

	// 启动管理 API
	addr = ":" + strconv.Itoa(cfg.ManageAPIPort)
	log.Printf("管理 API 启动于 %s", addr)
	r.Run(addr)
}
