package gost

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"gopkg.in/yaml.v3"
)

type GostUser struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Forward  string `yaml:"forward"`
}

type GostService struct {
	Name  string     `yaml:"name"`
	Addr  string     `yaml:"addr"`
	Users []GostUser `yaml:"users"`
}

type GostConfig struct {
	Services []GostService `yaml:"services"`
}

var (
	gostCmd  *exec.Cmd
	gostLock sync.Mutex
)

// EnsureReady 生成 gost 配置并启动 gost 进程
func EnsureReady(db *sql.DB) error {
	users, err := fetchUsersFromDB(db)
	if err != nil {
		return fmt.Errorf("查询用户失败: %w", err)
	}
	gostConfigPath := "gost-config.yaml"
	if err := writeGostConfig(gostConfigPath, users); err != nil {
		return fmt.Errorf("写入 gost 配置失败: %w", err)
	}
	if err := startGost(gostConfigPath); err != nil {
		return fmt.Errorf("gost 启动失败: %w", err)
	}
	return nil
}

func fetchUsersFromDB(db *sql.DB) ([]GostUser, error) {
	rows, err := db.Query("SELECT reg_key, ip_address FROM register_key_ip_map")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []GostUser
	for rows.Next() {
		var username, ip string
		if err := rows.Scan(&username, &ip); err != nil {
			return nil, err
		}
		users = append(users, GostUser{
			Username: username,
			Password: username, // 可根据实际情况调整
			Forward:  "http://" + ip + ":8939",
		})
	}
	return users, nil
}

func writeGostConfig(path string, users []GostUser) error {
	cfg := GostConfig{
		Services: []GostService{
			{
				Name:  "socks5",
				Addr:  ":1080",
				Users: users,
			},
			{
				Name:  "http",
				Addr:  ":1089",
				Users: users,
			},
		},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func startGost(configPath string) error {
	gostLock.Lock()
	defer gostLock.Unlock()
	if gostCmd != nil && gostCmd.Process != nil {
		_ = gostCmd.Process.Kill()
	}
	gostCmd = exec.Command("/usr/local/bin/gost", "-C", configPath)
	gostCmd.Stdout = os.Stdout
	gostCmd.Stderr = os.Stderr
	return gostCmd.Start()
} 