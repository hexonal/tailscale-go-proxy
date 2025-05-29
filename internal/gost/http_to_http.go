package gost

import (
	"bufio"
	"encoding/base64"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
)

// HTTPProxyServer 实现了基于用户名密码动态转发的 HTTP/HTTPS 代理服务。
// 支持标准 HTTP 代理协议，支持 CONNECT 隧道和普通 HTTP 请求，
// 可根据用户认证信息动态选择下游代理。
type HTTPProxyServer struct {
	addr string
}

// NewHTTPProxyServer 创建一个新的 HTTPProxyServer 实例。
// 参数 addr 为监听地址（如 ":8081"），返回 HTTPProxyServer 指针。
func NewHTTPProxyServer(addr string) *HTTPProxyServer {
	return &HTTPProxyServer{addr: addr}
}

// Start 启动 HTTP 代理服务器，监听指定地址并处理客户端请求。
// 返回 error 表示启动或运行过程中遇到的错误。
func (h *HTTPProxyServer) Start() error {
	// 创建 HTTP 服务器，指定自定义 Handler
	server := &http.Server{
		Addr:    h.addr,
		Handler: http.HandlerFunc(h.handleRequest),
	}
	log.Printf("HTTP 代理已启动，监听地址: %s", h.addr)
	// 启动监听主循环
	return server.ListenAndServe()
}

// handleRequest 处理所有进入的 HTTP 代理请求。
// 根据请求中的认证信息选择下游代理，支持 Basic 认证。
// 若认证失败则返回 407，认证成功则根据请求类型分发到 handleConnect 或 handleHTTP。
// 参数 w 为响应写入器，r 为客户端请求。
func (h *HTTPProxyServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	var username, password, proxyAddr string
	// 1. 解析认证信息（Basic Auth）
	if r.URL != nil && r.URL.User != nil {
		username = r.URL.User.Username()
		password, _ = r.URL.User.Password()
		userKey := username + ":" + password
		userProxyMapLock.RLock()
		proxyAddr = UserProxyMap[userKey]
		userProxyMapLock.RUnlock()
	}
	// 2. 支持匿名转发（无认证时，允许转发到未设置认证的下游 http 代理）
	if proxyAddr == "" {
		userProxyMapLock.RLock()
		for _, v := range UserProxyMap {
			u, err := url.Parse(v)
			if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil {
				proxyAddr = v
				log.Printf("HTTP: 匿名用户允许转发到下游 http 代理: %s", proxyAddr)
				break
			}
		}
		userProxyMapLock.RUnlock()
	}
	// 3. 认证失败，返回 407
	if proxyAddr == "" {
		w.Header().Set("Proxy-Authenticate", "Basic realm=\"proxy\"")
		http.Error(w, "Proxy Authentication Required", http.StatusProxyAuthRequired)
		return
	}
	log.Printf("HTTP: User %s authenticated, using proxy: %s", username, proxyAddr)
	// 4. 根据请求类型分发
	if r.Method == "CONNECT" {
		h.handleConnect(w, r, proxyAddr)
	} else {
		h.handleHTTP(w, r, proxyAddr)
	}
}

// handleConnect 处理 HTTP CONNECT 隧道请求。
// 通过下游代理建立到目标主机的隧道，并将客户端与下游代理连接起来。
// 参数 w 为响应写入器，r 为客户端请求，proxyAddr 为下游代理地址。
func (h *HTTPProxyServer) handleConnect(w http.ResponseWriter, r *http.Request, proxyAddr string) {
	// 1. 获取下游代理连接器
	connector, err := getProxyConnector(proxyAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 2. 通过下游代理建立到目标主机的连接
	proxyConn, err := connector(r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer proxyConn.Close()
	// 3. 劫持客户端连接，升级为 TCP 隧道
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
	// 4. 通知客户端隧道建立成功
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	// 5. 开始双向转发数据
	h.relay(clientConn, proxyConn)
}

// handleHTTP 处理普通 HTTP 请求（非 CONNECT）。
// 通过下游代理转发 HTTP 请求并将响应返回给客户端。
// 参数 w 为响应写入器，r 为客户端请求，proxyAddr 为下游代理地址。
func (h *HTTPProxyServer) handleHTTP(w http.ResponseWriter, r *http.Request, proxyAddr string) {
	// 1. 解析下游代理认证信息
	u, _ := url.Parse(proxyAddr)
	var auth string
	if u != nil && u.User != nil && u.User.Username() != "" {
		user := u.User.Username()
		pass, _ := u.User.Password()
		auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	}
	// 2. 获取下游代理连接器
	connector, err := getProxyConnector(proxyAddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 3. 通过下游代理建立到目标主机的连接
	proxyConn, err := connector(r.URL.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 4. 劫持客户端连接，转为原始 TCP
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 5. 循环转发 HTTP 请求与响应
	req := r
	for {
		// 构造新的 HTTP 请求，复制原始请求头
		newReq, err := http.NewRequest(req.Method, req.URL.String(), req.Body)
		if err != nil {
			break
		}
		for key, values := range req.Header {
			for _, value := range values {
				newReq.Header.Add(key, value)
			}
		}
		// 设置下游代理认证头
		if auth != "" {
			// newReq.Header.Set("Proxy-Authorization", auth)
		} else {
			newReq.Header.Del("Proxy-Authorization")
		}
		newReq.Header.Del("Proxy-Authorization")
		// 发送请求到下游代理
		err = newReq.WriteProxy(proxyConn)
		if err != nil {
			break
		}
		// 读取下游代理响应
		resp, err := http.ReadResponse(bufio.NewReader(proxyConn), newReq)
		if err != nil {
			break
		}
		// 转发响应给客户端
		resp.Write(clientConn)
		// 检查连接是否应关闭
		if resp.Close || newReq.Close {
			break
		}
		// 读取下一个客户端请求（长连接复用）
		req, err = http.ReadRequest(bufio.NewReader(clientConn))
		if err != nil {
			break
		}
	}
	proxyConn.Close()
	clientConn.Close()
}

// relay 实现两个连接之间的双向数据转发。
// 用于 CONNECT 隧道和 SOCKS5 隧道的数据转发。
// 参数 conn1、conn2 为需要互相转发数据的两个连接。
func (h *HTTPProxyServer) relay(conn1, conn2 net.Conn) {
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
