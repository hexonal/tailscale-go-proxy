# 链式代理使用指南

本文档介绍如何使用新实现的链式代理功能，该功能支持多层代理转发，适用于复杂的网络环境。

## 功能特性

- **多层代理支持**: 支持任意数量的代理层级
- **混合协议**: 支持 SOCKS5 和 HTTP 代理的任意组合
- **认证简化**: 仅需在第一层代理进行认证，后续层级自动跳过认证
- **透明兼容**: 现有客户端代码无需修改，完全透明支持
- **错误处理**: 完善的错误处理和日志记录

## 代理链格式

### 基本格式
```
第一层代理 -> 第二层代理 -> 第三层代理 -> ...
```

### 支持的协议
- `http://[user:pass@]host:port`
- `https://[user:pass@]host:port`
- `socks5://[user:pass@]host:port`

### 示例格式
```bash
# 简单的两层代理链
socks5://user:pass@proxy1.example.com:1080 -> http://proxy2.example.com:8080

# 复杂的三层代理链
socks5://admin:secret@first-proxy:1080 -> http://second-proxy:8080 -> socks5://third-proxy:1080

# 混合协议代理链
http://user1:pass1@http-proxy:8080 -> socks5://socks-proxy:1080 -> http://final-proxy:3128
```

## 配置方式

### 1. 设置用户代理映射

在代码中设置 `UserProxyMap`：

```go
import "tailscale-go-proxy/internal/gost"

// 配置单个代理（原有方式）
gost.UserProxyMap["user1:pass1"] = "http://downstream.example.com:8080"

// 配置代理链（新功能）
gost.UserProxyMap["admin:secret"] = "socks5://proxy1:1080 -> http://proxy2:8080 -> socks5://proxy3:1080"
```

### 2. 启动代理服务器

```go
package main

import (
    "log"
    "tailscale-go-proxy/internal/gost"
)

func main() {
    // 配置用户代理映射
    gost.UserProxyMap["chain-user:chain-pass"] = "socks5://first:1080 -> http://second:8080"
    
    // 启动 HTTP 代理服务器
    httpProxy := gost.NewHTTPProxyServer(":8080")
    go func() {
        if err := httpProxy.Start(); err != nil {
            log.Fatal("HTTP proxy failed:", err)
        }
    }()
    
    // 启动 SOCKS5 代理服务器
    socksProxy := gost.NewSOCKS5Server(":1080")
    if err := socksProxy.Start(); err != nil {
        log.Fatal("SOCKS5 proxy failed:", err)
    }
}
```

## 客户端使用

### HTTP 客户端

```go
package main

import (
    "fmt"
    "net/http"
    "net/url"
)

func main() {
    // 设置代理 URL，请求将通过代理链转发
    proxyURL, _ := url.Parse("http://chain-user:chain-pass@localhost:8080")
    
    client := &http.Client{
        Transport: &http.Transport{
            Proxy: http.ProxyURL(proxyURL),
        },
    }
    
    resp, err := client.Get("https://httpbin.org/ip")
    if err != nil {
        fmt.Printf("Request failed: %v\n", err)
        return
    }
    defer resp.Body.Close()
    
    fmt.Printf("Status: %s\n", resp.Status)
}
```

### SOCKS5 客户端

```go
package main

import (
    "fmt"
    "net"
    "golang.org/x/net/proxy"
)

func main() {
    // 创建 SOCKS5 dialer，请求将通过代理链转发
    dialer, err := proxy.SOCKS5("tcp", "localhost:1080",
        &proxy.Auth{
            User:     "chain-user",
            Password: "chain-pass",
        },
        proxy.Direct,
    )
    if err != nil {
        fmt.Printf("Failed to create SOCKS5 dialer: %v\n", err)
        return
    }
    
    // 通过代理链连接目标服务器
    conn, err := dialer.Dial("tcp", "httpbin.org:80")
    if err != nil {
        fmt.Printf("Failed to connect: %v\n", err)
        return
    }
    defer conn.Close()
    
    // 发送 HTTP 请求
    fmt.Fprintf(conn, "GET /ip HTTP/1.1\r\nHost: httpbin.org\r\nConnection: close\r\n\r\n")
    
    // 读取响应
    buf := make([]byte, 4096)
    n, _ := conn.Read(buf)
    fmt.Printf("Response:\n%s\n", string(buf[:n]))
}
```

## 现有代码兼容性

### Post.go 代码兼容性

您现有的 `Post.go` 代码无需任何修改，自动支持链式代理：

