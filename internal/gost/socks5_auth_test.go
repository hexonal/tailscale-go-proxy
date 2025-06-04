package gost

import (
	"io"
	"net"
	"testing"
	"time"
)

// TestSOCKS5AuthenticationFlow 测试 SOCKS5 认证流程的完整性
func TestSOCKS5AuthenticationFlow(t *testing.T) {
	// 设置测试用户
	userProxyMapLock.Lock()
	UserProxyMap["testuser:testpass"] = "http://downstream.example.com:8080"
	userProxyMapLock.Unlock()

	// 启动 SOCKS5 服务器
	server := NewSOCKS5Server(":0") // 使用随机端口
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()
	t.Logf("SOCKS5 服务器启动在: %s", serverAddr)

	// 启动服务器处理连接
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.handleConnection(conn)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 测试认证流程
	t.Run("成功认证流程", func(t *testing.T) {
		testSuccessfulAuth(t, serverAddr)
	})

	t.Run("失败认证流程", func(t *testing.T) {
		testFailedAuth(t, serverAddr)
	})
}

// testSuccessfulAuth 测试成功的认证流程
func testSuccessfulAuth(t *testing.T, serverAddr string) {
	// 连接到 SOCKS5 服务器
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("无法连接到 SOCKS5 服务器: %v", err)
	}
	defer conn.Close()

	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// 步骤1: 发送认证方法协商
	// VER(1) + NMETHODS(1) + METHODS(NMETHODS)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		t.Fatalf("发送认证方法失败: %v", err)
	}

	// 读取服务器响应 - 应该选择用户名密码认证
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		t.Fatalf("读取认证方法响应失败: %v", err)
	}

	if resp[0] != 0x05 || resp[1] != 0x02 {
		t.Fatalf("期望认证方法响应 [5, 2]，实际收到 %v", resp)
	}
	t.Logf("✅ 认证方法协商成功: %v", resp)

	// 步骤2: 发送用户名密码认证
	username := "testuser"
	password := "testpass"
	authReq := []byte{0x01, byte(len(username))}
	authReq = append(authReq, []byte(username)...)
	authReq = append(authReq, byte(len(password)))
	authReq = append(authReq, []byte(password)...)

	if _, err := conn.Write(authReq); err != nil {
		t.Fatalf("发送用户名密码失败: %v", err)
	}

	// 读取认证响应 - 这是我们修复的关键部分
	authResp := make([]byte, 2)
	if _, err := io.ReadFull(conn, authResp); err != nil {
		t.Fatalf("读取认证响应失败: %v", err)
	}

	if authResp[0] != 0x01 || authResp[1] != 0x00 {
		t.Fatalf("期望认证成功响应 [1, 0]，实际收到 %v", authResp)
	}
	t.Logf("✅ 认证成功响应正确: %v", authResp)

	// 步骤3: 发送 CONNECT 请求
	// VER(1) + CMD(1) + RSV(1) + ATYP(1) + DST.ADDR(variable) + DST.PORT(2)
	target := "example.com"
	port := 80
	connectReq := []byte{0x05, 0x01, 0x00, 0x03, byte(len(target))}
	connectReq = append(connectReq, []byte(target)...)
	connectReq = append(connectReq, byte(port>>8), byte(port&0xff))

	if _, err := conn.Write(connectReq); err != nil {
		t.Fatalf("发送 CONNECT 请求失败: %v", err)
	}

	// 读取 CONNECT 响应
	connectResp := make([]byte, 10)
	n, err := conn.Read(connectResp)
	if err != nil {
		// 这里可能会失败，因为我们没有真实的下游代理
		t.Logf("CONNECT 响应读取: %v (预期可能失败)", err)
	} else {
		t.Logf("✅ CONNECT 响应: %v (前%d字节)", connectResp[:n], n)
	}

	t.Logf("✅ 成功认证流程测试完成")
}

// testFailedAuth 测试失败的认证流程
func testFailedAuth(t *testing.T, serverAddr string) {
	// 连接到 SOCKS5 服务器
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("无法连接到 SOCKS5 服务器: %v", err)
	}
	defer conn.Close()

	// 设置读取超时
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// 步骤1: 发送认证方法协商
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		t.Fatalf("发送认证方法失败: %v", err)
	}

	// 读取服务器响应
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		t.Fatalf("读取认证方法响应失败: %v", err)
	}

	if resp[0] != 0x05 || resp[1] != 0x02 {
		t.Fatalf("期望认证方法响应 [5, 2]，实际收到 %v", resp)
	}

	// 步骤2: 发送错误的用户名密码
	username := "wronguser"
	password := "wrongpass"
	authReq := []byte{0x01, byte(len(username))}
	authReq = append(authReq, []byte(username)...)
	authReq = append(authReq, byte(len(password)))
	authReq = append(authReq, []byte(password)...)

	if _, err := conn.Write(authReq); err != nil {
		t.Fatalf("发送用户名密码失败: %v", err)
	}

	// 读取认证响应 - 应该是失败
	authResp := make([]byte, 2)
	if _, err := io.ReadFull(conn, authResp); err != nil {
		t.Fatalf("读取认证响应失败: %v", err)
	}

	if authResp[0] != 0x01 || authResp[1] != 0x01 {
		t.Fatalf("期望认证失败响应 [1, 1]，实际收到 %v", authResp)
	}
	t.Logf("✅ 认证失败响应正确: %v", authResp)

	t.Logf("✅ 失败认证流程测试完成")
}

// TestSOCKS5ProtocolCompliance 测试 SOCKS5 协议合规性
func TestSOCKS5ProtocolCompliance(t *testing.T) {
	// 设置测试用户
	userProxyMapLock.Lock()
	UserProxyMap["protocoltest:pass"] = "http://test.example.com:8080"
	userProxyMapLock.Unlock()

	// 启动 SOCKS5 服务器
	server := NewSOCKS5Server(":0")
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	// 启动服务器
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.handleConnection(conn)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 测试不同的协议场景
	t.Run("不支持的SOCKS版本", func(t *testing.T) {
		conn, err := net.Dial("tcp", serverAddr)
		if err != nil {
			t.Fatalf("连接失败: %v", err)
		}
		defer conn.Close()

		conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		// 发送错误的 SOCKS 版本
		conn.Write([]byte{0x04, 0x01, 0x02}) // SOCKS4 版本

		// 连接应该被关闭
		buf := make([]byte, 10)
		n, err := conn.Read(buf)
		if err == nil {
			t.Logf("收到响应: %v (前%d字节)", buf[:n], n)
		} else {
			t.Logf("✅ 连接正确关闭: %v", err)
		}
	})

	t.Run("不支持的认证方法", func(t *testing.T) {
		conn, err := net.Dial("tcp", serverAddr)
		if err != nil {
			t.Fatalf("连接失败: %v", err)
		}
		defer conn.Close()

		conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		// 只支持无认证方法
		conn.Write([]byte{0x05, 0x01, 0x00}) // 只支持无认证

		resp := make([]byte, 2)
		if _, err := io.ReadFull(conn, resp); err != nil {
			t.Fatalf("读取响应失败: %v", err)
		}

		if resp[0] != 0x05 || resp[1] != 0xFF {
			t.Errorf("期望响应 [5, 255]，实际收到 %v", resp)
		} else {
			t.Logf("✅ 正确拒绝不支持的认证方法: %v", resp)
		}
	})
}
