package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	KindAuto    = "auto"
	KindSystemd = "systemd"
	KindLaunchd = "launchd"

	systemdUnitMode os.FileMode = 0644
	launchdMode     os.FileMode = 0644

	// SubscriptionInstanceName 是 service.Manager 内部复用 launchd 逻辑时的订阅服务名称。
	SubscriptionInstanceName = "sboxsub"
)

// Runner 表示不经 shell 的外部命令执行器。
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// StreamRunner 表示可把长驻命令输出实时转发到当前终端的执行器。
type StreamRunner interface {
	Stream(ctx context.Context, name string, args ...string) error
}

// CommandRunner 使用 exec.CommandContext 和参数数组执行外部命令。
type CommandRunner struct{}

// Run 执行命令并返回合并后的 stdout/stderr。
func (CommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, nil
}

// Stream 执行命令并把 stdin/stdout/stderr 直接连接到当前进程。
func (CommandRunner) Stream(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// Options 描述服务管理器运行参数。
type Options struct {
	Kind           string
	UnitDir        string
	LaunchAgentDir string
	Runner         Runner
}

// Manager 负责 systemd/launchd 服务文件与生命周期动作。
type Manager struct {
	kind           string
	unitDir        string
	launchAgentDir string
	runner         Runner
}

// NewManager 创建可注入 runner 和服务文件目录的管理器。
func NewManager(options Options) (*Manager, error) {
	kind, err := ResolveKind(options.Kind)
	if err != nil {
		return nil, err
	}
	if options.Runner == nil {
		options.Runner = CommandRunner{}
	}
	if options.UnitDir == "" {
		options.UnitDir = "/etc/systemd/system"
	}
	if options.LaunchAgentDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve launchd plist directory: %w", err)
		}
		options.LaunchAgentDir = filepath.Join(home, "Library", "LaunchAgents")
	}
	return &Manager{
		kind:           kind,
		unitDir:        options.UnitDir,
		launchAgentDir: options.LaunchAgentDir,
		runner:         options.Runner,
	}, nil
}

// ResolveKind 根据 auto/systemd/launchd 和当前平台得到具体服务管理器。
func ResolveKind(kind string) (string, error) {
	switch kind {
	case "", KindAuto:
		switch runtime.GOOS {
		case "linux":
			return KindSystemd, nil
		case "darwin":
			return KindLaunchd, nil
		default:
			return "", fmt.Errorf("current platform %s does not support auto service-manager", runtime.GOOS)
		}
	case KindSystemd, KindLaunchd:
		return kind, nil
	default:
		return "", fmt.Errorf("unsupported service-manager %q", kind)
	}
}

// Kind 返回解析后的服务管理器类型。
func (m *Manager) Kind() string {
	if m == nil {
		return ""
	}
	return m.kind
}

// UnitPath 返回 systemd instance unit 路径。
func (m *Manager) UnitPath(instance string) string {
	return filepath.Join(m.unitDir, SystemdServiceName(instance))
}

// TemplateUnitPath 返回 systemd instance 模板 unit 路径。
func (m *Manager) TemplateUnitPath() string {
	return filepath.Join(m.unitDir, SystemdTemplateServiceName())
}

// PlistPath 返回 launchd instance plist 路径。
func (m *Manager) PlistPath(instance string) string {
	return filepath.Join(m.launchAgentDir, LaunchdLabel(instance)+".plist")
}

// SystemdTemplateServiceName 返回 instance 的 systemd 模板服务名。
func SystemdTemplateServiceName() string {
	return "sbox@.service"
}

// SystemdServiceName 返回 instance 的 systemd 服务名。
func SystemdServiceName(instance string) string {
	return "sbox@" + instance + ".service"
}

// SubscriptionSystemdServiceName 返回 sboxsub 的 systemd 服务名。
func SubscriptionSystemdServiceName() string {
	return "sboxsub.service"
}

