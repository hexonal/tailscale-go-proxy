package gost

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// getProxyConnector 根据下游代理地址 proxyAddr 返回一个连接器函数。
// 该连接器函数可用于通过指定的下游代理（支持 http、https、socks5 协议）建立到目标地址 targetAddr 的 TCP 连接。
//
// 参数：
//
//	proxyAddr: 下游代理地址，格式支持 http(s)://user:pass@host:port 或 socks5://user:pass@host:port
//
// 返回值：
//   - func(targetAddr string) (net.Conn, error)：用于连接目标地址的连接器函数
//   - error：如解析 proxyAddr 失败或协议不支持则返回错误
//
// 支持的协议：
//   - http/https: 通过 HTTP CONNECT 建立隧道，支持 Basic 认证
//   - socks5: 通过 SOCKS5 协议建立连接，支持用户名密码认证
//
// 若 proxyAddr 协议不被支持，则返回错误。
func getProxyConnector(proxyAddr string) (func(targetAddr string) (net.Conn, error), error) {
	// 解析下游代理地址，若无协议默认 http
	u, err := url.Parse(proxyAddr)
	if err != nil || u.Scheme == "" {
		u = &url.URL{Scheme: "http", Host: proxyAddr}
	}
	switch u.Scheme {
	case "http", "https":
		// 返回 HTTP/HTTPS 代理连接器
		return func(targetAddr string) (net.Conn, error) {
			// 1. 连接下游代理服务器
			conn, err := net.DialTimeout("tcp", u.Host, 10*time.Second)
			if err != nil {
				return nil, err
			}
			// 2. 构造 HTTP CONNECT 请求
			req := &http.Request{
				Method:     http.MethodConnect,
				URL:        &url.URL{Host: targetAddr},
				Host:       targetAddr,
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
			}
			// 4. 发送 CONNECT 请求
			if err := req.Write(conn); err != nil {
				conn.Close()
				return nil, err
			}
			// 5. 读取代理响应，判断是否建立成功
			resp, err := http.ReadResponse(bufio.NewReader(conn), req)
			if err != nil {
				conn.Close()
				return nil, err
			}
			if resp.StatusCode != http.StatusOK {
				conn.Close()
				return nil, fmt.Errorf("proxy returned %s", resp.Status)
			}
			// 6. 返回已建立的隧道连接
			return conn, nil
		}, nil
	case "socks5":
		// 返回 SOCKS5 代理连接器
		return func(targetAddr string) (net.Conn, error) {
			// 1. 连接下游 SOCKS5 代理
			conn, err := net.DialTimeout("tcp", u.Host, 10*time.Second)
			if err != nil {
				return nil, err
			}
			// 2. 认证协商
			var user, pass string
			if u.User != nil {
				user = u.User.Username()
				pass, _ = u.User.Password()
			}
			methods := []byte{0x00}
			if user != "" {
				methods = []byte{0x02}
			}
			// 发送 VER/NMETHODS/METHODS
			conn.Write([]byte{0x05, byte(len(methods))})
			conn.Write(methods)
			resp := make([]byte, 2)
			if _, err := io.ReadFull(conn, resp); err != nil {
				conn.Close()
				return nil, err
			}
			if resp[1] == 0x02 {
				// 需要用户名密码认证
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
			// 3. 发送 CONNECT 请求
			host, portStr, _ := net.SplitHostPort(targetAddr)
			port, _ := strconv.Atoi(portStr)
			var addrType byte
			var addrBytes []byte
			if ip := net.ParseIP(host); ip != nil {
				if ip.To4() != nil {
					addrType = 0x01
					addrBytes = ip.To4()
				} else if ip.To16() != nil {
					addrType = 0x04
					addrBytes = ip.To16()
				}
			} else {
				addrType = 0x03
				addrBytes = append([]byte{byte(len(host))}, []byte(host)...)
			}
			req := []byte{0x05, 0x01, 0x00, addrType}
			req = append(req, addrBytes...)
			req = append(req, byte(port>>8), byte(port&0xff))
			conn.Write(req)
			reply := make([]byte, 10)
			if _, err := io.ReadFull(conn, reply); err != nil || reply[1] != 0x00 {
				conn.Close()
				return nil, fmt.Errorf("SOCKS5 connect failed")
			}
			// 4. 返回已建立的隧道连接
			return conn, nil
		}, nil
	default:
		// 不支持的协议，返回错误
		return nil, fmt.Errorf("不支持的下游代理协议: %s", u.Scheme)
	}
}
