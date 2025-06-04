package register

import (
	"database/sql"
	"tailscale-go-proxy/internal/gost"
	"tailscale-go-proxy/internal/headscale"

	"github.com/gin-gonic/gin"
)

type RegisterRequest struct {
	Key string `json:"key" binding:"required"`
}

type RegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// handleRegisterCommon 公共注册处理逻辑
func handleRegisterCommon(key string, db *sql.DB, c *gin.Context) {
	// 1. 调用 headscale 注册节点，返回分配的 IP
	ip, err := headscale.RegisterNodeByDockerExec(key)
	if err != nil {
		c.JSON(500, RegisterResponse{Success: false, Message: "注册失败: " + err.Error()})
		return
	}

	// 2. 将 key 和 IP 映射关系写入数据库
	if err := headscale.SaveKeyIP(db, key, ip); err != nil {
		c.JSON(500, RegisterResponse{Success: false, Message: "数据库保存失败: " + err.Error()})
		return
	}

	// 3. 注册和数据库都成功后，增量写入 gost 配置并热加载，保证新注册用户立即生效
	if err := gost.AddUserToProxyMap(key, ip); err != nil {
		c.JSON(500, RegisterResponse{Success: false, Message: "gost 配置更新失败: " + err.Error()})
		return
	}

	// 4. 返回注册成功和分配的 IP
	c.JSON(200, RegisterResponse{Success: true, Message: "注册成功，IP: " + ip})
}

// HandleRegister 处理注册请求，注册成功后自动热加载 gost 配置
func HandleRegister(c *gin.Context, db *sql.DB) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, RegisterResponse{Success: false, Message: "参数错误"})
		return
	}
	handleRegisterCommon(req.Key, db, c)
}

// HandleRegisterV2 处理新版注册请求，支持 code 注册码（GET 方法，参数从 query 获取）
func HandleRegisterV2(c *gin.Context, db *sql.DB) {
	code := c.Query("code")
	if code == "" {
		c.JSON(400, RegisterResponse{Success: false, Message: "缺少 code 参数"})
		return
	}
	handleRegisterCommon(code, db, c)
}
