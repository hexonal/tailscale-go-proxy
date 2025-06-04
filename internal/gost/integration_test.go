package gost

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

// TestProxyChainIntegration 完整测试链式代理集成功能
func TestProxyChainIntegration(t *testing.T) {
	t.Skip("集成测试 - 需要外部代理服务器配合")

	// 1. 配置用户代理映射 - 模拟链式代理配置
	userProxyMapLock.Lock()
	UserProxyMap["testuser:testpass"] = "socks5://user:pass@proxy1.example.com:1080 -> http://proxy2.example.com:8080"
	UserProxyMap["httpuser:httppass"] = "http://first-proxy.example.com:8080 -> socks5://second-proxy.example.com:1080"
	userProxyMapLock.Unlock()

	// 2. 启动我们的代理服务器
	httpProxy := NewHTTPProxyServer(":18080")
	socksProxy := NewSOCKS5Server(":11080")

	go httpProxy.Start()
	go socksProxy.Start()
	time.Sleep(100 * time.Millisecond)

	// 3. 测试 HTTP 代理链
	t.Run("HTTP代理链测试", func(t *testing.T) {
		proxyURL, _ := url.Parse("http://testuser:testpass@localhost:18080")
		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
			Timeout: 10 * time.Second,
		}

		// 这个请求会经过：客户端 -> 我们的HTTP代理 -> proxy1 -> proxy2 -> 目标
		resp, err := client.Get("https://httpbin.org/ip")
		if err != nil {
			t.Logf("HTTP代理链请求(预期可能失败，因为测试代理不存在): %v", err)
		} else {
			defer resp.Body.Close()
			t.Logf("HTTP代理链请求成功，状态码: %d", resp.StatusCode)
		}
	})

	// 4. 测试 SOCKS5 代理链
	t.Run("SOCKS5代理链测试", func(t *testing.T) {
		dialer, err := proxy.SOCKS5("tcp", "localhost:11080",
			&proxy.Auth{
				User:     "httpuser",
				Password: "httppass",
			},
			proxy.Direct,
		)
		if err != nil {
			t.Fatalf("创建SOCKS5 dialer失败: %v", err)
		}

		// 这个连接会经过：客户端 -> 我们的SOCKS5代理 -> first-proxy -> second-proxy -> 目标
		conn, err := dialer.Dial("tcp", "httpbin.org:80")
		if err != nil {
			t.Logf("SOCKS5代理链连接(预期可能失败，因为测试代理不存在): %v", err)
		} else {
			defer conn.Close()
			t.Logf("SOCKS5代理链连接成功")
		}
	})
}

// TestProxyChainWorkflowVerification 验证代理链工作流程
func TestProxyChainWorkflowVerification(t *testing.T) {
	// 验证关键函数的工作流程

	// 1. 测试代理链解析
	t.Run("代理链解析验证", func(t *testing.T) {
		proxyChain := "socks5://user:pass@proxy1:1080 -> http://proxy2:8080 -> socks5://proxy3:1080"
		connector, err := getProxyConnector(proxyChain)
		if err != nil {
			t.Fatalf("代理链解析失败: %v", err)
		}
		if connector == nil {
			t.Fatal("连接器为空")
		}
		t.Logf("✅ 代理链解析成功")
	})

	// 2. 测试认证信息移除
	t.Run("认证信息移除验证", func(t *testing.T) {
		authProxy := "http://user:pass@proxy.example.com:8080"
		cleanProxy := removeAuthFromProxy(authProxy)
		expected := "http://proxy.example.com:8080"

		if cleanProxy != expected {
			t.Errorf("认证移除失败: 得到 %s, 期望 %s", cleanProxy, expected)
		}
		t.Logf("✅ 认证信息成功移除: %s -> %s", authProxy, cleanProxy)
	})

	// 3. 测试单个代理兼容性
	t.Run("单个代理兼容性验证", func(t *testing.T) {
		singleProxy := "http://proxy.example.com:8080"
		connector, err := getProxyConnector(singleProxy)
		if err != nil {
			t.Fatalf("单个代理解析失败: %v", err)
		}
		if connector == nil {
			t.Fatal("连接器为空")
		}
		t.Logf("✅ 单个代理兼容性验证成功")
	})

	// 4. 验证错误处理
	t.Run("错误处理验证", func(t *testing.T) {
		invalidChain := "socks5://proxy1:1080 ->"
		_, err := getProxyConnector(invalidChain)
		if err == nil {
			t.Error("期望解析错误但没有返回错误")
		} else {
			t.Logf("✅ 错误处理正常: %v", err)
		}
	})
}

