// Package gost 实现了基于用户名密码动态转发的 SOCKS5/HTTP 代理服务
package gost

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ========== 配置加载 ===========
// UserProxyMap 全局用户转发表，key 为 "username:password"，value 为目标转发地址
var UserProxyMap = make(map[string]string)

// userProxyMapLock 用于保护 UserProxyMap 的并发读写
var userProxyMapLock sync.RWMutex

// LoadUserProxyMap 加载 gost-config.yaml 到 UserProxyMap（非线程安全，建议仅初始化时调用）
func LoadUserProxyMap(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	type userConfig struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		Forward  string `yaml:"forward"`
	}
	type config struct {
		Services []struct {
			Users []userConfig `yaml:"users"`
		} `yaml:"services"`
	}
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	UserProxyMap = make(map[string]string)
	for _, svc := range cfg.Services {
		for _, u := range svc.Users {
			UserProxyMap[u.Username+":"+u.Password] = u.Forward
		}
	}
	return nil
}

// ReloadUserProxyMap 线程安全地重新加载 gost-config.yaml 到 UserProxyMap
// 用于注册/配置变更后热加载，无需重启进程
func ReloadUserProxyMap(configPath string) error {
	userProxyMapLock.Lock()
	defer userProxyMapLock.Unlock()
	return LoadUserProxyMap(configPath)
}

// ========== SOCKS5 代理 =============
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

// SOCKS5Server 实现基于用户名密码动态转发的 SOCKS5 代理服务
// 支持多用户独立转发目标
// addr: 监听地址（如 :1080）
type SOCKS5Server struct {
	addr string
}

// NewSOCKS5Server 创建 SOCKS5Server 实例
func NewSOCKS5Server(addr string) *SOCKS5Server {
	return &SOCKS5Server{addr: addr}
}

// Start 启动 SOCKS5 代理服务，阻塞运行
func (s *SOCKS5Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	log.Printf("SOCKS5 代理已启动，监听地址: %s", s.addr)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

// handleConnection 处理单个 SOCKS5 客户端连接
func (s *SOCKS5Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	// 1. 握手与认证
	username, password, err := s.handleHandshake(conn)
	if err != nil {
		log.Printf("Handshake error: %v", err)
		return
	}
	// 2. 用户认证与目标查找
	proxyAddr := s.authenticate(username, password)
	if proxyAddr == "" {
		// 认证失败，回复 0x01
		conn.Write([]byte{0x01, 0x01})
		log.Printf("Authentication failed for user: %s", username)
		return
	}
	// 认证成功，回复 0x00
	conn.Write([]byte{0x01, 0x00})
	log.Printf("User %s authenticated, using proxy: %s", username, proxyAddr)
	// 3. 解析客户端请求目标
	targetAddr, err := s.handleConnect(conn)
	if err != nil {
		log.Printf("Connect handling error: %v", err)
		return
	}
	// 4. 代理转发
	s.proxyConnection(conn, targetAddr, proxyAddr)
}

// handleHandshake 完成 SOCKS5 握手和用户名密码认证，返回用户名和密码
func (s *SOCKS5Server) handleHandshake(conn net.Conn) (string, string, error) {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", "", err
	}
	version, nMethods := buf[0], buf[1]
	if version != SOCKS5Version {
		return "", "", io.ErrUnexpectedEOF
	}
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", "", err
	}
	// 检查客户端是否支持用户名密码认证
	supportUserPass := false
	for _, m := range methods {
		if m == UserPassAuth {
			supportUserPass = true
			break
		}
	}
	if !supportUserPass {
		// 不支持则回复 0xFF，断开
		conn.Write([]byte{SOCKS5Version, 0xFF})
		return "", "", fmt.Errorf("client does not support username/password auth")
	}
	// 回复客户端：需要用户名密码认证
	if _, err := conn.Write([]byte{SOCKS5Version, UserPassAuth}); err != nil {
		return "", "", err
	}
	// 读取认证包
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

// authenticate 校验用户名密码，返回对应的转发目标地址
func (s *SOCKS5Server) authenticate(username, password string) string {
	userKey := username + ":" + password
	userProxyMapLock.RLock()
	defer userProxyMapLock.RUnlock()
	return UserProxyMap[userKey]
}

// handleConnect 解析 SOCKS5 CONNECT 请求，返回目标地址
func (s *SOCKS5Server) handleConnect(conn net.Conn) (string, error) {
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", err
	}
	version, cmd, _, addrType := buf[0], buf[1], buf[2], buf[3]
	if version != SOCKS5Version || cmd != ConnectCmd {
		return "", io.ErrUnexpectedEOF
	}
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
	buf = make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", err
	}
	port := int(buf[0])<<8 + int(buf[1])
	return addr + ":" + fmt.Sprintf("%d", port), nil
}

