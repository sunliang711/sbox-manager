package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	trafficPeriodHourly  = "hourly"
	trafficPeriodDaily   = "daily"
	trafficPeriodMonthly = "monthly"
)

// TrafficPeriods 返回内置 traffic timer 周期列表。
func TrafficPeriods() []string {
	return []string{trafficPeriodHourly, trafficPeriodDaily, trafficPeriodMonthly}
}

// TrafficSystemdServiceName 返回 traffic oneshot service 名称。
func TrafficSystemdServiceName(period string) string {
	return "sbox-traffic-" + period + ".service"
}

// TrafficSystemdTimerName 返回 traffic systemd timer 名称。
func TrafficSystemdTimerName(period string) string {
	return "sbox-traffic-" + period + ".timer"
}

// TrafficLaunchdLabel 返回 traffic launchd label。
func TrafficLaunchdLabel(period string) string {
	return "com.sbox-manager.traffic." + period
}

// RenderTrafficSystemdService 生成 traffic oneshot systemd service。
func RenderTrafficSystemdService(baseDir string, trafficDir string, logsDir string, binary string, period string) []byte {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "[Unit]\n")
	fmt.Fprintf(&buffer, "Description=sbox-manager traffic %s collector\n", period)
	fmt.Fprintf(&buffer, "After=network-online.target\n")
	fmt.Fprintf(&buffer, "Wants=network-online.target\n\n")
	fmt.Fprintf(&buffer, "[Service]\n")
	fmt.Fprintf(&buffer, "Type=oneshot\n")
	fmt.Fprintf(&buffer, "User=sbox\n")
	fmt.Fprintf(&buffer, "Group=sbox\n")
	fmt.Fprintf(&buffer, "ExecStart=%s --base-dir %s traffic collect %s --instance ALL\n", binary, baseDir, period)
	fmt.Fprintf(&buffer, "WorkingDirectory=%s\n", baseDir)
	fmt.Fprintf(&buffer, "NoNewPrivileges=true\n")
	fmt.Fprintf(&buffer, "ProtectSystem=strict\n")
	fmt.Fprintf(&buffer, "ProtectHome=true\n")
	fmt.Fprintf(&buffer, "PrivateTmp=true\n")
	fmt.Fprintf(&buffer, "ReadWritePaths=%s %s\n", trafficDir, logsDir)
	fmt.Fprintf(&buffer, "SyslogIdentifier=sbox-traffic-%s\n", period)
	return buffer.Bytes()
}

// RenderTrafficSystemdTimer 生成 traffic systemd timer。
func RenderTrafficSystemdTimer(period string) []byte {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "[Unit]\n")
	fmt.Fprintf(&buffer, "Description=sbox-manager traffic %s timer\n\n", period)
	fmt.Fprintf(&buffer, "[Timer]\n")
	fmt.Fprintf(&buffer, "OnCalendar=%s\n", trafficOnCalendar(period))
	fmt.Fprintf(&buffer, "Persistent=true\n")
	fmt.Fprintf(&buffer, "AccuracySec=1min\n\n")
	fmt.Fprintf(&buffer, "[Install]\n")
	fmt.Fprintf(&buffer, "WantedBy=timers.target\n")
	return buffer.Bytes()
}