// LaunchdLabel 返回 instance 的 launchd label。
func LaunchdLabel(instance string) string {
	return "com.sbox-manager." + instance
}

// SubscriptionLaunchdLabel 返回 sboxsub 的 launchd label。
func SubscriptionLaunchdLabel() string {
	return LaunchdLabel(SubscriptionInstanceName)
}

// InstanceFromServiceName 从 systemd 服务名或 launchd label 还原 instance 名称。
func InstanceFromServiceName(serviceName string) string {
	if strings.HasPrefix(serviceName, "sbox@") && strings.HasSuffix(serviceName, ".service") {
		return strings.TrimSuffix(strings.TrimPrefix(serviceName, "sbox@"), ".service")
	}
	if strings.HasPrefix(serviceName, "com.sbox-manager.") {
		return strings.TrimPrefix(serviceName, "com.sbox-manager.")
	}
	return serviceName
}

// ServiceNameForKind 根据目标服务管理器返回对应服务标识。
func ServiceNameForKind(kind string, instance string) string {
	if kind == KindLaunchd {
		return LaunchdLabel(instance)
	}
	return SystemdServiceName(instance)
}

// SubscriptionServiceNameForKind 根据目标服务管理器返回 sboxsub 服务标识。
func SubscriptionServiceNameForKind(kind string) string {
	if kind == KindLaunchd {
		return SubscriptionLaunchdLabel()
	}
	return SubscriptionSystemdServiceName()
}

// WriteFileAtomic 以固定权限原子写入服务文件。
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file %s: %w", dir, err)
	}
	tempPath := tempFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tempFile.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := tempFile.Chmod(mode); err != nil {
		return fmt.Errorf("set temp file permissions %s: %w", tempPath, err)
	}
	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("write temp file %s: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		closed = true
		return fmt.Errorf("close temp file %s: %w", tempPath, err)
	}
	closed = true
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace file %s: %w", path, err)
	}
	return nil
}

// RenderSystemdTemplateUnit 生成 instance systemd 模板 unit 内容。
func RenderSystemdTemplateUnit(baseDir string, binDir string, generatedDir string, trafficDir string, logsDir string) []byte {
	generatedPath := filepath.Join(generatedDir, "sing-box", "%i.json")
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "[Unit]\n")
	fmt.Fprintf(&buffer, "Description=sbox-manager instance %%i\n")
	fmt.Fprintf(&buffer, "After=network-online.target\n")
	fmt.Fprintf(&buffer, "Wants=network-online.target\n\n")
	fmt.Fprintf(&buffer, "[Service]\n")
	fmt.Fprintf(&buffer, "Type=simple\n")
	fmt.Fprintf(&buffer, "User=sbox\n")
	fmt.Fprintf(&buffer, "Group=sbox\n")
	fmt.Fprintf(&buffer, "ExecStart=%s run -c %s\n", filepath.Join(binDir, "sing-box"), generatedPath)
	fmt.Fprintf(&buffer, "WorkingDirectory=%s\n", baseDir)
	fmt.Fprintf(&buffer, "Restart=on-failure\n")
	fmt.Fprintf(&buffer, "NoNewPrivileges=true\n")
	fmt.Fprintf(&buffer, "ProtectSystem=strict\n")
	fmt.Fprintf(&buffer, "ProtectHome=true\n")
	fmt.Fprintf(&buffer, "PrivateTmp=true\n")
	fmt.Fprintf(&buffer, "ReadWritePaths=%s %s %s\n", filepath.Clean(filepath.Dir(generatedDir)), trafficDir, logsDir)
	fmt.Fprintf(&buffer, "SyslogIdentifier=sbox-%%i\n\n")
	fmt.Fprintf(&buffer, "[Install]\n")
	fmt.Fprintf(&buffer, "WantedBy=multi-user.target\n")
	return buffer.Bytes()
}