// proxyConnection 通过 HTTP CONNECT 方式将客户端流量转发到目标代理
func (s *SOCKS5Server) proxyConnection(clientConn net.Conn, targetAddr, proxyAddr string) {
	connector, err := getProxyConnector(proxyAddr)
	if err != nil {
		log.Printf("getProxyConnector error: %v", err)
		clientConn.Write([]byte{SOCKS5Version, 0x01, 0x00, IPv4Addr, 0, 0, 0, 0, 0, 0})
		return
	}
	proxyConn, err := connector(targetAddr)
	if err != nil {
		log.Printf("Failed to connect to proxy %s: %v", proxyAddr, err)
		clientConn.Write([]byte{SOCKS5Version, 0x01, 0x00, IPv4Addr, 0, 0, 0, 0, 0, 0})
		return
	}
	defer proxyConn.Close()
	if _, err := clientConn.Write([]byte{SOCKS5Version, 0x00, 0x00, IPv4Addr, 0, 0, 0, 0, 0, 0}); err != nil {
		log.Printf("Failed to send success response: %v", err)
		return
	}
	s.relay(clientConn, proxyConn)
}

// relay 实现双向数据转发（中继），用于 SOCKS5/HTTP 代理
func (s *SOCKS5Server) relay(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(conn1, conn2)
		conn1.Close()
	}()
	go func() {
		defer wg.Done()
		io.Copy(conn2, conn1)
		conn2.Close()
	}()
	wg.Wait()
}

// ========== HTTP 代理 =============
// HTTPProxyServer 实现基于用户名密码动态转发的 HTTP 代理服务
// 支持多用户独立转发目标
// addr: 监听地址（如 :1089）
type HTTPProxyServer struct {
	addr string
}

// NewHTTPProxyServer 创建 HTTPProxyServer 实例
func NewHTTPProxyServer(addr string) *HTTPProxyServer {
	return &HTTPProxyServer{addr: addr}
}

// Start 启动 HTTP 代理服务，阻塞运行
func (h *HTTPProxyServer) Start() error {
	server := &http.Server{
		Addr:    h.addr,
		Handler: http.HandlerFunc(h.handleRequest),
	}
	log.Printf("HTTP 代理已启动，监听地址: %s", h.addr)
	return server.ListenAndServe()
}

// handleRequest 处理 HTTP 代理请求，仅支持 user:pass@host:port 方式认证
func (h *HTTPProxyServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	var username, password, proxyAddr string
	if r.URL != nil && r.URL.User != nil {
		username = r.URL.User.Username()
		password, _ = r.URL.User.Password()
		userKey := username + ":" + password
		userProxyMapLock.RLock()
		proxyAddr = UserProxyMap[userKey]
		userProxyMapLock.RUnlock()
	}
	if proxyAddr == "" {
		w.Header().Set("Proxy-Authenticate", "Basic realm=\"proxy\"")
		http.Error(w, "Proxy Authentication Required", http.StatusProxyAuthRequired)
		return
	}
	log.Printf("HTTP: User %s authenticated, using proxy: %s", username, proxyAddr)
	if r.Method == "CONNECT" {
		h.handleConnect(w, r, proxyAddr)
	} else {
		h.handleHTTP(w, r, proxyAddr)
	}
}

