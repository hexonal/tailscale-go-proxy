package headscale

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

type RegisterResult struct {
	ID          int      `json:"id"`
	IPAddresses []string `json:"ip_addresses"`
	Error       string   `json:"error"`
}

// RegisterNodeByDockerExec 通过 docker exec 调用 headscale 注册节点，返回第一个 IPv4 地址
func RegisterNodeByDockerExec(key string) (string, error) {
	cmd := exec.Command(
		"docker", "exec", "-i", "headscale",
		"headscale", "--user", "flink", "nodes", "register",
		"--key", key, "--output", "json",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", errors.New("cmd: " + strings.Join(cmd.Args, " ") + ", error: " + err.Error() + ", output: " + out.String())
	}

	var result RegisterResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", errors.New(result.Error)
	}

	for _, ip := range result.IPAddresses {
		if strings.Contains(ip, ".") {
			return ip, nil
		}
	}
	return "", errors.New("no ipv4 address found")
}
