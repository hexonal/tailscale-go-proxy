package tailscale

import (
	"os/exec"
	"testing"
	"os"
	"tailscale-go-proxy/internal/config"
	"runtime"
)

// mock exec.Command 用于测试
func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestEnsureReady_InvalidAuthKey(t *testing.T) {
	err := EnsureReady("", "")
	if err == nil || err.Error() != "TS_AUTHKEY 环境变量未设置" {
		t.Errorf("期望 TS_AUTHKEY 环境变量未设置 错误，实际: %v", err)
	}
}

func TestEnsureReady_WithRealConfigFile(t *testing.T) {
	cfg, err := config.LoadConfig("../../docker/config/tailscale-go-proxy-config.yaml")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.TSAuthKey == "" || cfg.LoginServer == "" {
		t.Fatalf("配置文件 ts_authkey 或 login_server 为空")
	}
	// 这里只做参数传递测试，不关心 tailscaled 是否真的启动
	err = EnsureReady(cfg.TSAuthKey, cfg.LoginServer)
	if err == nil {
		t.Log("EnsureReady 执行通过（未真正启动 tailscaled，仅参数测试）")
	} else {
		t.Logf("EnsureReady 返回错误（预期可能因未启动 tailscaled）: %v", err)
	}
}

func TestEnsureReady_Integration(t *testing.T) {
	cfg, err := config.LoadConfig("../../docker/config/tailscale-go-proxy-config.yaml")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.TSAuthKey == "" || cfg.LoginServer == "" {
		t.Fatalf("配置文件 ts_authkey 或 login_server 为空")
	}
	if os.Geteuid() != 0 {
		t.Skip("需要 root 权限运行 tailscaled")
	}
	if runtime.GOOS != "linux" {
		t.Skip("仅支持在 Linux 下运行 tailscaled 集成测试")
	}
	// 清理 tailscaled 进程，防止重复启动
	_ = exec.Command("pkill", "tailscaled").Run()
	// 执行集成测试
	err = EnsureReady(cfg.TSAuthKey, cfg.LoginServer)
	if err != nil {
		t.Fatalf("EnsureReady 启动失败: %v", err)
	}
	t.Log("EnsureReady 启动成功，tailscaled 和 tailscale up 均无报错")
}
