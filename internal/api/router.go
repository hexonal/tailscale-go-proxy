package api

import (
	"database/sql"
	"tailscale-go-proxy/internal/register"
	"github.com/gin-gonic/gin"
)

// NewRouter 创建 gin 路由
func NewRouter(db *sql.DB) *gin.Engine {
	r := gin.Default()
	r.POST("/register", func(c *gin.Context) {
		register.HandleRegister(c, db)
	})
	return r
} 