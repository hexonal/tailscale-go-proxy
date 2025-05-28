# tailscale-go-proxy

Tailscale 智能代理转发服务，支持 HTTP/HTTPS、SOCKS5 代理，基于 Headscale 动态注册和节点健康管理。

## 启动方式

```bash
go run main.go
```

## 主要端口
- HTTP 代理: 8081
- SOCKS5 代理: 1080
- 管理 API: 9091

## 主要依赖
- Go 1.21+
- Docker (可选) 