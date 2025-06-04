package gost

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// getProxyConnector 根据下游代理地址 proxyAddr 返回一个连接器函数。
// 该连接器函数可用于通过指定的下游代理（支持 http、https、socks5 协议）建立到目标地址 targetAddr 的 TCP 连接。
//
// 参数：
//
//	proxyAddr: 下游代理地址，支持以下格式：
//	  1. 单个代理：http(s)://user:pass@host:port 或 socks5://user:pass@host:port
//	  2. 代理链：socks5://user:pass@proxy1:1080 -> http://proxy2:8080 -> socks5://proxy3:1080
//
// 返回值：
//   - func(targetAddr string) (net.Conn, error)：用于连接目标地址的连接器函数
//   - error：如解析 proxyAddr 失败或协议不支持则返回错误
//
// 支持的协议：
//   - http/https: 通过 HTTP CONNECT 建立隧道，支持 Basic 认证
//   - socks5: 通过 SOCKS5 协议建立连接，支持用户名密码认证
//
// 代理链特性：
//   - 仅第一层代理需要认证，后续层级自动跳过认证
//   - 支持混合协议的代理链（如 socks5 -> http -> socks5）
//
// 若 proxyAddr 协议不被支持，则返回错误。
func getProxyConnector(proxyAddr string) (func(targetAddr string) (net.Conn, error), error) {
	// 检查是否为代理链格式（包含 "->" 分隔符）
	if strings.Contains(proxyAddr, "->") {
		return getProxyChainConnector(proxyAddr)
	}

	// 单个代理的原有逻辑
	return getSingleProxyConnector(proxyAddr)
}

// getProxyChainConnector 处理代理链连接器。
// 解析代理链格式，依次建立多层代理连接。
// 仅在第一层代理进行认证，后续层级跳过认证。
func getProxyChainConnector(proxyChain string) (func(targetAddr string) (net.Conn, error), error) {
	// 解析代理链，分割各层代理
	proxies := strings.Split(proxyChain, "->")
	var validProxies []string
	for i := range proxies {
		proxy := strings.TrimSpace(proxies[i])
		if proxy != "" {
			validProxies = append(validProxies, proxy)
		}
	}

	if len(validProxies) < 2 {
		return nil, fmt.Errorf("代理链格式错误：至少需要 2 个有效代理")
	}

	proxies = validProxies

	// 验证所有代理地址格式的合法性
	for i, proxy := range proxies {
		u, err := url.Parse(proxy)
		if err != nil || u.Scheme == "" {
			u = &url.URL{Scheme: "http", Host: proxy}
		}
		if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" {
			return nil, fmt.Errorf("代理链第 %d 层协议不支持: %s", i+1, u.Scheme)
		}
	}

	return func(targetAddr string) (net.Conn, error) {
		// 从第一层代理开始建立连接
		firstProxy := proxies[0]
		conn, err := connectToFirstProxy(firstProxy)
		if err != nil {
			return nil, fmt.Errorf("连接第一层代理失败: %v", err)
		}

		// 依次通过每层代理建立到下一层的连接
		for i := 1; i < len(proxies); i++ {
			nextProxy := removeAuthFromProxy(proxies[i]) // 移除后续代理的认证信息
			conn, err = connectThroughProxy(conn, nextProxy, firstProxy, false)
			if err != nil {
				return nil, fmt.Errorf("通过第 %d 层代理连接失败: %v", i, err)
			}
		}

		// 最后建立到目标地址的连接
		conn, err = connectThroughProxy(conn, targetAddr, firstProxy, false)
		if err != nil {
			return nil, fmt.Errorf("连接目标地址失败: %v", err)
		}

		return conn, nil
	}, nil
}

// connectToFirstProxy 连接到第一层代理，包含认证逻辑。
func connectToFirstProxy(proxyAddr string) (net.Conn, error) {
	u, err := url.Parse(proxyAddr)
	if err != nil || u.Scheme == "" {
		u = &url.URL{Scheme: "http", Host: proxyAddr}
	}

	// 连接到第一层代理服务器
	conn, err := net.DialTimeout("tcp", u.Host, 10*time.Second)
	if err != nil {
		return nil, err
	}

	// 如果是 SOCKS5 协议，需要进行认证协商
	if u.Scheme == "socks5" {
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
	}

	return conn, nil
}