// handleConnect 处理 HTTP CONNECT 请求，转发到目标代理
func (h *HTTPProxyServer) handleConnect(w http.ResponseWriter, r *http.Request, proxyAddr string) {
	connector, err := getProxyConnector(proxyAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	proxyConn, err := connector(r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer proxyConn.Close()
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	h.relay(clientConn, proxyConn)
}

// handleHTTP 处理普通 HTTP 请求，转发到目标代理
func (h *HTTPProxyServer) handleHTTP(w http.ResponseWriter, r *http.Request, proxyAddr string) {
	connector, err := getProxyConnector(proxyAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	proxyConn, err := connector(r.URL.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer proxyConn.Close()

	// 解析下游代理认证信息
	u, err := url.Parse(proxyAddr)
	var auth string
	if err == nil && u.User != nil {
		auth = "Basic " + base64Encode(u.User.Username()+":"+getPassword(u.User))
	}

	// 构造新的请求，带 Proxy-Authorization
	req, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if auth != "" {
		req.Header.Set("Proxy-Authorization", auth)
	}

	err = req.Write(proxyConn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(proxyConn), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// relay 实现双向数据转发（中继），用于 SOCKS5/HTTP 代理
func (h *HTTPProxyServer) relay(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(conn1, conn2)
		conn1.Close()
	}()
	go func() {
		defer wg.Done()
		io.Copy(conn2, conn1)
		conn2.Close()
	}()
	wg.Wait()
}

// ========== 新增：下游代理连接器工厂 =============
func getProxyConnector(proxyAddr string) (func(targetAddr string) (net.Conn, error), error) {
	u, err := url.Parse(proxyAddr)
	if err != nil || u.Scheme == "" {
		// 兼容老格式（无协议，直接 host:port），默认 http
		return func(targetAddr string) (net.Conn, error) {
			// HTTP CONNECT
			conn, err := net.Dial("tcp", proxyAddr)
			if err != nil {
				return nil, err
			}
			// 发送 CONNECT
			connectReq := "CONNECT " + targetAddr + " HTTP/1.1\r\nHost: " + targetAddr + "\r\n\r\n"
			if _, err := conn.Write([]byte(connectReq)); err != nil {
				conn.Close()
				return nil, err
			}
			reader := bufio.NewReader(conn)
			resp, err := reader.ReadString('\n')
			if err != nil || !strings.Contains(resp, "200") {
				conn.Close()
				return nil, fmt.Errorf("proxy returned non-200: %s", resp)
			}
			// 跳过 HTTP 头
			for {
				line, err := reader.ReadString('\n')
				if err != nil || line == "\r\n" {
					break
				}
			}
			return conn, nil
		}, nil
	}
	switch u.Scheme {
	case "http":
		return func(targetAddr string) (net.Conn, error) {
			// HTTP CONNECT
			conn, err := net.Dial("tcp", u.Host)
			if err != nil {
				return nil, err
			}
			// 认证
			var auth string
			if u.User != nil {
				auth = "Proxy-Authorization: Basic " +
					base64Encode(u.User.Username()+":"+getPassword(u.User)) + "\r\n"
			}
			connectReq := "CONNECT " + targetAddr + " HTTP/1.1\r\nHost: " + targetAddr + "\r\n" + auth + "\r\n"
			if _, err := conn.Write([]byte(connectReq)); err != nil {
				conn.Close()
				return nil, err
			}
			reader := bufio.NewReader(conn)
			resp, err := reader.ReadString('\n')
			if err != nil || !strings.Contains(resp, "200") {
				conn.Close()
				return nil, fmt.Errorf("proxy returned non-200: %s", resp)
			}
			for {
				line, err := reader.ReadString('\n')
				if err != nil || line == "\r\n" {
					break
				}
			}
			return conn, nil
		}, nil
	case "socks5":
		return func(targetAddr string) (net.Conn, error) {
			conn, err := net.Dial("tcp", u.Host)
			if err != nil {
				return nil, err
			}
			// SOCKS5 握手
			var user, pass string
			if u.User != nil {
				user = u.User.Username()
				pass = getPassword(u.User)
			}
			// 1. greeting
			methods := []byte{0x00}
			if user != "" {
				methods = []byte{0x02}
			}
			conn.Write([]byte{0x05, byte(len(methods))})
			conn.Write(methods)
			resp := make([]byte, 2)
			if _, err := io.ReadFull(conn, resp); err != nil {
				conn.Close()
				return nil, err
			}
			if resp[1] == 0x02 {
				// 用户名密码认证
				conn.Write([]byte{0x01, byte(len(user))})
				conn.Write([]byte(user))
				conn.Write([]byte{byte(len(pass))})
				conn.Write([]byte(pass))
				authResp := make([]byte, 2)
				if _, err := io.ReadFull(conn, authResp); err != nil || authResp[1] != 0x00 {
					conn.Close()
					return nil, fmt.Errorf("SOCKS5 auth failed")
				}
			}
			// 2. CONNECT
			host, portStr, _ := net.SplitHostPort(targetAddr)
			port := parsePort(portStr)
			addrType, addrBytes := packAddr(host)
			req := []byte{0x05, 0x01, 0x00, addrType}
			req = append(req, addrBytes...)
			req = append(req, byte(port>>8), byte(port&0xff))
			conn.Write(req)
			reply := make([]byte, 10)
			if _, err := io.ReadFull(conn, reply); err != nil || reply[1] != 0x00 {
				conn.Close()
				return nil, fmt.Errorf("SOCKS5 connect failed")
			}
			return conn, nil
		}, nil
	default:
		return nil, fmt.Errorf("不支持的下游代理协议: %s", u.Scheme)
	}
}

// base64 工具
func base64Encode(s string) string {
	return strings.TrimRight(strings.ReplaceAll(fmt.Sprintf("%q", s), "\\x", ""), "\"")
}
func getPassword(u *url.Userinfo) string {
	p, _ := u.Password()
	return p
}
func parsePort(portStr string) int {
	port, _ := strconv.Atoi(portStr)
	return port
}
func packAddr(host string) (byte, []byte) {
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() != nil {
			return 0x01, ip.To4()
			// IPv4
		} else if ip.To16() != nil {
			return 0x04, ip.To16()
		}
	}
	return 0x03, append([]byte{byte(len(host))}, []byte(host)...)
}
