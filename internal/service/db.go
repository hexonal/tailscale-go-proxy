package service

import (
	"database/sql"
	"log"
	"tailscale-go-proxy/internal/config"
	_ "github.com/lib/pq"
)

// MustInitDB 初始化数据库并自动建表，失败直接 panic
func MustInitDB(cfg *config.Config) *sql.DB {
	dsn := "host=" + cfg.DBHost +
		" user=" + cfg.DBUser +
		" password=" + cfg.DBPassword +
		" dbname=" + cfg.DBName +
		" sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	if err := config.InitPGTable(db); err != nil {
		log.Fatalf("自动建表失败: %v", err)
	}
	return db
} 