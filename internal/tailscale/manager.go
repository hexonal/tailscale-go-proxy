package tailscale

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// EnsureReady 启动 tailscaled 并完成 up，确保 Tailscale 网络就绪
func EnsureReady(authKey, loginServer string) error {
	if authKey == "" {
		return fmt.Errorf("TS_AUTHKEY 环境变量未设置")
	}
	if err := startTailscaled(); err != nil {
		return fmt.Errorf("启动 tailscaled 失败: %w", err)
	}
	if err := waitTailscaledReady(); err != nil {
		return fmt.Errorf("tailscaled sock 未就绪: %w", err)
	}
	if err := tailscaleUp(authKey, loginServer); err != nil {
		return fmt.Errorf("tailscale up 失败: %w", err)
	}
	if err := waitTailscaleIP(); err != nil {
		return fmt.Errorf("tailscale IP 未就绪: %w", err)
	}
	return nil
}

func startTailscaled() error {
	cmd := exec.Command("tailscaled", "--state=/var/lib/tailscale/tailscaled.state", "--socket=/var/run/tailscale/tailscaled.sock")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// tailscaled 需要常驻后台运行
	if err := cmd.Start(); err != nil {
		return err
	}
	// 用 goroutine 等待 tailscaled 退出，避免僵尸进程
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

func waitTailscaledReady() error {
	for i := 0; i < 30; i++ {
		if _, err := os.Stat("/var/run/tailscale/tailscaled.sock"); err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("tailscaled sock not ready")
}

func tailscaleUp(authKey, loginServer string) error {
	args := []string{"up", "--authkey=" + authKey, "--hostname=go-proxy", "--accept-dns=true"}
	if loginServer != "" {
		args = append(args, "--login-server="+loginServer)
	}
	cmd := exec.Command("tailscale", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		// 打印出具体命令和参数，方便排查
		fmt.Printf("tailscale up 执行失败，命令: tailscale %v，错误: %v\n", args, err)
	}
	return err
}

func waitTailscaleIP() error {
	for i := 0; i < 30; i++ {
		out, err := exec.Command("tailscale", "ip").Output()
		if err == nil && len(out) > 0 {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("tailscale ip not ready")
} 