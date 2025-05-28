package config

import (
	"os"

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
