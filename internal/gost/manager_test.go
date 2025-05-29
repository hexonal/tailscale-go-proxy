package gost

import (
	"os"
	"testing"
)

func TestLoadUserProxyMap(t *testing.T) {
	// 构造一个临时配置文件
	configContent := `
services:
  - users:
      - username: testuser
        password: testpass
        forward: 127.0.0.1:9999
  - users:
      - username: foo
        password: bar
        forward: 10.0.0.1:8080
`
	file := "test-gost-config.yaml"
	if err := os.WriteFile(file, []byte(configContent), 0644); err != nil {
		t.Fatalf("写入测试配置文件失败: %v", err)
	}
	defer os.Remove(file)

	if err := LoadUserProxyMap(file); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if got := UserProxyMap["testuser:testpass"]; got != "127.0.0.1:9999" {
		t.Errorf("UserProxyMap 加载错误，期望 127.0.0.1:9999，实际 %s", got)
	}
	if got := UserProxyMap["foo:bar"]; got != "10.0.0.1:8080" {
		t.Errorf("UserProxyMap 加载错误，期望 10.0.0.1:8080，实际 %s", got)
	}
}

func TestReloadUserProxyMap(t *testing.T) {
	configContent := `
services:
  - users:
      - username: reload
        password: reload
        forward: 1.2.3.4:1234
`
	file := "test-gost-config.yaml"
	if err := os.WriteFile(file, []byte(configContent), 0644); err != nil {
		t.Fatalf("写入测试配置文件失败: %v", err)
	}
	defer os.Remove(file)

	if err := ReloadUserProxyMap(file); err != nil {
		t.Fatalf("ReloadUserProxyMap 失败: %v", err)
	}
	userProxyMapLock.RLock()
	defer userProxyMapLock.RUnlock()
	if got := UserProxyMap["reload:reload"]; got != "1.2.3.4:1234" {
		t.Errorf("ReloadUserProxyMap 加载错误，期望 1.2.3.4:1234，实际 %s", got)
	}
} 