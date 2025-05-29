// Package gost 实现了基于用户名密码动态转发的 SOCKS5/HTTP 代理服务
package gost

import (
	"database/sql"
	"strconv"
	"sync"
)

// ========== 配置加载 ===========
// UserProxyMap 存储用户名密码到下游代理地址的映射关系。
// 键为 "username:password"，值为下游代理地址（如 http://host:port 或 socks5://host:port）。
var UserProxyMap = make(map[string]string)

// userProxyMapLock 用于保护 UserProxyMap 的并发读写。
var userProxyMapLock sync.RWMutex

// 代理端口常量，需与 main.go 启动端口保持一致
const (
	SOCKS5ProxyPort = 1080 // SOCKS5 代理端口
	HTTPProxyPort   = 1089 // HTTP 代理端口
	SourcePort      = 8939 //源端端口
)

// ========== 数据库加载与内存缓存 ===========
// UserEntry 结构体用于数据库查询
// 仅供包内通用方法使用
type UserEntry struct {
	Username string
	Password string
	Forward  string
}

// LoadUserProxyMap 从数据库加载用户代理映射到内存缓存
func LoadUserProxyMap(db *sql.DB) error {
	userProxyMapLock.Lock()
	defer userProxyMapLock.Unlock()
	UserProxyMap = make(map[string]string)

	rows, err := db.Query("SELECT reg_key, ip_address FROM register_key_ip_map")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, ip string
		if err := rows.Scan(&key, &ip); err != nil {
			return err
		}
		UserProxyMap[key+":"+key] = ip + ":" + strconv.Itoa(SourcePort)
	}
	return nil
}

// RefreshUserProxyMapFromDB 是 LoadUserProxyMap 的别名，便于兼容旧调用
func RefreshUserProxyMapFromDB(db *sql.DB) error {
	return LoadUserProxyMap(db)
}
