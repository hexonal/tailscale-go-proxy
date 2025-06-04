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
	// 支持 /registerV2/:key 形式，将 key 作为 code 传递
	r.GET("/registerV2/:key", func(c *gin.Context) {
		register.HandleRegisterV2(c, db)
	})
	return r
}
