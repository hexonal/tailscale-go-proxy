package gost

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

// SOCKS5 协议常量
const (
	SOCKS5Version = 0x05 // SOCKS5 协议版本
	NoAuth        = 0x00 // 无需认证
	UserPassAuth  = 0x02 // 用户名密码认证
	ConnectCmd    = 0x01 // CONNECT 命令
	IPv4Addr      = 0x01 // IPv4 地址类型
	DomainAddr    = 0x03 // 域名地址类型
	IPv6Addr      = 0x04 // IPv6 地址类型
)

// SOCKS5Server 实现了基于用户名密码动态转发的 SOCKS5 代理服务。
// 支持标准 SOCKS5 协议，支持用户名密码认证，
// 可根据用户认证信息动态选择下游代理。
type SOCKS5Server struct {
	addr string
}

// NewSOCKS5Server 创建一个新的 SOCKS5Server 实例。
// 参数 addr 为监听地址（如 ":1080"），返回 SOCKS5Server 指针。
func NewSOCKS5Server(addr string) *SOCKS5Server {
	return &SOCKS5Server{addr: addr}
}

// Start 启动 SOCKS5 代理服务器，监听指定地址并处理客户端请求。
// 返回 error 表示启动或运行过程中遇到的错误。
func (s *SOCKS5Server) Start() error {
	// 启动 TCP 监听，等待客户端连接
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	log.Printf("SOCKS5 代理已启动，监听地址: %s", s.addr)
	for {
		// 接受新连接，主循环
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		// 每个连接独立 goroutine 处理，防止阻塞主循环
		go s.handleConnection(conn)
	}
}

// handleConnection 处理单个 SOCKS5 客户端连接。
// 完成认证、目标地址解析、下游代理选择和数据转发。
// 参数 conn 为客户端连接。
func (s *SOCKS5Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	// 1. 处理 SOCKS5 握手和认证
	username, password, err := s.handleHandshake(conn)
	if err != nil {
		log.Printf("Handshake error: %v", err)
		return
	}
	// 2. 根据认证信息查找下游代理
	proxyAddr := s.authenticate(username, password)
	if proxyAddr == "" {
		// 认证失败，返回认证失败响应
		conn.Write([]byte{0x01, 0x01})
		log.Printf("Authentication failed for user: %s", username)
		return
	}
	// 认证成功，返回认证成功响应（已在 handleHandshake 内部完成，不需要再次发送）
	log.Printf("User %s authenticated, using proxy: %s", username, proxyAddr)
	// 3. 解析客户端请求的目标地址
	targetAddr, err := s.handleConnect(conn)
	if err != nil {
		log.Printf("Connect handling error: %v", err)
		return
	}
	// 4. 通过下游代理建立到目标地址的连接
	connector, err := getProxyConnector(proxyAddr)
	if err != nil {
		log.Printf("getProxyConnector error: %v", err)
		// 返回 SOCKS5 连接失败响应
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	proxyConn, err := connector(targetAddr)
	if err != nil {
		log.Printf("Failed to connect to proxy %s: %v", proxyAddr, err)
		// 返回 SOCKS5 连接失败响应
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer proxyConn.Close()
	// 5. 通知客户端连接建立成功
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	// 6. 开始双向转发数据
	s.relay(conn, proxyConn)
}

// handleHandshake 处理 SOCKS5 握手和认证流程。
// 返回客户端提供的用户名、密码和错误信息。
// 参数 conn 为客户端连接。
// 返回值：用户名、密码、error。
func (s *SOCKS5Server) handleHandshake(conn net.Conn) (string, string, error) {
	// 读取客户端发来的 VER、NMETHODS
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", "", err
	}
	version, nMethods := buf[0], buf[1]
	if version != SOCKS5Version {
		return "", "", io.ErrUnexpectedEOF
	}
	// 读取支持的认证方法
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", "", err
	}
	// 检查是否支持用户名密码认证
	supportUserPass := false
	for _, m := range methods {
		if m == UserPassAuth {
			supportUserPass = true
			break
		}

	}
	if !supportUserPass {
		// 不支持则返回 0xFF，协议要求
		conn.Write([]byte{SOCKS5Version, 0xFF})
		return "", "", fmt.Errorf("client does not support username/password auth")
	}
	// 通知客户端选择用户名密码认证
	if _, err := conn.Write([]byte{SOCKS5Version, UserPassAuth}); err != nil {
		return "", "", err
	}
	// 读取认证子协商：VER、ULEN、UNAME、PLEN、PASSWD
	buf = make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", "", err
	}
	if buf[0] != 0x01 {
		return "", "", io.ErrUnexpectedEOF
	}
	usernameLen := buf[1]
	username := make([]byte, usernameLen)
	if _, err := io.ReadFull(conn, username); err != nil {
		return "", "", err
	}
	buf = make([]byte, 1)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", "", err
	}
	passwordLen := buf[0]
	password := make([]byte, passwordLen)
	if _, err := io.ReadFull(conn, password); err != nil {
		return "", "", err
	}
	return string(username), string(password), nil
}

// authenticate 根据用户名密码查找下游代理地址。
// 参数 username、password 为客户端认证信息。
// 返回值：匹配的下游代理地址字符串。
func (s *SOCKS5Server) authenticate(username, password string) string {
	// 拼接 key 查询用户映射表
	userKey := username + ":" + password
	userProxyMapLock.RLock()
	defer userProxyMapLock.RUnlock()
	return UserProxyMap[userKey]
}

// handleConnect 解析 SOCKS5 CONNECT 请求，获取目标地址。
// 参数 conn 为客户端连接。
// 返回值：目标地址字符串（host:port）、error。
func (s *SOCKS5Server) handleConnect(conn net.Conn) (string, error) {
	// 读取 SOCKS5 CONNECT 请求头部
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", err
	}
	version, cmd, _, addrType := buf[0], buf[1], buf[2], buf[3]
	if version != SOCKS5Version || cmd != ConnectCmd {
		return "", io.ErrUnexpectedEOF
	}
	// 解析目标地址
	var addr string
	switch addrType {
	case IPv4Addr:
		buf = make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		addr = net.IP(buf).String()
	case DomainAddr:
		buf = make([]byte, 1)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		domainLen := buf[0]
		domain := make([]byte, domainLen)
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", err
		}
		addr = string(domain)
	case IPv6Addr:
		buf = make([]byte, 16)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		addr = net.IP(buf).String()
	default:
		return "", io.ErrUnexpectedEOF
	}
	// 读取端口
	buf = make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", err
	}
	port := int(buf[0])<<8 + int(buf[1])
	return addr + ":" + fmt.Sprintf("%d", port), nil
}

// relay 实现两个连接之间的双向数据转发。
// 用于 CONNECT 隧道和 SOCKS5 隧道的数据转发。
// 参数 conn1、conn2 为需要互相转发数据的两个连接。
func (s *SOCKS5Server) relay(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	// 启动两个 goroutine 实现双向转发
	go func() {
		defer wg.Done()
		io.Copy(conn1, conn2) // 下游 -> 客户端
		conn1.Close()
	}()
	go func() {
		defer wg.Done()
		io.Copy(conn2, conn1) // 客户端 -> 下游
		conn2.Close()
	}()
	wg.Wait()
}
