package main

import (
	"log"
	"os"
	"strconv"
	"tailscale-go-proxy/internal/config"
	"tailscale-go-proxy/internal/tailscale"
	"tailscale-go-proxy/internal/gost"
	"tailscale-go-proxy/internal/api"
	"tailscale-go-proxy/internal/service"
)

func main() {
	// 1. 启动 tailscaled 并 up
	if err := tailscale.EnsureReady(os.Getenv("TS_AUTHKEY")); err != nil {
		log.Fatalf("Tailscale 启动失败: %v", err)
	}

	// 2. 加载配置
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 3. 初始化数据库
	db := service.MustInitDB(cfg)
	defer db.Close()

	// 4. 启动 gost
	if err := gost.EnsureReady(db); err != nil {
		log.Fatalf("gost 启动失败: %v", err)
	}

	// 5. 启动 gin 路由
	r := api.NewRouter(db)
	log.Printf("管理 API 启动于 :%d", cfg.ManageAPIPort)
	r.Run(":" + strconv.Itoa(cfg.ManageAPIPort))
}
