package proxy

import (
	"log"
	"tailscale-go-proxy/internal/cache"
	"tailscale-go-proxy/internal/config"

	"github.com/txthinking/socks5"
)

func StartSocks5Proxy(addr string) error {
	server, err := socks5.NewClassicServer(addr, "", "", "", 60, 60)
	if err != nil {
		return err
	}
	log.Printf("SOCKS5 代理启动，监听地址: %s", addr)
	return server.ListenAndServe(nil)
}

func StartSOCKS5Proxy(cfg *config.Config, nodeCache *cache.NodeCache) {
	// TODO: 启动 SOCKS5 代理监听 cfg.SOCKS5ProxyPort
	// 负载均衡选择节点，转发流量
}
