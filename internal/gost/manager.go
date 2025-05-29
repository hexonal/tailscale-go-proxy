// Package gost 实现了基于用户名密码动态转发的 SOCKS5/HTTP 代理服务
package gost

import (
	"database/sql"
	"os"
	"strconv"
	"sync"

	"gopkg.in/yaml.v3"
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
	
	SourcePort = 8939 //源端端口
)

// gost 配置文件路径常量，便于统一维护
const configPath = "gost-config.yaml"

// gost 用户配置结构体
// 仅供包内通用方法使用
type UserEntry struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Forward  string `yaml:"forward"`
}

type GostConfig struct {
	Services []struct {
		Users []UserEntry `yaml:"users"`
	} `yaml:"services"`
}

// loadGostConfig 读取并解析 gost 配置文件
func loadGostConfig(path string) (*GostConfig, error) {
	var cfg GostConfig
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

// saveGostConfig 序列化并写回 gost 配置文件
func saveGostConfig(path string, cfg *GostConfig) error {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// makeUserEntry 构造用户 entry，forward 端口与 SOCKS5 代理端口保持一致
func makeUserEntry(key, ip string) UserEntry {
	return UserEntry{
		Username: key,
		Password: key,
		Forward:  ip + ":" + strconv.Itoa(SourcePort),
	}
}

// LoadUserProxyMap 从指定的 YAML 配置文件加载用户代理映射。
// configPath 为配置文件路径。
// 返回 error 表示加载或解析失败。
func LoadUserProxyMap(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	type userConfig struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
		Forward  string `yaml:"forward"`
	}
	type config struct {
		Services []struct {
			Users []userConfig `yaml:"users"`
		} `yaml:"services"`
	}
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	UserProxyMap = make(map[string]string)
	for _, svc := range cfg.Services {
		for _, u := range svc.Users {
			UserProxyMap[u.Username+":"+u.Password] = u.Forward
		}
	}
	return nil
}

// ReloadUserProxyMap 重新加载用户代理映射（带写锁保护）。
// configPath 为配置文件路径。
// 返回 error 表示加载或解析失败。
func ReloadUserProxyMap(configPath string) error {
	userProxyMapLock.Lock()
	defer userProxyMapLock.Unlock()
	return LoadUserProxyMap(configPath)
}

// RefreshUserProxyMapFromDB 从数据库读取所有 key/ip，生成 gost 配置 yaml，写入磁盘并热加载到内存。
// 参数 db 为数据库连接。
// 返回 error 表示任一步骤失败。
func RefreshUserProxyMapFromDB(db *sql.DB) error {
	// 1. 查询数据库，获取所有 reg_key/ip_address 映射（表结构与 headscale/db.go、service/db.go 保持一致）
	rows, err := db.Query("SELECT reg_key, ip_address FROM register_key_ip_map")
	if err != nil {
		return err
	}
	defer rows.Close()
	var users []UserEntry
	for rows.Next() {
		var key, ip string
		if err := rows.Scan(&key, &ip); err != nil {
			return err
		}
		users = append(users, makeUserEntry(key, ip))
	}
	// 2. 组装 gost 配置结构
	cfg := &GostConfig{
		Services: []struct {
			Users []UserEntry `yaml:"users"`
		}{
			{Users: users},
		},
	}
	// 3. 保存并热加载
	if err := saveGostConfig(configPath, cfg); err != nil {
		return err
	}
	return ReloadUserProxyMap(configPath)
}

// AddUserToProxyMap 增量将新用户写入 gost 配置文件并热加载到内存。
// 参数 key 为用户名（也是密码），ip 为目标节点 IP。
// 返回 error 表示写入或热加载失败。
func AddUserToProxyMap(key, ip string) error {
	// 1. 读取现有配置
	cfg, err := loadGostConfig(configPath)
	if err != nil {
		return err
	}
	// 2. 追加新用户
	if len(cfg.Services) == 0 {
		cfg.Services = append(cfg.Services, struct {
			Users []UserEntry `yaml:"users"`
		}{})
	}
	cfg.Services[0].Users = append(cfg.Services[0].Users, makeUserEntry(key, ip))
	// 3. 保存并热加载
	if err := saveGostConfig(configPath, cfg); err != nil {
		return err
	}
	return ReloadUserProxyMap(configPath)
}