// RenderLaunchdPlist 生成 instance launchd plist 内容。
func RenderLaunchdPlist(baseDir string, binDir string, generatedDir string, logsDir string, instance string) []byte {
	label := LaunchdLabel(instance)
	arguments := []string{
		filepath.Join(binDir, "sing-box"),
		"run",
		"-c",
		filepath.Join(generatedDir, "sing-box", instance+".json"),
	}
	var buffer bytes.Buffer
	buffer.WriteString(xml.Header)
	buffer.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	buffer.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	writePlistString(&buffer, "Label", label)
	buffer.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, argument := range arguments {
		buffer.WriteString("\t\t<string>")
		xml.EscapeText(&buffer, []byte(argument))
		buffer.WriteString("</string>\n")
	}
	buffer.WriteString("\t</array>\n")
	writePlistString(&buffer, "WorkingDirectory", baseDir)
	buffer.WriteString("\t<key>RunAtLoad</key>\n\t<false/>\n")
	writePlistString(&buffer, "StandardOutPath", filepath.Join(logsDir, label+".out.log"))
	writePlistString(&buffer, "StandardErrorPath", filepath.Join(logsDir, label+".err.log"))
	buffer.WriteString("</dict>\n</plist>\n")
	return buffer.Bytes()
}

// RenderSubscriptionSystemdUnit 生成 sboxsub systemd unit 内容。
func RenderSubscriptionSystemdUnit(baseDir string, binary string) []byte {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "[Unit]\n")
	fmt.Fprintf(&buffer, "Description=sbox-manager subscription service\n")
	fmt.Fprintf(&buffer, "After=network-online.target\n")
	fmt.Fprintf(&buffer, "Wants=network-online.target\n\n")
	fmt.Fprintf(&buffer, "[Service]\n")
	fmt.Fprintf(&buffer, "Type=simple\n")
	fmt.Fprintf(&buffer, "User=sbox\n")
	fmt.Fprintf(&buffer, "Group=sbox\n")
	fmt.Fprintf(&buffer, "ExecStart=%s --base-dir %s serve\n", binary, baseDir)
	fmt.Fprintf(&buffer, "WorkingDirectory=%s\n", baseDir)
	fmt.Fprintf(&buffer, "Restart=on-failure\n")
	fmt.Fprintf(&buffer, "NoNewPrivileges=true\n")
	fmt.Fprintf(&buffer, "ProtectSystem=strict\n")
	fmt.Fprintf(&buffer, "ProtectHome=true\n")
	fmt.Fprintf(&buffer, "PrivateTmp=true\n")
	fmt.Fprintf(&buffer, "ReadWritePaths=%s\n", baseDir)
	fmt.Fprintf(&buffer, "SyslogIdentifier=sboxsub\n\n")
	fmt.Fprintf(&buffer, "[Install]\n")
	fmt.Fprintf(&buffer, "WantedBy=multi-user.target\n")
	return buffer.Bytes()
}

// RenderSubscriptionLaunchdPlist 生成 sboxsub launchd plist 内容。
func RenderSubscriptionLaunchdPlist(baseDir string, binary string) []byte {
	label := SubscriptionLaunchdLabel()
	arguments := []string{
		binary,
		"--base-dir",
		baseDir,
		"serve",
	}
	var buffer bytes.Buffer
	buffer.WriteString(xml.Header)
	buffer.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	buffer.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	writePlistString(&buffer, "Label", label)
	buffer.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, argument := range arguments {
		buffer.WriteString("\t\t<string>")
		xml.EscapeText(&buffer, []byte(argument))
		buffer.WriteString("</string>\n")
	}
	buffer.WriteString("\t</array>\n")
	writePlistString(&buffer, "WorkingDirectory", baseDir)
	buffer.WriteString("\t<key>RunAtLoad</key>\n\t<false/>\n")
	writePlistString(&buffer, "StandardOutPath", filepath.Join(baseDir, "logs", "sboxsub.out.log"))
	writePlistString(&buffer, "StandardErrorPath", filepath.Join(baseDir, "logs", "sboxsub.err.log"))
	buffer.WriteString("</dict>\n</plist>\n")
	return buffer.Bytes()
}

