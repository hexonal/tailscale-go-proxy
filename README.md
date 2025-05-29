# tailscale-go-proxy

Tailscale 智能代理转发服务

## 功能特性
- 支持 HTTP/HTTPS 代理（8081）
- 支持 SOCKS5 代理（1080）
- 支持基于用户名密码的节点认证与精确转发
- 提供注册 API（/register）动态添加代理节点
- 自动集成 Tailscale 网络，支持 Headscale 控制面
- 支持 Docker 一体化部署

---

## 快速开始

### 1. 本地运行
```bash
git clone <repo-url>
cd tailscale-go-proxy
go run main.go
```

### 2. 编译
```bash
go build -o tailscale-go-proxy main.go
```

### 3. Docker 部署
推荐使用 docker-compose，已集成 tailscaled、gost、gin、数据库连接。
```bash
docker-compose up --build
```

---

## 代理功能说明

### HTTP/HTTPS 代理
- 监听端口：8081
- 支持标准 HTTP 代理协议
- 支持用户名密码认证（用户名=密码=key）

### SOCKS5 代理
- 监听端口：1080
- 支持 SOCKS5 协议
- 支持用户名密码认证（用户名=密码=key）

### 认证与节点路由
- 认证格式：`key:key@host:port`
- 认证通过后，流量自动转发到数据库注册的目标节点

#### 示例
```bash
# HTTP 代理带认证
curl -x http://yourkey:yourkey@localhost:8081 https://example.com

# SOCKS5 代理带认证
curl --socks5 yourkey:yourkey@localhost:1080 https://example.com
```

---

## 注册 API 用法

- 注册节点（写入数据库，自动刷新 gost 配置）
- 接口：`POST /register`
- 示例：
```bash
curl -X POST http://localhost:9091/register -H 'Content-Type: application/json' -d '{"key": "yourkey", "ip": "192.168.1.101"}'
```
- 注册后即可用 yourkey 作为代理认证信息进行流量转发

---

## 配置文件说明

- 配置文件路径：`config.yaml`
- 主要字段：
  - `manage_api_port`：管理 API 端口（如 9091）
  - `db_host`、`db_port`、`db_user`、`db_password`、`db_name`：PostgreSQL 数据库连接信息
- 示例：
```yaml
manage_api_port: 9091
db_host: pg
db_port: 5432
db_user: tailscale
db_password: tailscale
db_name: tailscale
```

---

## 常见问题与故障排查

- **代理无法认证/转发？**
  - 检查注册 API 是否已正确注册 key 和目标 IP
  - 检查 gost 配置是否已自动刷新
  - 检查数据库连接和 tailscale 网络状态
- **tailscale 网络不通？**
  - 检查 TS_AUTHKEY 环境变量和 headscale 服务
  - 检查容器是否具备 NET_ADMIN、/dev/net/tun 权限
- **端口冲突？**
  - 检查 8081/1080/9091 端口是否被占用

---

## 目录结构
- main.go 只负责组装和启动
- internal/tailscale 进程管理
- internal/gost 代理配置与进程管理
- internal/service 数据库初始化
- internal/api gin 路由注册
- internal/config 配置加载

---

## 参考
- 详细设计方案见 docs/设计方案.md 