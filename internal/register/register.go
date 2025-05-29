package register

import (
	"database/sql"
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

func HandleRegister(c *gin.Context, db *sql.DB) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, RegisterResponse{Success: false, Message: "参数错误"})
		return
	}

	ip, err := headscale.RegisterNodeByDockerExec(req.Key)
	if err != nil {
		c.JSON(500, RegisterResponse{Success: false, Message: "注册失败: " + err.Error()})
		return
	}

	if err := headscale.SaveKeyIP(db, req.Key, ip); err != nil {
		c.JSON(500, RegisterResponse{Success: false, Message: "数据库保存失败: " + err.Error()})
		return
	}

	c.JSON(200, RegisterResponse{Success: true, Message: "注册成功，IP: " + ip})
}
