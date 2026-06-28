package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// Install 写入目标 instance 的 systemd unit 或 launchd plist，且不启动服务。
func (m *Manager) Install(ctx context.Context, baseDir string, global domain.GlobalConfig, instances []domain.Instance, target string) error {
	targets, err := SelectInstances(instances, target)
	if err != nil {
		return err
	}
	for _, instance := range targets {
		switch m.kind {
		case KindSystemd:
			path := m.UnitPath(instance.Name)
			data := RenderSystemdUnit(baseDir, global.Paths.Bin, global.Paths.Generated, global.Paths.Traffic, global.Paths.Logs, instance.Name)
			if err := WriteFileAtomic(path, data, systemdUnitMode); err != nil {
				return fmt.Errorf("安装 systemd unit %s: %w", path, err)
			}
		case KindLaunchd:
			path := m.PlistPath(instance.Name)
			data := RenderLaunchdPlist(baseDir, global.Paths.Bin, global.Paths.Generated, global.Paths.Logs, instance.Name)
			if err := WriteFileAtomic(path, data, launchdMode); err != nil {
				return fmt.Errorf("安装 launchd plist %s: %w", path, err)
			}
		default:
			return fmt.Errorf("不支持的 service-manager %q", m.kind)
		}
	}
	if m.kind == KindSystemd && len(targets) > 0 {
		if _, err := m.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("执行 systemctl daemon-reload: %w", err)
		}
	}
	return nil
}

// Uninstall 删除目标 instance 的服务文件，且不启动服务。
func (m *Manager) Uninstall(ctx context.Context, instances []domain.Instance, target string) error {
	targets, err := SelectInstances(instances, target)
	if err != nil {
		return err
	}
	for _, instance := range targets {
		var path string
		switch m.kind {
		case KindSystemd:
			path = m.UnitPath(instance.Name)
		case KindLaunchd:
			path = m.PlistPath(instance.Name)
		default:
			return fmt.Errorf("不支持的 service-manager %q", m.kind)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("卸载服务文件 %s: %w", path, err)
		}
	}
	if m.kind == KindSystemd && len(targets) > 0 {
		if _, err := m.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("执行 systemctl daemon-reload: %w", err)
		}
	}
	return nil
}

// InstallSubscription 写入 sboxsub 的 systemd unit 或 launchd plist，且不启动服务。
func (m *Manager) InstallSubscription(ctx context.Context, baseDir string, binary string) error {
	switch m.kind {
	case KindSystemd:
		path := filepath.Join(m.unitDir, SubscriptionSystemdServiceName())
		data := RenderSubscriptionSystemdUnit(baseDir, binary)
		if err := WriteFileAtomic(path, data, systemdUnitMode); err != nil {
			return fmt.Errorf("安装 systemd unit %s: %w", path, err)
		}
		if _, err := m.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("执行 systemctl daemon-reload: %w", err)
		}
		return nil
	case KindLaunchd:
		path := filepath.Join(m.launchAgentDir, SubscriptionLaunchdLabel()+".plist")
		data := RenderSubscriptionLaunchdPlist(baseDir, binary)
		if err := WriteFileAtomic(path, data, launchdMode); err != nil {
			return fmt.Errorf("安装 launchd plist %s: %w", path, err)
		}
		return nil
	default:
		return fmt.Errorf("不支持的 service-manager %q", m.kind)
	}
}

// UninstallSubscription 删除 sboxsub 的服务文件，且不停止服务。
func (m *Manager) UninstallSubscription(ctx context.Context) error {
	var path string
	switch m.kind {
	case KindSystemd:
		path = filepath.Join(m.unitDir, SubscriptionSystemdServiceName())
	case KindLaunchd:
		path = filepath.Join(m.launchAgentDir, SubscriptionLaunchdLabel()+".plist")
	default:
		return fmt.Errorf("不支持的 service-manager %q", m.kind)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("卸载服务文件 %s: %w", path, err)
	}
	if m.kind == KindSystemd {
		if _, err := m.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("执行 systemctl daemon-reload: %w", err)
		}
	}
	return nil
}

// SelectInstances 按 CLI target 规则选择 instance，空 target 只选择 enabled。
func SelectInstances(instances []domain.Instance, target string) ([]domain.Instance, error) {
	if target == "" {
		selected := make([]domain.Instance, 0, len(instances))
		for _, instance := range instances {
			if instance.Enabled {
				selected = append(selected, instance)
			}
		}
		return selected, nil
	}
	for _, instance := range instances {
		if instance.Name == target {
			return []domain.Instance{instance}, nil
		}
	}
	return nil, fmt.Errorf("instance %q 不存在", target)
}

// ServicesForInstances 返回当前管理器语义下的服务标识。
func (m *Manager) ServicesForInstances(instances []domain.Instance) []string {
	result := make([]string, 0, len(instances))
	for _, instance := range instances {
		result = append(result, ServiceNameForKind(m.kind, instance.Name))
	}
	return stableServices(result)
}
