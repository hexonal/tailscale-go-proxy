package main

import (
	"log"
	"strconv"
	"tailscale-go-proxy/internal/api"
	"tailscale-go-proxy/internal/config"
	"tailscale-go-proxy/internal/gost"
	"tailscale-go-proxy/internal/service"
	"tailscale-go-proxy/internal/tailscale"
)

func main() {
	// 1. 加载配置
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 打印 TS_AUTHKEY 前后几位，便于排查
	tsAuthKey := cfg.TSAuthKey
	log.Printf("[DEBUG] TS_AUTHKEY: %s (共%d位)", tsAuthKey, len(tsAuthKey))

	log.Printf("[DEBUG] 配置内容: %+v", cfg)

	// 2. 启动 tailscaled 并 up
	if err := tailscale.EnsureReady(tsAuthKey, cfg.LoginServer); err != nil {
		log.Fatalf("Tailscale 启动失败: %v", err)
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