// TestRealWorldUsageExample 展示真实使用场景的示例
func TestRealWorldUsageExample(t *testing.T) {
	// 模拟真实使用场景的配置

	// 清理之前的配置
	userProxyMapLock.Lock()
	UserProxyMap = make(map[string]string)
	userProxyMapLock.Unlock()

	// 配置不同用户的代理链
	exampleConfigs := map[string]string{
		// VIP用户 - 3层代理链
		"vip:secret123": "socks5://first-level:1080 -> http://second-level:8080 -> socks5://third-level:1080",

		// 普通用户 - 2层代理链
		"user:password": "http://level1:8080 -> socks5://level2:1080",

		// 简单用户 - 单个代理（兼容旧配置）
		"simple:123456": "http://simple-proxy:8080",

		// 企业用户 - 混合协议代理链
		"enterprise:complex": "socks5://corp-proxy:1080 -> http://external-proxy:8080",
	}

	userProxyMapLock.Lock()
	for user, proxyChain := range exampleConfigs {
		UserProxyMap[user] = proxyChain
	}
	userProxyMapLock.Unlock()

	// 验证每个配置都能正确解析
	for user, proxyChain := range exampleConfigs {
		t.Run(fmt.Sprintf("用户配置验证-%s", strings.Split(user, ":")[0]), func(t *testing.T) {
			connector, err := getProxyConnector(proxyChain)
			if err != nil {
				t.Errorf("用户 %s 的代理配置解析失败: %v", user, err)
			} else {
				t.Logf("✅ 用户 %s 的代理配置解析成功", user)
			}
			if connector == nil {
				t.Errorf("用户 %s 的连接器为空", user)
			}
		})
	}

	t.Logf("✅ 所有用户配置验证完成")
}

// TestProxyChainCompatibilityWithExistingCode 测试与现有代码的兼容性
func TestProxyChainCompatibilityWithExistingCode(t *testing.T) {
	// 模拟现有代码中的调用方式

	t.Run("Post.go兼容性", func(t *testing.T) {
		// 模拟 Post.go 中可能的代理使用方式
		proxyAddr := "socks5://proxy1:1080 -> http://proxy2:8080"

		// 这模拟了 Socks5Client 函数中的代理处理
		if strings.Contains(proxyAddr, "->") {
			t.Logf("✅ 检测到代理链格式，将使用链式代理功能")
			connector, err := getProxyConnector(proxyAddr)
			if err != nil {
				t.Errorf("代理链处理失败: %v", err)
			} else if connector != nil {
				t.Logf("✅ Post.go 代理链兼容性验证成功")
			}
		}
	})

	t.Run("TcpClient.go兼容性", func(t *testing.T) {
		// 模拟 TcpClient.go 中 CreateConnection 的调用
		remoteAddr := "target.example.com:80"
		proxyAddr := "socks5://first:1080 -> http://second:8080"
		proxyUser := "user"
		proxyPassword := "pass"

		t.Logf("目标地址: %s", remoteAddr)
		t.Logf("代理配置: %s", proxyAddr)
		t.Logf("认证信息: %s:%s", proxyUser, proxyPassword)

		// 验证代理地址格式是否支持
		if strings.Contains(proxyAddr, "->") {
			connector, err := getProxyConnector(proxyAddr)
			if err != nil {
				t.Errorf("TcpClient 代理链处理失败: %v", err)
			} else if connector != nil {
				t.Logf("✅ TcpClient.go 代理链兼容性验证成功")
			}
		}
	})

	t.Run("WXSliderOCR.go兼容性", func(t *testing.T) {
		// 模拟 WeChatSMS 和 WeChatQrCode 函数中的代理使用
		proxyAddr := "http://layer1:8080 -> socks5://layer2:1080"
		proxyUser := "wxuser"
		proxyPass := "wxpass"

		t.Logf("微信模块代理配置: %s", proxyAddr)
		t.Logf("微信模块认证: %s:%s", proxyUser, proxyPass)

		// 验证代理配置兼容性
		connector, err := getProxyConnector(proxyAddr)
		if err != nil {
			t.Errorf("微信模块代理链处理失败: %v", err)
		} else if connector != nil {
			t.Logf("✅ WXSliderOCR.go 代理链兼容性验证成功")
		}
	})
}
