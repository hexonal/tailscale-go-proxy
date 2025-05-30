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
推荐使用 docker-compose，已集成 tailscaled、gost（v3.0.0 最新稳定版）、gin、数据库连接。
镜像已自动集成 gost，无需手动下载。
```bash
docker-compose up --build
```

### 环境变量配置

Docker 部署时，TS_AUTHKEY 通过环境变量传递。请在 docker 目录下创建 .env 文件，内容如下（可参考 .env.example）：

```env
TS_AUTHKEY=your-ts-authkey
```

- .env.example 已提供模板，实际部署时请复制为 .env 并填写真实的 Tailscale AuthKey。
- docker-compose.yml 会自动读取该变量用于 tailscale-go-proxy 服务。

### 如何获取 Tailscale AuthKey

- 如使用官方 Tailscale：
  1. 登录 https://login.tailscale.com/admin/settings/keys
  2. 点击"Generate auth key"生成新的 AuthKey。
  3. 复制该密钥，填入 .env 文件 TS_AUTHKEY 字段。

- 如使用自建 Headscale：
  1. 进入 headscale 容器：
     ```bash
     docker exec -it headscale headscale preauthkeys create --user flink --reusable --expiration 999999d
     ```
  2. 复制输出的 key，填入 .env 文件 TS_AUTHKEY 字段。
  3. 该 key 为永久有效（不会过期），可多次复用。

- 详细说明可参考：
  - [Tailscale 官方文档](https://tailscale.com/kb/1085/auth-keys)
  - [Headscale 官方文档](https://headscale.net/docs/)

### Headscale 创建用户

headscale 服务启动后，可通过以下命令在容器内创建用户（如 flink）：

```bash
docker exec -it headscale headscale users create flink
```

- 该命令会在 headscale 控制面创建名为 flink 的用户。
- 如需查看所有用户，可执行：
  ```bash
  docker exec -it headscale headscale users list
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
- **用户代理映射全部由数据库驱动，服务启动和注册节点后，自动从数据库加载到内存缓存（O(1)查找），无需手动维护 gost 配置文件**

#### 示例
```bash
# HTTP 代理带认证
curl -x http://yourkey:yourkey@localhost:1089 https://ipinfo.io

# SOCKS5 代理带认证
curl --socks5 yourkey:yourkey@localhost:1080 https://ipinfo.io
```

---

## 注册 API 用法

- 注册节点（写入数据库，自动刷新 gost 配置）
- 接口：`POST /register`
- 示例：
```bash
curl -X POST http://localhost:8081/register \
  -H 'Content-Type: application/json' \
  -d '{"key": "yourkey"}'
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
  - 检查服务是否已自动从数据库刷新内存缓存（无需关心 gost 配置文件）
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
- internal/gost 代理配置与进程管理（**用户代理映射仅依赖数据库和内存缓存**）
- internal/service 数据库初始化
- internal/api gin 路由注册
- internal/config 配置加载

---

## 参考
- 详细设计方案见 docs/设计方案.md 

---

## 部署说明

### 1. 域名准备

本项目涉及多个服务，建议为每个服务准备独立的二级域名，并为每个域名申请有效的 SSL 证书。常见域名分配如下：

| 服务         | 推荐域名                  | 用途说明           |
| ------------ | ------------------------- | ------------------ |
| DERP         | derp.yourdomain.com       | DERP 中继服务器    |
| Headscale    | headscale.yourdomain.com  | Headscale 控制面   |
| Headplane    | hs.yourdomain.com         | Headplane 管理界面 |

**注意：**  
- 请将 `yourdomain.com` 替换为你自己的主域名。
- 证书文件建议命名为对应域名，如 `derp.yourdomain.com.pem` 和 `derp.yourdomain.com.key`。

---

### 2. 证书准备

将所有域名的 SSL 证书（`.pem` 和 `.key` 文件）放入 `docker/nginx/ssl/` 目录下，并确保 Nginx 配置文件中路径正确。

---

### 3. 配置文件修改

#### 3.1 Nginx 配置

- 进入 `docker/nginx/conf.d/` 目录，编辑 `derper.conf`、`headplane.conf`、`headscale.conf` 等文件。
- 替换所有 `server_name`、`ssl_certificate`、`ssl_certificate_key` 字段为你的实际域名和证书路径。例如：

  ```nginx
  server_name derp.yourdomain.com;
  ssl_certificate /etc/nginx/ssl/derp.yourdomain.com.pem;
  ssl_certificate_key /etc/nginx/ssl/derp.yourdomain.com.key;
  ```

#### 3.2 DERP 配置

- 编辑 `docker/config/derp.yaml`，将 `hostname` 字段替换为你的 DERP 域名：

  ```yaml
  hostname: "derp.yourdomain.com"
  ```

#### 3.3 Headscale 配置

- 编辑 `docker/config/config.yaml`，将 `server_url` 字段替换为你的 Headscale 域名：

  ```yaml
  server_url: https://headscale.yourdomain.com
  ```

#### 3.4 docker-compose 环境变量

- 编辑 `docker/docker-compose.yml`，将 `DERP_DOMAIN`、`DERP_HOSTNAME` 等环境变量替换为你的实际域名：

  ```yaml
  - DERP_DOMAIN=derp.yourdomain.com
  - DERP_HOSTNAME=derp.yourdomain.com
  ```

---

### 4. 数据库初始化

- 默认已集成 PostgreSQL 服务，配置见 `docker/docker-compose.yml` 和 `docker/config/config.yaml`。
- 默认数据库信息如下（如需自定义请同步修改相关配置文件）：

  ```
  db_host: pg
  db_port: 5432
  db_user: tailscale
  db_password: tailscale
  db_name: tailscale
  ```

---

### 5. 启动服务

在项目根目录下执行：

```bash
cd docker
docker-compose up --build
```

- 首次启动会自动拉取镜像并初始化数据卷。
- 各服务会自动通过 Nginx 反向代理对外提供 HTTPS 服务。

---

#### 已有安装包/镜像的快速启动

如果你已经提前构建好镜像或下载好官方安装包，无需再次执行 `--build`，可以直接运行：

```bash
cd docker
docker-compose up -d
```

- 这样会直接使用现有镜像和配置启动所有服务。
- 如需更新镜像，请先执行 `docker-compose pull` 拉取最新镜像，再执行上述命令。

---

### 6. 访问服务

- **DERP 中继**：`https://derp.yourdomain.com`
- **Headscale 控制面**：`https://headscale.yourdomain.com`
- **Headplane 管理界面**：`https://hs.yourdomain.com`

---

### 7. 其他注意事项

- **端口占用**：请确保 80、443、5432、8081、1080、1089 等端口未被其他服务占用。
- **TUN 设备权限**：容器需具备 `/dev/net/tun` 设备访问权限，已在 compose 文件中配置。
- **环境变量**：Tailscale AuthKey 需通过 `.env` 文件传递，详见前文"环境变量配置"章节。
- **数据库持久化**：数据卷 `pg-data`、`headscale-data`、`tailscale-data`、`headplane-data` 用于持久化存储。

---

如需自定义更多参数，请参考各配置文件注释及官方文档。  
如遇问题可查阅"常见问题与故障排查"章节。
