package gost

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadUserProxyMap(t *testing.T) {
	// 构造一个临时配置文件
	configContent := `
services:
  - users:
      - username: testuser
        password: testpass
        forward: 127.0.0.1:9999
  - users:
      - username: foo
        password: bar
        forward: 10.0.0.1:8080
`
	file := "test-gost-config.yaml"
	if err := os.WriteFile(file, []byte(configContent), 0644); err != nil {
		t.Fatalf("写入测试配置文件失败: %v", err)
	}
	defer os.Remove(file)

	if err := LoadUserProxyMap(file); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if got := UserProxyMap["testuser:testpass"]; got != "127.0.0.1:9999" {
		t.Errorf("UserProxyMap 加载错误，期望 127.0.0.1:9999，实际 %s", got)
	}
	if got := UserProxyMap["foo:bar"]; got != "10.0.0.1:8080" {
		t.Errorf("UserProxyMap 加载错误，期望 10.0.0.1:8080，实际 %s", got)
	}
}

func TestReloadUserProxyMap(t *testing.T) {
	configContent := `
services:
  - users:
      - username: reload
        password: reload
        forward: 1.2.3.4:1234
`
	file := "test-gost-config.yaml"
	if err := os.WriteFile(file, []byte(configContent), 0644); err != nil {
		t.Fatalf("写入测试配置文件失败: %v", err)
	}
	defer os.Remove(file)

	if err := ReloadUserProxyMap(file); err != nil {
		t.Fatalf("ReloadUserProxyMap 失败: %v", err)
	}
	userProxyMapLock.RLock()
	defer userProxyMapLock.RUnlock()
	if got := UserProxyMap["reload:reload"]; got != "1.2.3.4:1234" {
		t.Errorf("ReloadUserProxyMap 加载错误，期望 1.2.3.4:1234，实际 %s", got)
	}
}

func TestProxyChain_SOCKS5_HTTP(t *testing.T) {
	// t.Skip("集成测试，需本地 1080 端口可用，且能访问外网 baidu.com")
	// 1. 启动本地 HTTP 代理（监听 1080）
	httpProxy := NewHTTPProxyServer(":1080")
	go httpProxy.Start()
	t.Cleanup(func() { /* 这里无法优雅关闭 http.Server，实际部署用 context 控制 */ })
	time.Sleep(500 * time.Millisecond)

	// 2. 配置 UserProxyMap，SOCKS5 用户指向 http://127.0.0.1:1080
	userProxyMapLock.Lock()
	UserProxyMap["socksuser:sockspass"] = "http://127.0.0.1:1080"
	userProxyMapLock.Unlock()

	// 3. 启动 SOCKS5 代理（监听 1111）
	socks5Proxy := NewSOCKS5Server(":1111")
	go socks5Proxy.Start()
	t.Cleanup(func() {})
	time.Sleep(500 * time.Millisecond)

	// 4. 用 go-socks5-client 通过 SOCKS5 代理访问 baidu.com
	proxyDialer, err := net.Dial("tcp", "127.0.0.1:1111")
	if err != nil {
		t.Fatalf("无法连接本地SOCKS5代理: %v", err)
	}
	defer proxyDialer.Close()
	// 手动实现SOCKS5握手
	proxyDialer.Write([]byte{0x05, 0x01, 0x02}) // VER, NMETHODS, USERPASS
	resp := make([]byte, 2)
	if _, err := proxyDialer.Read(resp); err != nil || resp[1] != 0x02 {
		t.Fatalf("SOCKS5握手失败: %v, resp=%v", err, resp)
	}
	// 用户名密码认证
	user := "socksuser"
	pass := "sockspass"
	proxyDialer.Write([]byte{0x01, byte(len(user))})
	proxyDialer.Write([]byte(user))
	proxyDialer.Write([]byte{byte(len(pass))})
	proxyDialer.Write([]byte(pass))
	if _, err := proxyDialer.Read(resp); err != nil || resp[1] != 0x00 {
		t.Fatalf("SOCKS5认证失败: %v, resp=%v", err, resp)
	}
	// CONNECT baidu.com:80
	host := "baidu.com"
	port := 80
	addr := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	addr = append(addr, []byte(host)...)
	addr = append(addr, byte(port>>8), byte(port&0xff))
	proxyDialer.Write(addr)
	reply := make([]byte, 10)
	if _, err := proxyDialer.Read(reply); err != nil || reply[1] != 0x00 {
		t.Fatalf("SOCKS5 CONNECT失败: %v, reply=%v", err, reply)
	}
	// 通过代理发起 HTTP 请求
	fmt.Fprintf(proxyDialer, "GET / HTTP/1.1\r\nHost: baidu.com\r\nConnection: close\r\n\r\n")
	buf := make([]byte, 4096)
	n, _ := proxyDialer.Read(buf)
	if !strings.Contains(string(buf[:n]), "baidu") {
		t.Errorf("SOCKS5->HTTP链路未获取到百度首页")
	} else {
		fmt.Println("SOCKS5->HTTP链路返回内容前512字：\n" + string(buf[:min(n, 512)]))
	}
}

func TestProxyChain_HTTP_HTTP(t *testing.T) {
	// t.Skip("集成测试，需本地 1080 端口可用，且能访问外网 baidu.com")
	// 1. 启动本地 HTTP 代理（监听 1080）
	httpProxy := NewHTTPProxyServer(":1080")
	go httpProxy.Start()
	t.Cleanup(func() {})
	time.Sleep(500 * time.Millisecond)

	// 2. 配置 UserProxyMap，HTTP 用户指向 http://127.0.0.1:1080
	userProxyMapLock.Lock()
	UserProxyMap["httpuser:httppass"] = "http://127.0.0.1:1080"
	userProxyMapLock.Unlock()

	// 3. 启动 HTTP 代理（监听 1112）
	httpProxy2 := NewHTTPProxyServer(":1112")
	go httpProxy2.Start()
	t.Cleanup(func() {})
	time.Sleep(500 * time.Millisecond)

	// 4. 用 http.Client 通过 HTTP 代理访问 baidu.com
	proxyURL, _ := url.Parse("http://httpuser:httppass@127.0.0.1:1112")
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 10 * time.Second,
	}
	resp, err := client.Get("http://baidu.com/")
	if err != nil {
		t.Fatalf("HTTP->HTTP链路请求失败: %v", err)
	}
	defer resp.Body.Close()
	fmt.Printf("HTTP->HTTP resp.StatusCode=%d\n", resp.StatusCode)
	fmt.Printf("HTTP->HTTP resp.Header=%v\n", resp.Header)
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	if !strings.Contains(string(body[:n]), "<html") {
		t.Errorf("HTTP->HTTP链路未获取到HTML内容")
	} else {
		fmt.Println("HTTP->HTTP链路返回内容前512字：\n" + string(body[:min(n, 512)]))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
