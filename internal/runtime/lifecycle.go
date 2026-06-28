package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// NoopConfigChecker 是不执行外部命令的配置检查器。
type NoopConfigChecker struct{}

// Check 对配置内容执行空检查，适用于测试或未安装 sing-box 的环境。
func (NoopConfigChecker) Check(ctx context.Context, instance string, data []byte) error {
	return nil
}

// CommandConfigChecker 通过 sing-box check -c 检查配置。
type CommandConfigChecker struct {
	Binary string
}

// Check 将配置写入临时文件并用参数数组调用 sing-box check。
func (c CommandConfigChecker) Check(ctx context.Context, instance string, data []byte) error {
	binary := c.Binary
	if binary == "" {
		found, err := exec.LookPath("sing-box")
		if err != nil {
			return fmt.Errorf("找不到 sing-box 二进制: %w", err)
		}
		binary = found
	}

	tempFile, err := os.CreateTemp("", "sbox-manager-check-*.json")
	if err != nil {
		return fmt.Errorf("创建 sing-box check 临时文件: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if err := tempFile.Chmod(0600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("设置 sing-box check 临时文件权限: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("写入 sing-box check 临时文件: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("关闭 sing-box check 临时文件: %w", err)
	}

	command := exec.CommandContext(ctx, binary, "check", "-c", tempPath)
	output, err := command.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("sing-box check failed for instance %s: %w: %s", instance, err, string(output))
		}
		return fmt.Errorf("sing-box check failed for instance %s: %w", instance, err)
	}
	return nil
}

// NoopServiceManager 是不触发系统服务副作用的服务管理器。
type NoopServiceManager struct{}

// Start 空实现实例服务启动。
func (NoopServiceManager) Start(ctx context.Context, services []string) error {
	return nil
}

// Restart 空实现实例服务重启。
func (NoopServiceManager) Restart(ctx context.Context, services []string) error {
	return nil
}

// CheckPlan 对 plan 中所有期望配置执行 checker，过程发生在 apply 之前。
func CheckPlan(ctx context.Context, plan *Plan, checker ConfigChecker) error {
	if plan == nil {
		return fmt.Errorf("plan 不能为空")
	}
	if checker == nil {
		return fmt.Errorf("config checker 不能为空，显式跳过检查请注入 NoopConfigChecker")
	}
	for _, desired := range plan.DesiredFiles {
		if err := checker.Check(ctx, desired.Instance, desired.Data); err != nil {
			return err
		}
	}
	return nil
}

// Start 检查配置、应用 runtime plan，并调用服务管理器 Start。
func Start(ctx context.Context, plan *Plan, checker ConfigChecker, manager ServiceManager, clock Clock) (*ApplyResult, error) {
	if manager == nil {
		manager = NoopServiceManager{}
	}
	if err := CheckPlan(ctx, plan, checker); err != nil {
		return nil, err
	}
	result, err := ApplyPlan(plan, clock)
	if err != nil {
		return nil, err
	}
	if err := manager.Start(ctx, plan.Services()); err != nil {
		return nil, err
	}
	return result, nil
}

// Restart 检查配置、应用 runtime plan，并调用服务管理器 Restart。
func Restart(ctx context.Context, plan *Plan, checker ConfigChecker, manager ServiceManager, clock Clock) (*ApplyResult, error) {
	if manager == nil {
		manager = NoopServiceManager{}
	}
	if err := CheckPlan(ctx, plan, checker); err != nil {
		return nil, err
	}
	result, err := ApplyPlan(plan, clock)
	if err != nil {
		return nil, err
	}
	if err := manager.Restart(ctx, plan.Services()); err != nil {
		return nil, err
	}
	return result, nil
}