// removeAuthFromProxy 从代理 URL 中移除认证信息。
// 用于后续代理链转发时移除认证，只保留第一层代理的认证。
func removeAuthFromProxy(proxyAddr string) string {
	u, err := url.Parse(proxyAddr)
	if err != nil {
		return proxyAddr
	}
	// 移除用户认证信息
	u.User = nil
	return u.String()
}

// connectThroughProxy 通过当前连接（已连接到代理）建立到目标地址的连接。
// isFirstHop 表示是否是第一次跳转（从第一层代理到第二层代理），用于特殊处理。
func connectThroughProxy(conn net.Conn, targetAddr string, firstProxyAddr string, isFirstHop bool) (net.Conn, error) {
	// 解析第一层代理信息以确定协议
	firstU, err := url.Parse(firstProxyAddr)
	if err != nil || firstU.Scheme == "" {
		firstU = &url.URL{Scheme: "http", Host: firstProxyAddr}
	}

	switch firstU.Scheme {
	case "http", "https":
		// 通过 HTTP 代理发送 CONNECT 请求
		req := &http.Request{
			Method:     http.MethodConnect,
			URL:        &url.URL{Host: targetAddr},
			Host:       targetAddr,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
		}
		// 后续代理链转发时不添加认证头，因为内部代理不需要认证
		if err := req.Write(conn); err != nil {
			conn.Close()
			return nil, err
		}
		resp, err := http.ReadResponse(bufio.NewReader(conn), req)
		if err != nil {
			conn.Close()
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			conn.Close()
			return nil, fmt.Errorf("HTTP CONNECT failed: %s", resp.Status)
		}
		return conn, nil

	case "socks5":
		// 通过 SOCKS5 代理发送 CONNECT 请求
		host, portStr, err := net.SplitHostPort(targetAddr)
		if err != nil {
			// 如果没有端口，默认使用 80 端口
			host = targetAddr
			portStr = "80"
		}
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
		return conn, nil

	default:
		conn.Close()
		return nil, fmt.Errorf("不支持的代理协议: %s", firstU.Scheme)
	}
}

// getSingleProxyConnector 处理单个代理连接器（原有逻辑）。
func getSingleProxyConnector(proxyAddr string) (func(targetAddr string) (net.Conn, error), error) {
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
			// 3. 添加认证信息（如果有）
			if u.User != nil {
				username := u.User.Username()
				password, _ := u.User.Password()
				if username != "" {
					auth := username + ":" + password
					encoded := base64EncodeString(auth)
					req.Header.Set("Proxy-Authorization", "Basic "+encoded)
				}
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
			host, portStr, err := net.SplitHostPort(targetAddr)
			if err != nil {
				// 如果没有端口，默认使用 80 端口
				host = targetAddr
				portStr = "80"
			}
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

// base64EncodeString 简单的 base64 编码函数，避免导入 encoding/base64 包。
func base64EncodeString(src string) string {
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

	data := []byte(src)
	result := make([]byte, ((len(data)+2)/3)*4)

	for i := 0; i < len(data); i += 3 {
		// 取出 3 个字节
		var chunk uint32
		switch len(data) - i {
		case 1:
			chunk = uint32(data[i]) << 16
		case 2:
			chunk = uint32(data[i])<<16 | uint32(data[i+1])<<8
		default:
			chunk = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
		}

		// 编码为 4 个字符
		j := (i / 3) * 4
		result[j] = base64Chars[(chunk>>18)&63]
		result[j+1] = base64Chars[(chunk>>12)&63]
		if i+1 < len(data) {
			result[j+2] = base64Chars[(chunk>>6)&63]
		} else {
			result[j+2] = '='
		}
		if i+2 < len(data) {
			result[j+3] = base64Chars[chunk&63]
		} else {
			result[j+3] = '='
		}
	}

	return string(result)
}