func writePlistString(buffer *bytes.Buffer, key string, value string) {
	buffer.WriteString("\t<key>")
	xml.EscapeText(buffer, []byte(key))
	buffer.WriteString("</key>\n\t<string>")
	xml.EscapeText(buffer, []byte(value))
	buffer.WriteString("</string>\n")
}

// Result 表示一次服务动作输出。
type Result struct {
	Service string
	Output  []byte
}

// Start 按 runtime.ServiceManager 契约启动服务。
func (m *Manager) Start(ctx context.Context, services []string) error {
	_, err := m.Run(ctx, "start", services, false)
	return err
}

// Restart 按 runtime.ServiceManager 契约重启服务。
func (m *Manager) Restart(ctx context.Context, services []string) error {
	_, err := m.Run(ctx, "restart", services, false)
	return err
}

// IsRunning 判断服务当前是否处于运行状态，供配置变更后按需重启。
func (m *Manager) IsRunning(ctx context.Context, serviceName string) (bool, error) {
	if m == nil {
		return false, fmt.Errorf("service manager cannot be empty")
	}
	switch m.kind {
	case KindSystemd:
		return m.isSystemdRunning(ctx, serviceName)
	case KindLaunchd:
		return m.isLaunchdRunning(ctx, serviceName)
	default:
		return false, fmt.Errorf("unsupported service-manager %q", m.kind)
	}
}

// isSystemdRunning 通过 systemctl is-active 判断 systemd 服务是否 active。
func (m *Manager) isSystemdRunning(ctx context.Context, serviceName string) (bool, error) {
	output, err := m.runner.Run(ctx, "systemctl", "is-active", serviceName)
	state := strings.TrimSpace(string(output))
	if state == "active" {
		return true, nil
	}
	if state != "" || err == nil || isServiceNotLoaded(output, err) {
		return false, nil
	}
	return false, fmt.Errorf("check service running status %s: %w", serviceName, err)
}

// isLaunchdRunning 通过 launchctl print 判断 launchd 服务是否 running。
func (m *Manager) isLaunchdRunning(ctx context.Context, serviceName string) (bool, error) {
	instance := InstanceFromServiceName(serviceName)
	label := LaunchdLabel(instance)
	target := launchdDomain() + "/" + label
	output, err := m.runner.Run(ctx, "launchctl", "print", target)
	if err != nil {
		if isServiceNotLoaded(output, err) {
			return false, nil
		}
		return false, fmt.Errorf("check service running status %s: %w", serviceName, err)
	}
	text := strings.ToLower(string(output))
	return strings.Contains(text, "state = running") || strings.Contains(text, "\npid = "), nil
}

// Run 执行 start/stop/restart/status/logs/enable/disable。
func (m *Manager) Run(ctx context.Context, action string, services []string, follow bool) ([]Result, error) {
	if m == nil {
		return nil, fmt.Errorf("service manager cannot be empty")
	}
	services = stableServices(services)
	results := make([]Result, 0, len(services))
	for _, serviceName := range services {
		output, err := m.runOne(ctx, action, serviceName, follow)
		if err != nil {
			return results, err
		}
		results = append(results, Result{Service: serviceName, Output: output})
	}
	return results, nil
}

func (m *Manager) runOne(ctx context.Context, action string, serviceName string, follow bool) ([]byte, error) {
	switch m.kind {
	case KindSystemd:
		return m.runSystemd(ctx, action, serviceName, follow)
	case KindLaunchd:
		return m.runLaunchd(ctx, action, serviceName, follow)
	default:
		return nil, fmt.Errorf("unsupported service-manager %q", m.kind)
	}
}

// runStreamOrCapture 优先流式执行 follow 日志命令，测试 runner 不支持时回退为捕获输出。
func (m *Manager) runStreamOrCapture(ctx context.Context, name string, args ...string) ([]byte, error) {
	if streamer, ok := m.runner.(StreamRunner); ok {
		return nil, streamer.Stream(ctx, name, args...)
	}
	return m.runner.Run(ctx, name, args...)
}

