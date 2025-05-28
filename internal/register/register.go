package register

import (
	"tailscale-go-proxy/internal/cache"
	"tailscale-go-proxy/internal/config"
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

func HandleRegister(c *gin.Context, nodeCache *cache.NodeCache, cfg *config.Config) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, RegisterResponse{Success: false, Message: "参数错误"})
		return
	}

	nodeInfo, err := headscale.RegisterNode(req.Key)
	if err != nil {
		c.JSON(500, RegisterResponse{Success: false, Message: "headscale 注册失败"})
		return
	}

	if nodeInfo.Device == "Android" {
		nodeCache.Add(&cache.Node{
			ID:     nodeInfo.ID,
			IP:     nodeInfo.IP,
			Port:   nodeInfo.Port,
			Device: nodeInfo.Device,
			Online: true,
		})
		c.JSON(200, RegisterResponse{Success: true, Message: "注册成功，已加入代理池 (Android)"})
		return
	} else {
		c.JSON(200, RegisterResponse{Success: true, Message: "注册成功，但已忽略 (iOS)"})
		return
	}
}