```go
// 原有代码保持不变
func (httpclient *HttpClientModel) POST(cgiurl string, data []byte, host string, P models.ProxyInfo) ([]byte, error) {
    // ... 现有逻辑保持不变
    // 当 P.ProxyIp 配置为代理链格式时，自动使用链式代理
}
```

只需在配置 `ProxyInfo` 时使用链式格式：

```go
proxy := models.ProxyInfo{
    ProxyIp:       "socks5://first-proxy:1080 -> http://second-proxy:8080",
    ProxyUser:     "user",     // 第一层代理的用户名
    ProxyPassword: "password", // 第一层代理的密码
}
```

### TcpClient.go 代码兼容性

`TcpClient.go` 中的 `CreateConnection` 函数也完全兼容：

```go
// 原有调用方式保持不变
conn, err := CreateConnection(
    "target.example.com:80",
    "socks5://proxy1:1080 -> http://proxy2:8080", // 使用链式代理格式
    "username",
    "password",
)
```

## 认证机制

### 第一层代理认证（我们的代理服务器）

- **HTTP代理服务器** (`http_to_http.go`): 接收客户端的 Proxy-Authorization 头进行认证
- **SOCKS5代理服务器** (`socks_http.go`): 接收客户端的 SOCKS5 用户名密码进行认证
- 认证通过后，从 `UserProxyMap` 中获取对应的代理链配置

### 后续代理链转发

- **完全跳过认证**: 所有后续代理都不会发送任何认证信息
- **自动清理**: 即使代理链配置中包含认证信息，系统也会自动移除
- **无认证转发**: 代理链中的每一层都以无认证方式连接

### 工作流程示例

```
客户端 ──认证──> 第一层代理服务器 ──无认证──> 代理链
                 (http_to_http.go)              proxy1 -> proxy2 -> proxy3
                 (socks_http.go)
```

1. 客户端向我们的代理服务器发送认证信息（用户名:密码）
2. 代理服务器验证认证信息，获取代理链配置
3. 后续所有代理连接都不携带任何认证信息

## 错误处理

### 常见错误

1. **代理链格式错误**
   ```
   错误：代理链格式错误：至少需要 2 个有效代理
   解决：确保代理链包含至少两个有效的代理地址
   ```

2. **不支持的协议**
   ```
   错误：代理链第 1 层协议不支持: socks4
   解决：只使用支持的协议（http, https, socks5）
   ```

3. **连接失败**
   ```
   错误：连接第一层代理失败: dial tcp: connection refused
   解决：检查第一层代理服务器是否正常运行
   ```

### 日志记录

系统会记录详细的代理链连接日志：

```
HTTP: User admin authenticated, using proxy: socks5://proxy1:1080 -> http://proxy2:8080
```

## 性能注意事项

1. **延迟叠加**: 每层代理都会增加网络延迟
2. **带宽限制**: 整个链路的带宽受最慢代理限制
3. **连接池**: 建议在客户端使用连接池来复用连接
4. **超时设置**: 合理设置客户端超时时间，考虑多层代理的延迟

## 监控和调试

### 启用详细日志

```go
import "log"

// 设置日志级别以查看详细的代理连接信息
log.SetFlags(log.LstdFlags | log.Lshortfile)
```

### 测试代理链

```bash
# 测试 HTTP 代理链
curl -x "http://user:pass@localhost:8080" "https://httpbin.org/ip"

# 测试 SOCKS5 代理链
curl --socks5 "user:pass@localhost:1080" "https://httpbin.org/ip"
```

## 最佳实践

1. **安全性**: 确保所有代理连接都使用加密传输（HTTPS/SOCKS5）
2. **可靠性**: 选择稳定可靠的代理服务器
3. **监控**: 实施代理服务器的健康检查和监控
4. **备份**: 为关键应用配置备用代理链
5. **认证**: 使用强密码和定期轮换认证信息

## 故障排除

### 常见问题

1. **代理链中某一层断开**
   - 整个请求会失败
   - 检查各层代理服务器状态
   - 考虑实施自动重试机制

2. **认证失败**
   - 检查第一层代理的用户名密码是否正确
   - 确认代理服务器支持对应的认证方式

3. **性能问题**
   - 减少代理层级数量
   - 选择地理位置更近的代理服务器
   - 优化网络配置

### 调试工具

```bash
# 检查代理连接
telnet proxy1.example.com 1080

# 测试 HTTP 代理
curl -v -x "http://proxy:8080" "http://example.com"

# 测试 SOCKS5 代理
curl -v --socks5 "proxy:1080" "http://example.com"
``` 