func (m *Manager) runSystemd(ctx context.Context, action string, serviceName string, follow bool) ([]byte, error) {
	switch action {
	case "start", "stop", "restart", "enable", "disable":
		return m.runner.Run(ctx, "systemctl", action, serviceName)
	case "status":
		return m.runner.Run(ctx, "systemctl", "status", "--no-pager", serviceName)
	case "logs", "log":
		args := []string{"-u", serviceName, "--no-pager", "-n", "200"}
		if follow {
			args = append(args, "-f")
			return m.runStreamOrCapture(ctx, "journalctl", args...)
		}
		return m.runner.Run(ctx, "journalctl", args...)
	default:
		return nil, fmt.Errorf("unsupported service action %q", action)
	}
}

func (m *Manager) runLaunchd(ctx context.Context, action string, serviceName string, follow bool) ([]byte, error) {
	instance := InstanceFromServiceName(serviceName)
	label := LaunchdLabel(instance)
	domain := launchdDomain()
	target := domain + "/" + label
	switch action {
	case "start":
		return m.bootstrapAndRun(ctx, instance, "launchctl", "kickstart", "-k", target)
	case "stop":
		return m.runner.Run(ctx, "launchctl", "bootout", target)
	case "restart":
		return m.bootstrapAndRun(ctx, instance, "launchctl", "kickstart", "-k", target)
	case "status":
		return m.runner.Run(ctx, "launchctl", "print", target)
	case "enable":
		return m.bootstrapAndRun(ctx, instance, "launchctl", "enable", target)
	case "disable":
		return m.runner.Run(ctx, "launchctl", "disable", target)
	case "logs", "log":
		processName := "sing-box"
		if instance == SubscriptionInstanceName {
			processName = "sboxsub"
		}
		predicate := fmt.Sprintf(`process == "%s" && eventMessage CONTAINS "%s"`, processName, label)
		args := []string{"show", "--style", "compact", "--predicate", predicate, "--last", "1h"}
		if follow {
			args[0] = "stream"
			args = args[:len(args)-2]
			return m.runStreamOrCapture(ctx, "log", args...)
		}
		return m.runner.Run(ctx, "log", args...)
	default:
		return nil, fmt.Errorf("unsupported service action %q", action)
	}
}

func (m *Manager) bootstrapAndRun(ctx context.Context, instance string, name string, args ...string) ([]byte, error) {
	domain := launchdDomain()
	plistPath := m.PlistPath(instance)
	bootstrapOutput, err := m.runner.Run(ctx, "launchctl", "bootstrap", domain, plistPath)
	if err != nil && !isLaunchdAlreadyBootstrapped(bootstrapOutput, err) {
		return bootstrapOutput, err
	}
	actionOutput, err := m.runner.Run(ctx, name, args...)
	if len(bootstrapOutput) == 0 {
		return actionOutput, err
	}
	combined := append([]byte{}, bootstrapOutput...)
	combined = append(combined, actionOutput...)
	return combined, err
}

func isLaunchdAlreadyBootstrapped(output []byte, err error) bool {
	text := strings.ToLower(string(output) + " " + err.Error())
	return strings.Contains(text, "already bootstrapped") || strings.Contains(text, "service already loaded") || strings.Contains(text, "bootstrap failed: 5")
}

func launchdDomain() string {
	uid := os.Getuid()
	if uid < 0 {
		return "gui/0"
	}
	return fmt.Sprintf("gui/%d", uid)
}

func stableServices(services []string) []string {
	if len(services) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(services))
	for _, serviceName := range services {
		if strings.TrimSpace(serviceName) == "" {
			continue
		}
		if _, exists := seen[serviceName]; exists {
			continue
		}
		seen[serviceName] = struct{}{}
		result = append(result, serviceName)
	}
	sort.Strings(result)
	return result
}
