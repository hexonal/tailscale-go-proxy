package gost

import (
	"net"
	"testing"
	"time"
)

// TestProxyChainParsing 测试代理链解析功能
func TestProxyChainParsing(t *testing.T) {
	tests := []struct {
		name        string
		proxyChain  string
		expectError bool
		expectChain bool
	}{
		{
			name:        "单个HTTP代理",
			proxyChain:  "http://127.0.0.1:8080",
			expectError: false,
			expectChain: false,
		},
		{
			name:        "单个SOCKS5代理",
			proxyChain:  "socks5://user:pass@127.0.0.1:1080",
			expectError: false,
			expectChain: false,
		},
		{
			name:        "简单代理链",
			proxyChain:  "socks5://user:pass@127.0.0.1:1080 -> http://127.0.0.1:8080",
			expectError: false,
			expectChain: true,
		},
		{
			name:        "多层代理链",
			proxyChain:  "socks5://user:pass@proxy1:1080 -> http://proxy2:8080 -> socks5://proxy3:1080",
			expectError: false,
			expectChain: true,
		},
		{
			name:        "不支持的协议",
			proxyChain:  "socks4://127.0.0.1:1080 -> http://127.0.0.1:8080",
			expectError: true,
			expectChain: true,
		},
		{
			name:        "单层代理链（错误格式）",
			proxyChain:  "socks5://127.0.0.1:1080 ->",
			expectError: true,
			expectChain: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector, err := getProxyConnector(tt.proxyChain)

			if tt.expectError {
				if err == nil {
					t.Errorf("期望出现错误，但没有错误")
				}
				return
			}

			if err != nil {
				t.Errorf("不期望出现错误，但出现了错误: %v", err)
				return
			}

			if connector == nil {
				t.Errorf("连接器为空")
			}
		})
	}
}

// TestProxyChainConnection 测试代理链连接功能（需要手动设置代理服务器）
func TestProxyChainConnection(t *testing.T) {
	t.Skip("集成测试，需要手动设置代理服务器")

	// 设置用户代理映射，模拟真实使用场景
	userProxyMapLock.Lock()
	UserProxyMap["user1:pass1"] = "socks5://user2:pass2@127.0.0.1:1081 -> http://127.0.0.1:8080"
	UserProxyMap["user2:pass2"] = "http://127.0.0.1:8080"
	userProxyMapLock.Unlock()

	// 启动 HTTP 代理服务器
	httpProxy := NewHTTPProxyServer(":8081")
	go httpProxy.Start()
	time.Sleep(100 * time.Millisecond)

	// 启动 SOCKS5 代理服务器
	socksProxy := NewSOCKS5Server(":1081")
	go socksProxy.Start()
	time.Sleep(100 * time.Millisecond)

	// 测试通过代理链连接
	t.Run("HTTP代理链连接", func(t *testing.T) {
		// 连接到 HTTP 代理
		conn, err := net.Dial("tcp", "127.0.0.1:8081")
		if err != nil {
			t.Fatalf("连接HTTP代理失败: %v", err)
		}
		defer conn.Close()

		// 发送带认证的 CONNECT 请求
		request := "CONNECT httpbin.org:80 HTTP/1.1\r\n"
		request += "Host: httpbin.org:80\r\n"
		request += "Proxy-Authorization: Basic dXNlcjE6cGFzczE=\r\n" // user1:pass1 的 base64 编码
		request += "\r\n"

		_, err = conn.Write([]byte(request))
		if err != nil {
			t.Fatalf("发送CONNECT请求失败: %v", err)
		}

		// 读取响应
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("读取响应失败: %v", err)
		}

		response := string(buf[:n])
		t.Logf("代理响应: %s", response)

		// 检查是否连接成功
		if len(response) > 0 {
			t.Logf("代理链连接测试完成")
		}
	})
}

// TestRemoveAuthFromProxy 测试从代理URL中移除认证信息的功能
func TestRemoveAuthFromProxy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "HTTP代理带认证",
			input:    "http://user:pass@proxy.example.com:8080",
			expected: "http://proxy.example.com:8080",
		},
		{
			name:     "SOCKS5代理带认证",
			input:    "socks5://admin:secret@proxy.example.com:1080",
			expected: "socks5://proxy.example.com:1080",
		},
		{
			name:     "代理无认证信息",
			input:    "http://proxy.example.com:8080",
			expected: "http://proxy.example.com:8080",
		},
		{
			name:     "HTTPS代理带认证",
			input:    "https://user:pass@secure-proxy.example.com:443",
			expected: "https://secure-proxy.example.com:443",
		},
		{
			name:     "只有用户名无密码",
			input:    "http://user@proxy.example.com:8080",
			expected: "http://proxy.example.com:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeAuthFromProxy(tt.input)
			if result != tt.expected {
				t.Errorf("removeAuthFromProxy() = %s, 期望 %s", result, tt.expected)
			}
		})
	}
}

// 演示如何在实际应用中使用代理链
func ExampleProxyChain() {
	// 配置用户代理映射
	userProxyMapLock.Lock()
	UserProxyMap["admin:secret"] = "socks5://upstream:password@proxy1.example.com:1080 -> http://proxy2.example.com:8080 -> socks5://proxy3.example.com:1080"
	userProxyMapLock.Unlock()

	// 启动 HTTP 代理服务器
	httpProxy := NewHTTPProxyServer(":8080")
	go httpProxy.Start()

	// 启动 SOCKS5 代理服务器
	socksProxy := NewSOCKS5Server(":1080")
	go socksProxy.Start()

	// 客户端现在可以连接到本地代理，请求将通过代理链转发
	// HTTP 客户端示例：设置代理为 http://admin:secret@localhost:8080
	// SOCKS5 客户端示例：设置代理为 socks5://admin:secret@localhost:1080
}
