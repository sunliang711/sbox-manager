package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

const (
	systemdServiceUser  = "sbox"
	systemdServiceGroup = "sbox"
)

// Install 写入 systemd 模板 unit 或目标 instance 的 launchd plist，且不启动服务。
func (m *Manager) Install(ctx context.Context, baseDir string, global domain.GlobalConfig, instances []domain.Instance, target string) error {
	targets, err := SelectInstances(instances, target)
	if err != nil {
		return err
	}
	if m.kind == KindSystemd && len(targets) > 0 {
		if err := m.prepareSystemdServiceEnvironment(ctx, baseDir); err != nil {
			return err
		}
		path := m.TemplateUnitPath()
		data := RenderSystemdTemplateUnit(baseDir, global.Paths.Bin, global.Paths.Generated, global.Paths.Traffic, global.Paths.Logs)
		if err := WriteFileAtomic(path, data, systemdUnitMode); err != nil {
			return fmt.Errorf("安装 systemd unit %s: %w", path, err)
		}
		for _, instance := range targets {
			if _, err := removeFileIfExists(m.UnitPath(instance.Name)); err != nil {
				return fmt.Errorf("清理旧 systemd unit %s: %w", m.UnitPath(instance.Name), err)
			}
		}
		if _, err := m.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("执行 systemctl daemon-reload: %w", err)
		}
		return nil
	}
	for _, instance := range targets {
		switch m.kind {
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
	return nil
}

// Uninstall 删除目标 instance 的服务文件，且不启动服务。
func (m *Manager) Uninstall(ctx context.Context, instances []domain.Instance, target string) error {
	targets, err := SelectInstances(instances, target)
	if err != nil {
		return err
	}
	return m.UninstallInstances(ctx, targets)
}

// StopInstancesForUninstall 在 purge 删除服务文件前尽量停止实例服务，忽略未加载服务。
func (m *Manager) StopInstancesForUninstall(ctx context.Context, instances []domain.Instance) error {
	for _, instance := range instances {
		exists, err := m.instanceServiceFileExists(instance.Name)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		serviceName := ServiceNameForKind(m.kind, instance.Name)
		output, err := m.runOne(ctx, "stop", serviceName, false)
		if err != nil && !isServiceNotLoaded(output, err) {
			return fmt.Errorf("停止服务 %s: %w", serviceName, err)
		}
	}
	return nil
}

// UninstallInstances 删除给定实例集合的服务文件，且不启动服务。
func (m *Manager) UninstallInstances(ctx context.Context, instances []domain.Instance) error {
	if m.kind != KindSystemd && m.kind != KindLaunchd {
		return fmt.Errorf("不支持的 service-manager %q", m.kind)
	}
	removed := false
	seen := map[string]struct{}{}
	for _, instance := range instances {
		for _, path := range m.instanceServiceFilePaths(instance.Name) {
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			deleted, err := removeFileIfExists(path)
			if err != nil {
				return fmt.Errorf("卸载服务文件 %s: %w", path, err)
			}
			removed = removed || deleted
		}
	}
	if m.kind == KindSystemd && removed {
		if _, err := m.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("执行 systemctl daemon-reload: %w", err)
		}
	}
	return nil
}

// instanceServiceFilePaths 返回 instance 在当前服务管理器下可能存在的服务文件路径。
func (m *Manager) instanceServiceFilePaths(instance string) []string {
	switch m.kind {
	case KindSystemd:
		return []string{m.TemplateUnitPath(), m.UnitPath(instance)}
	case KindLaunchd:
		return []string{m.PlistPath(instance)}
	default:
		return nil
	}
}

// instanceServiceFileExists 判断 instance 在当前服务管理器下是否存在可管理服务文件。
func (m *Manager) instanceServiceFileExists(instance string) (bool, error) {
	for _, path := range m.instanceServiceFilePaths(instance) {
		exists, err := pathExists(path)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	if m.kind != KindSystemd && m.kind != KindLaunchd {
		return false, fmt.Errorf("不支持的 service-manager %q", m.kind)
	}
	return false, nil
}

// InstallSubscription 写入 sboxsub 的 systemd unit 或 launchd plist，且不启动服务。
func (m *Manager) InstallSubscription(ctx context.Context, baseDir string, binary string) error {
	switch m.kind {
	case KindSystemd:
		if err := m.prepareSystemdServiceEnvironment(ctx, baseDir); err != nil {
			return err
		}
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
	deleted, err := removeFileIfExists(path)
	if err != nil {
		return fmt.Errorf("卸载服务文件 %s: %w", path, err)
	}
	if m.kind == KindSystemd && deleted {
		if _, err := m.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("执行 systemctl daemon-reload: %w", err)
		}
	}
	return nil
}

// pathExists 使用 Lstat 判断路径是否存在，保留 symlink 自身的存在性语义。
func pathExists(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("检查路径 %s: %w", path, err)
	}
	return true, nil
}

// removeFileIfExists 删除文件或 symlink；路径不存在时视为已完成。
func removeFileIfExists(path string) (bool, error) {
	exists, err := pathExists(path)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}

// prepareSystemdServiceEnvironment 确保 systemd unit 使用的 sbox 用户和目录权限已就绪。
func (m *Manager) prepareSystemdServiceEnvironment(ctx context.Context, paths ...string) error {
	if err := m.ensureSystemdServiceUser(ctx); err != nil {
		return err
	}
	for _, item := range uniqueCleanPaths(paths) {
		if err := os.MkdirAll(item, 0750); err != nil {
			return fmt.Errorf("创建服务目录 %s: %w", item, err)
		}
		if _, err := m.runner.Run(ctx, "chown", "-R", systemdServiceUser+":"+systemdServiceGroup, item); err != nil {
			return fmt.Errorf("设置服务目录权限 %s: %w", item, err)
		}
	}
	return nil
}

// ensureSystemdServiceUser 确保 Linux systemd 服务运行所需的 sbox 用户和组存在。
func (m *Manager) ensureSystemdServiceUser(ctx context.Context) error {
	if _, err := m.runner.Run(ctx, "getent", "group", systemdServiceGroup); err != nil {
		if _, groupErr := m.runner.Run(ctx, "groupadd", "--system", systemdServiceGroup); groupErr != nil {
			return fmt.Errorf("创建服务组 %s: %w", systemdServiceGroup, groupErr)
		}
	}
	if _, err := m.runner.Run(ctx, "id", "-u", systemdServiceUser); err != nil {
		if _, userErr := m.runner.Run(ctx, "useradd", "--system", "--no-create-home", "--gid", systemdServiceGroup, "--shell", "/usr/sbin/nologin", systemdServiceUser); userErr != nil {
			return fmt.Errorf("创建服务用户 %s: %w", systemdServiceUser, userErr)
		}
	}
	return nil
}

// uniqueCleanPaths 返回去重后的非空路径列表，避免重复 chown 同一目录。
func uniqueCleanPaths(paths []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(paths))
	for _, item := range paths {
		if strings.TrimSpace(item) == "" {
			continue
		}
		cleaned := filepath.Clean(item)
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
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