// RenderTrafficLaunchdPlist 生成 traffic launchd plist。
func RenderTrafficLaunchdPlist(baseDir string, logsDir string, binary string, period string) []byte {
	label := TrafficLaunchdLabel(period)
	arguments := []string{
		binary,
		"--base-dir",
		baseDir,
		"traffic",
		"collect",
		period,
		"--instance",
		"ALL",
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
	buffer.WriteString("\t<key>StartCalendarInterval</key>\n")
	writeTrafficCalendarInterval(&buffer, period)
	buffer.WriteString("\t<key>RunAtLoad</key>\n\t<false/>\n")
	writePlistString(&buffer, "StandardOutPath", filepath.Join(logsDir, label+".out.log"))
	writePlistString(&buffer, "StandardErrorPath", filepath.Join(logsDir, label+".err.log"))
	buffer.WriteString("</dict>\n</plist>\n")
	return buffer.Bytes()
}

// InstallTrafficTimers 写入 traffic timer 服务文件，且不启用不启动。
func (m *Manager) InstallTrafficTimers(ctx context.Context, baseDir string, trafficDir string, logsDir string, binary string) error {
	switch m.kind {
	case KindSystemd:
		if err := m.prepareSystemdServiceEnvironment(ctx, baseDir); err != nil {
			return err
		}
		for _, period := range TrafficPeriods() {
			servicePath := filepath.Join(m.unitDir, TrafficSystemdServiceName(period))
			if err := WriteFileAtomic(servicePath, RenderTrafficSystemdService(baseDir, trafficDir, logsDir, binary, period), systemdUnitMode); err != nil {
				return fmt.Errorf("安装 traffic systemd service %s: %w", servicePath, err)
			}
			timerPath := filepath.Join(m.unitDir, TrafficSystemdTimerName(period))
			if err := WriteFileAtomic(timerPath, RenderTrafficSystemdTimer(period), systemdUnitMode); err != nil {
				return fmt.Errorf("安装 traffic systemd timer %s: %w", timerPath, err)
			}
		}
		if _, err := m.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("执行 systemctl daemon-reload: %w", err)
		}
		return nil
	case KindLaunchd:
		for _, period := range TrafficPeriods() {
			path := filepath.Join(m.launchAgentDir, TrafficLaunchdLabel(period)+".plist")
			if err := WriteFileAtomic(path, RenderTrafficLaunchdPlist(baseDir, logsDir, binary, period), launchdMode); err != nil {
				return fmt.Errorf("安装 traffic launchd plist %s: %w", path, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("不支持的 service-manager %q", m.kind)
	}
}

// UninstallTrafficTimers 停用并删除 traffic timer 服务文件。
func (m *Manager) UninstallTrafficTimers(ctx context.Context) error {
	switch m.kind {
	case KindSystemd:
		for _, period := range TrafficPeriods() {
			output, err := m.runner.Run(ctx, "systemctl", "disable", "--now", TrafficSystemdTimerName(period))
			if err != nil && !isServiceNotLoaded(output, err) {
				return fmt.Errorf("停用 traffic timer %s: %w", period, err)
			}
			for _, path := range []string{
				filepath.Join(m.unitDir, TrafficSystemdServiceName(period)),
				filepath.Join(m.unitDir, TrafficSystemdTimerName(period)),
			} {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("删除 traffic 服务文件 %s: %w", path, err)
				}
			}
		}
		if _, err := m.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("执行 systemctl daemon-reload: %w", err)
		}
		return nil
	case KindLaunchd:
		domain := launchdDomain()
		for _, period := range TrafficPeriods() {
			label := TrafficLaunchdLabel(period)
			output, err := m.runner.Run(ctx, "launchctl", "bootout", domain+"/"+label)
			if err != nil && !isServiceNotLoaded(output, err) {
				return fmt.Errorf("卸载 traffic launchd %s: %w", label, err)
			}
			path := filepath.Join(m.launchAgentDir, label+".plist")
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("删除 traffic plist %s: %w", path, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("不支持的 service-manager %q", m.kind)
	}
}

// RunTrafficTimers 执行 traffic timer 的 enable/disable/status/logs 动作。
func (m *Manager) RunTrafficTimers(ctx context.Context, action string, follow bool) ([]Result, error) {
	results := make([]Result, 0, len(TrafficPeriods()))
	for _, period := range TrafficPeriods() {
		output, err := m.runTrafficTimer(ctx, action, period, follow)
		if err != nil {
			return results, err
		}
		results = append(results, Result{Service: trafficServiceForKind(m.kind, period), Output: output})
	}
	return results, nil
}

// runTrafficTimer 执行单个 traffic timer 动作。
func (m *Manager) runTrafficTimer(ctx context.Context, action string, period string, follow bool) ([]byte, error) {
	switch m.kind {
	case KindSystemd:
		timerName := TrafficSystemdTimerName(period)
		switch action {
		case "enable":
			return m.runner.Run(ctx, "systemctl", "enable", "--now", timerName)
		case "disable":
			return m.runner.Run(ctx, "systemctl", "disable", "--now", timerName)
		case "status":
			return m.runner.Run(ctx, "systemctl", "status", "--no-pager", timerName)
		case "logs", "log":
			args := []string{"-u", timerName, "--no-pager", "-n", "200"}
			if follow {
				args = append(args, "-f")
			}
			return m.runner.Run(ctx, "journalctl", args...)
		default:
			return nil, fmt.Errorf("不支持的 traffic timer 动作 %q", action)
		}
	case KindLaunchd:
		label := TrafficLaunchdLabel(period)
		target := launchdDomain() + "/" + label
		switch action {
		case "enable":
			bootstrapOutput, err := m.runner.Run(ctx, "launchctl", "bootstrap", launchdDomain(), filepath.Join(m.launchAgentDir, label+".plist"))
			if err != nil && !isLaunchdAlreadyBootstrapped(bootstrapOutput, err) {
				return bootstrapOutput, err
			}
			return m.runner.Run(ctx, "launchctl", "enable", target)
		case "disable":
			disableOutput, err := m.runner.Run(ctx, "launchctl", "disable", target)
			if err != nil && !isServiceNotLoaded(disableOutput, err) {
				return disableOutput, err
			}
			bootoutOutput, err := m.runner.Run(ctx, "launchctl", "bootout", target)
			if err != nil && !isServiceNotLoaded(bootoutOutput, err) {
				return append(disableOutput, bootoutOutput...), err
			}
			return append(disableOutput, bootoutOutput...), nil
		case "status":
			return m.runner.Run(ctx, "launchctl", "print", target)
		case "logs", "log":
			predicate := fmt.Sprintf(`process == "sboxctl" && eventMessage CONTAINS "%s"`, label)
			args := []string{"show", "--style", "compact", "--predicate", predicate, "--last", "1h"}
			if follow {
				args[0] = "stream"
				args = args[:len(args)-2]
			}
			return m.runner.Run(ctx, "log", args...)
		default:
			return nil, fmt.Errorf("不支持的 traffic timer 动作 %q", action)
		}
	default:
		return nil, fmt.Errorf("不支持的 service-manager %q", m.kind)
	}
}

// trafficOnCalendar 返回 systemd OnCalendar 表达式。
func trafficOnCalendar(period string) string {
	switch period {
	case trafficPeriodDaily:
		return "*-*-* 00:10:00"
	case trafficPeriodMonthly:
		return "*-*-01 00:30:00"
	default:
		return "*-*-* *:00:00"
	}
}

// writeTrafficCalendarInterval 写入 launchd StartCalendarInterval。
func writeTrafficCalendarInterval(buffer *bytes.Buffer, period string) {
	buffer.WriteString("\t<dict>\n")
	switch period {
	case trafficPeriodDaily:
		buffer.WriteString("\t\t<key>Hour</key>\n\t\t<integer>0</integer>\n")
		buffer.WriteString("\t\t<key>Minute</key>\n\t\t<integer>10</integer>\n")
	case trafficPeriodMonthly:
		buffer.WriteString("\t\t<key>Day</key>\n\t\t<integer>1</integer>\n")
		buffer.WriteString("\t\t<key>Hour</key>\n\t\t<integer>0</integer>\n")
		buffer.WriteString("\t\t<key>Minute</key>\n\t\t<integer>30</integer>\n")
	default:
		buffer.WriteString("\t\t<key>Minute</key>\n\t\t<integer>0</integer>\n")
	}
	buffer.WriteString("\t</dict>\n")
}

// trafficServiceForKind 返回 service.Manager 输出使用的 traffic 服务标识。
func trafficServiceForKind(kind string, period string) string {
	if kind == KindLaunchd {
		return TrafficLaunchdLabel(period)
	}
	return TrafficSystemdTimerName(period)
}

func isServiceNotLoaded(output []byte, err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(string(output) + " " + err.Error())
	return strings.Contains(text, "not loaded") ||
		strings.Contains(text, "not-found") ||
		strings.Contains(text, "not found") ||
		strings.Contains(text, "not bootstrapped") ||
		strings.Contains(text, "no such process") ||
		strings.Contains(text, "could not find service")
}
