package config

import (
	"database/sql"
	"os"

	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPProxyPort       int    `yaml:"http_proxy_port"`
	SOCKS5ProxyPort     int    `yaml:"socks5_proxy_port"`
	ManageAPIPort       int    `yaml:"manage_api_port"`
	HeadscaleGRPCAddr   string `yaml:"headscale_grpc_addr"`
	HeadscaleHTTPAddr   string `yaml:"headscale_http_addr"`
	CacheUpdateInterval int    `yaml:"cache_update_interval"`
	StatusCheckInterval int    `yaml:"status_check_interval"`

	DBHost     string `yaml:"db_host"`
	DBPort     int    `yaml:"db_port"`
	DBUser     string `yaml:"db_user"`
	DBPassword string `yaml:"db_password"`
	DBName     string `yaml:"db_name"`
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg Config
	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// InitPGTable 检查并自动创建 register_key_ip_map 表
func InitPGTable(db *sql.DB) error {
	createTableSQL := `CREATE TABLE IF NOT EXISTS register_key_ip_map (
		id SERIAL PRIMARY KEY,
		reg_key VARCHAR(255) NOT NULL UNIQUE,
		ip_address VARCHAR(64) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`
	_, err := db.Exec(createTableSQL)
	return err
}
