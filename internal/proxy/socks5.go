package proxy

import (
	"database/sql"
	"errors"
	"io"
	"log"
	"net"
	"tailscale-go-proxy/internal/cache"
	"tailscale-go-proxy/internal/config"

	"github.com/txthinking/socks5"
)

// KeyToIP 查询 key 对应的 ip
func KeyToIP(db *sql.DB, key string) (string, error) {
	var ip string
	err := db.QueryRow("SELECT ip_address FROM register_key_ip_map WHERE reg_key = $1", key).Scan(&ip)
	if err != nil {
		return "", err
	}
	return ip, nil
}

// CustomHandler 实现 socks5.Handler
type CustomHandler struct {
	DB *sql.DB
}

// TCPHandle 认证并强制转发到 ip:8939
func (h *CustomHandler) TCPHandle(s *socks5.Server, client *net.TCPConn, req *socks5.Request) error {
	user := string(req.User)
	pass := string(req.Password)
	if user != "any" || pass == "" {
		client.Close()
		return errors.New("认证失败")
	}
	ip, err := KeyToIP(h.DB, pass)
	if err != nil {
		client.Close()
		return err
	}
	target := net.JoinHostPort(ip, "8939")
	remote, err := net.Dial("tcp", target)
	if err != nil {
		client.Close()
		return err
	}
	defer remote.Close()

	// 双向转发
	go io.Copy(remote, client)
	io.Copy(client, remote)
	return nil
}

// UDPHandle 不实现
func (h *CustomHandler) UDPHandle(s *socks5.Server, addr *net.UDPAddr, d *socks5.Datagram) error {
	return nil
}

// StartSocks5ProxyWithDB 启动 socks5 代理
func StartSocks5ProxyWithDB(addr string, db *sql.DB) error {
	handler := &CustomHandler{DB: db}
	server, err := socks5.NewClassicServer(addr, "", "", "", 0, 60)
	if err != nil {
		return err
	}
	log.Printf("SOCKS5 代理启动，监听地址: %s", addr)
	return server.ListenAndServe(handler)
}

func StartSOCKS5Proxy(cfg *config.Config, nodeCache *cache.NodeCache) {
	// TODO: 启动 SOCKS5 代理监听 cfg.SOCKS5ProxyPort
	// 负载均衡选择节点，转发流量
}
