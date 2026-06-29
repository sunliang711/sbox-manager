package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// TestRenderSystemdTemplateUnitFields 验证 systemd 模板 unit 关键字段符合数据规格。
func TestRenderSystemdTemplateUnitFields(t *testing.T) {
	baseDir := t.TempDir()
	data := string(RenderSystemdTemplateUnit(baseDir, filepath.Join(baseDir, "bin"), filepath.Join(baseDir, "runtime", "generated"), filepath.Join(baseDir, "traffic"), filepath.Join(baseDir, "logs")))

	for _, want := range []string{
		"Description=sbox-manager instance %i",
		"User=sbox",
		"Group=sbox",
		"ExecStart=" + filepath.Join(baseDir, "bin", "sing-box") + " run -c " + filepath.Join(baseDir, "runtime", "generated", "sing-box", "%i.json"),
		"WorkingDirectory=" + baseDir,
		"Restart=on-failure",
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
		"ProtectHome=true",
		"PrivateTmp=true",
		"ReadWritePaths=" + filepath.Join(baseDir, "runtime") + " " + filepath.Join(baseDir, "traffic") + " " + filepath.Join(baseDir, "logs"),
		"SyslogIdentifier=sbox-%i",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("unit missing %q:\n%s", want, data)
		}
	}
}

// TestRenderLaunchdPlistFields 验证 launchd plist 关键字段符合数据规格。
func TestRenderLaunchdPlistFields(t *testing.T) {
	baseDir := t.TempDir()
	data := string(RenderLaunchdPlist(baseDir, filepath.Join(baseDir, "bin"), filepath.Join(baseDir, "runtime", "generated"), filepath.Join(baseDir, "logs"), "edge-us"))

	for _, want := range []string{
		"<string>com.sbox-manager.edge-us</string>",
		"<string>" + filepath.Join(baseDir, "bin", "sing-box") + "</string>",
		"<string>run</string>",
		"<string>-c</string>",
		"<string>" + filepath.Join(baseDir, "runtime", "generated", "sing-box", "edge-us.json") + "</string>",
		"<key>WorkingDirectory</key>",
		"<string>" + baseDir + "</string>",
		"<key>RunAtLoad</key>",
		"<false/>",
		"<string>" + filepath.Join(baseDir, "logs", "com.sbox-manager.edge-us.out.log") + "</string>",
		"<string>" + filepath.Join(baseDir, "logs", "com.sbox-manager.edge-us.err.log") + "</string>",
	} {
		if !strings.Contains(data, want) {
			t.Fatalf("plist missing %q:\n%s", want, data)
		}
	}
	if strings.Contains(data, "KeepAlive") {
		t.Fatalf("plist should not contain KeepAlive:\n%s", data)
	}
}

// TestRenderSubscriptionServiceFiles 验证 sboxsub 服务文件关键字段符合数据规格。
func TestRenderSubscriptionServiceFiles(t *testing.T) {
	baseDir := t.TempDir()
	binary := filepath.Join(baseDir, "bin", "sboxsub")
	unit := string(RenderSubscriptionSystemdUnit(baseDir, binary))
	for _, want := range []string{
		"User=sbox",
		"Group=sbox",
		"ExecStart=" + binary + " --base-dir " + baseDir + " serve",
		"WorkingDirectory=" + baseDir,
		"ReadWritePaths=" + baseDir,
		"SyslogIdentifier=sboxsub",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("subscription unit missing %q:\n%s", want, unit)
		}
	}

	plist := string(RenderSubscriptionLaunchdPlist(baseDir, binary))
	for _, want := range []string{
		"<string>com.sbox-manager.sboxsub</string>",
		"<string>" + binary + "</string>",
		"<string>--base-dir</string>",
		"<string>" + baseDir + "</string>",
		"<string>serve</string>",
		"<string>" + filepath.Join(baseDir, "logs", "sboxsub.out.log") + "</string>",
		"<string>" + filepath.Join(baseDir, "logs", "sboxsub.err.log") + "</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("subscription plist missing %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "KeepAlive") {
		t.Fatalf("subscription plist should not contain KeepAlive:\n%s", plist)
	}
}

// TestRenderTrafficTimerFiles 验证 traffic timer 服务文件关键字段符合数据规格。
func TestRenderTrafficTimerFiles(t *testing.T) {
	baseDir := t.TempDir()
	binary := filepath.Join(baseDir, "bin", "sboxctl")
	unit := string(RenderTrafficSystemdService(baseDir, filepath.Join(baseDir, "traffic"), filepath.Join(baseDir, "logs"), binary, "hourly"))
	for _, want := range []string{
		"Type=oneshot",
		"User=sbox",
		"Group=sbox",
		"ExecStart=" + binary + " --base-dir " + baseDir + " traffic collect hourly --instance ALL",
		"ReadWritePaths=" + filepath.Join(baseDir, "traffic") + " " + filepath.Join(baseDir, "logs"),
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("traffic systemd service missing %q:\n%s", want, unit)
		}
	}

	timer := string(RenderTrafficSystemdTimer("monthly"))
	for _, want := range []string{"OnCalendar=*-*-01 00:30:00", "Persistent=true", "AccuracySec=1min", "WantedBy=timers.target"} {
		if !strings.Contains(timer, want) {
			t.Fatalf("traffic systemd timer missing %q:\n%s", want, timer)
		}
	}

	plist := string(RenderTrafficLaunchdPlist(baseDir, filepath.Join(baseDir, "logs"), binary, "daily"))
	for _, want := range []string{
		"<string>com.sbox-manager.traffic.daily</string>",
		"<string>" + binary + "</string>",
		"<string>--base-dir</string>",
		"<string>" + baseDir + "</string>",
		"<string>traffic</string>",
		"<string>collect</string>",
		"<string>daily</string>",
		"<key>StartCalendarInterval</key>",
		"<key>Hour</key>",
		"<integer>0</integer>",
		"<key>Minute</key>",
		"<integer>10</integer>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("traffic launchd plist missing %q:\n%s", want, plist)
		}
	}
}

// TestInstallSystemdWritesTemplateUnitAndOnlyReloads 验证 service install 写模板 unit 且不启动服务。
func TestInstallSystemdWritesTemplateUnitAndOnlyReloads(t *testing.T) {
	baseDir := t.TempDir()
	unitDir := filepath.Join(baseDir, "units")
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, UnitDir: unitDir, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	global := serviceFixtureGlobal(baseDir)
	instances := []domain.Instance{{Name: "edge-us", Enabled: true}}
	if err := os.MkdirAll(unitDir, 0750); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "sbox@edge-us.service"), []byte("legacy"), 0644); err != nil {
		t.Fatalf("write legacy unit: %v", err)
	}

	if err := manager.Install(context.Background(), baseDir, global, instances, ""); err != nil {
		t.Fatalf("install: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(unitDir, "sbox@.service"))
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if !strings.Contains(string(data), filepath.Join(baseDir, "runtime", "generated", "sing-box", "%i.json")) {
		t.Fatalf("unexpected unit:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "sbox@edge-us.service")); !os.IsNotExist(err) {
		t.Fatalf("per-instance systemd unit should not be written, stat err: %v", err)
	}
	got := runner.joined()
	for _, want := range []string{
		"getent group sbox",
		"id -u sbox",
		"chown -R sbox:sbox " + baseDir,
		"systemctl daemon-reload",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("service install missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "systemctl start") {
		t.Fatalf("service install should not start service, got %q", got)
	}
}

// TestUninstallInstancesSkipsMissingFiles 验证实例服务文件不存在时卸载不调用 stop 或 daemon-reload。
func TestUninstallInstancesSkipsMissingFiles(t *testing.T) {
	baseDir := t.TempDir()
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, UnitDir: filepath.Join(baseDir, "units"), Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	instances := []domain.Instance{{Name: "edge-us", Enabled: true}}

	if err := manager.StopInstancesForUninstall(context.Background(), instances); err != nil {
		t.Fatalf("stop instances for uninstall: %v", err)
	}
	if err := manager.UninstallInstances(context.Background(), instances); err != nil {
		t.Fatalf("uninstall instances: %v", err)
	}
	if got := runner.joined(); got != "" {
		t.Fatalf("missing service files should not run commands, got %q", got)
	}
}

// TestInstallSubscriptionSystemdWritesUnitAndOnlyReloads 验证 sboxsub service install 只写服务文件并 reload。
func TestInstallSubscriptionSystemdWritesUnitAndOnlyReloads(t *testing.T) {
	baseDir := t.TempDir()
	unitDir := filepath.Join(baseDir, "units")
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, UnitDir: unitDir, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	binary := filepath.Join(baseDir, "bin", "sboxsub")

	if err := manager.InstallSubscription(context.Background(), baseDir, binary); err != nil {
		t.Fatalf("install subscription: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(unitDir, "sboxsub.service"))
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if !strings.Contains(string(data), "ExecStart="+binary+" --base-dir "+baseDir+" serve") {
		t.Fatalf("unexpected subscription unit:\n%s", data)
	}
	got := runner.joined()
	for _, want := range []string{
		"getent group sbox",
		"id -u sbox",
		"chown -R sbox:sbox " + baseDir,
		"systemctl daemon-reload",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("subscription service install missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "systemctl start") {
		t.Fatalf("subscription service install should not start service, got %q", got)
	}
}

// TestUninstallSubscriptionSkipsMissingFile 验证订阅服务文件不存在时卸载不调用 daemon-reload。
func TestUninstallSubscriptionSkipsMissingFile(t *testing.T) {
	baseDir := t.TempDir()
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, UnitDir: filepath.Join(baseDir, "units"), Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if err := manager.UninstallSubscription(context.Background()); err != nil {
		t.Fatalf("uninstall subscription: %v", err)
	}
	if got := runner.joined(); got != "" {
		t.Fatalf("missing subscription service file should not run commands, got %q", got)
	}
}

// TestInstallTrafficTimersWritesFilesAndOnlyReloads 验证 timer install 只写文件和 daemon-reload。
func TestInstallTrafficTimersWritesFilesAndOnlyReloads(t *testing.T) {
	baseDir := t.TempDir()
	unitDir := filepath.Join(baseDir, "units")
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, UnitDir: unitDir, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	binary := filepath.Join(baseDir, "bin", "sboxctl")

	if err := manager.InstallTrafficTimers(context.Background(), baseDir, filepath.Join(baseDir, "traffic"), filepath.Join(baseDir, "logs"), binary); err != nil {
		t.Fatalf("install traffic timers: %v", err)
	}
	for _, name := range []string{
		"sbox-traffic-hourly.service",
		"sbox-traffic-hourly.timer",
		"sbox-traffic-daily.service",
		"sbox-traffic-daily.timer",
		"sbox-traffic-monthly.service",
		"sbox-traffic-monthly.timer",
	} {
		if _, err := os.Stat(filepath.Join(unitDir, name)); err != nil {
			t.Fatalf("traffic timer file %s should be written: %v", name, err)
		}
	}
	got := runner.joined()
	for _, want := range []string{
		"getent group sbox",
		"id -u sbox",
		"chown -R sbox:sbox " + baseDir,
		"systemctl daemon-reload",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("traffic timer install missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "systemctl start") {
		t.Fatalf("traffic timer install should not start service, got %q", got)
	}
}

// TestUninstallTrafficTimersSkipsMissingFiles 验证 traffic timer 文件不存在时卸载不调用 disable 或 daemon-reload。
func TestUninstallTrafficTimersSkipsMissingFiles(t *testing.T) {
	baseDir := t.TempDir()
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, UnitDir: filepath.Join(baseDir, "units"), Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if err := manager.UninstallTrafficTimers(context.Background()); err != nil {
		t.Fatalf("uninstall traffic timers: %v", err)
	}
	if got := runner.joined(); got != "" {
		t.Fatalf("missing traffic timer files should not run commands, got %q", got)
	}
}

// TestTrafficTimerEnableUsesEnableNow 验证 systemd timer enable 使用 enable --now。
func TestTrafficTimerEnableUsesEnableNow(t *testing.T) {
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.RunTrafficTimers(context.Background(), "enable", false); err != nil {
		t.Fatalf("enable traffic timers: %v", err)
	}
	for _, want := range []string{
		"systemctl enable --now sbox-traffic-hourly.timer",
		"systemctl enable --now sbox-traffic-daily.timer",
		"systemctl enable --now sbox-traffic-monthly.timer",
	} {
		if !strings.Contains(runner.joined(), want) {
			t.Fatalf("traffic timer enable missing %q: %q", want, runner.joined())
		}
	}
}

// TestUninstallTrafficTimersIgnoresMissingTimer 验证未加载的 timer 不阻塞受管文件删除。
func TestUninstallTrafficTimersIgnoresMissingTimer(t *testing.T) {
	baseDir := t.TempDir()
	unitDir := filepath.Join(baseDir, "units")
	if err := os.MkdirAll(unitDir, 0750); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	for _, period := range TrafficPeriods() {
		for _, name := range []string{TrafficSystemdServiceName(period), TrafficSystemdTimerName(period)} {
			if err := os.WriteFile(filepath.Join(unitDir, name), []byte("managed"), 0644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
	}
	runner := &missingTimerRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, UnitDir: unitDir, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if err := manager.UninstallTrafficTimers(context.Background()); err != nil {
		t.Fatalf("uninstall traffic timers: %v", err)
	}
	for _, period := range TrafficPeriods() {
		for _, name := range []string{TrafficSystemdServiceName(period), TrafficSystemdTimerName(period)} {
			if _, err := os.Stat(filepath.Join(unitDir, name)); !os.IsNotExist(err) {
				t.Fatalf("traffic timer file %s should be removed, stat err: %v", name, err)
			}
		}
	}
	if !strings.Contains(runner.joined(), "systemctl daemon-reload") {
		t.Fatalf("uninstall should daemon-reload, got %q", runner.joined())
	}
}

// TestInstallLaunchdWritesPlistAndDoesNotRun 验证 launchd install 只写 plist，不调用 launchctl。
func TestInstallLaunchdWritesPlistAndDoesNotRun(t *testing.T) {
	baseDir := t.TempDir()
	plistDir := filepath.Join(baseDir, "plist")
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindLaunchd, LaunchAgentDir: plistDir, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	global := serviceFixtureGlobal(baseDir)
	instances := []domain.Instance{{Name: "edge-us", Enabled: true}}

	if err := manager.Install(context.Background(), baseDir, global, instances, ""); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(plistDir, "com.sbox-manager.edge-us.plist")); err != nil {
		t.Fatalf("stat plist: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("launchd install should not run commands, got %+v", runner.calls)
	}
}

// TestSystemdActionsUseArgumentArrays 验证 systemctl/journalctl 使用参数数组调用。
func TestSystemdActionsUseArgumentArrays(t *testing.T) {
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.Run(context.Background(), "logs", []string{"sbox@edge-us.service"}, false); err != nil {
		t.Fatalf("run logs: %v", err)
	}
	want := "journalctl -u sbox@edge-us.service --no-pager -n 200"
	if got := runner.joined(); got != want {
		t.Fatalf("unexpected command: got %q want %q", got, want)
	}
}

// TestSystemdLogsFollowStreamsOutput 验证 systemd follow 日志使用流式执行器实时输出。
func TestSystemdLogsFollowStreamsOutput(t *testing.T) {
	runner := &streamingRecordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.Run(context.Background(), "logs", []string{"sbox@edge-us.service"}, true); err != nil {
		t.Fatalf("run logs: %v", err)
	}
	want := "journalctl -u sbox@edge-us.service --no-pager -n 200 -f"
	if got := runner.streamJoined(); got != want {
		t.Fatalf("unexpected stream command: got %q want %q", got, want)
	}
	if got := runner.joined(); got != "" {
		t.Fatalf("follow logs should not use captured runner, got %q", got)
	}
}

// TestSystemdIsRunningUsesIsActive 验证 systemd 运行态通过 is-active 判断。
func TestSystemdIsRunningUsesIsActive(t *testing.T) {
	runner := &commandResultRunner{outputs: map[string][]byte{
		"systemctl is-active sbox@edge-us.service": []byte("active\n"),
	}}
	manager, err := NewManager(Options{Kind: KindSystemd, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	running, err := manager.IsRunning(context.Background(), "sbox@edge-us.service")
	if err != nil {
		t.Fatalf("is running: %v", err)
	}
	if !running {
		t.Fatal("active service should be running")
	}
	if got := runner.joined(); got != "systemctl is-active sbox@edge-us.service" {
		t.Fatalf("unexpected command: %q", got)
	}
}

// TestSystemdIsRunningTreatsInactiveAsStopped 验证 inactive 状态不会作为错误返回。
func TestSystemdIsRunningTreatsInactiveAsStopped(t *testing.T) {
	runner := &commandResultRunner{
		outputs: map[string][]byte{
			"systemctl is-active sbox@edge-us.service": []byte("inactive\n"),
		},
		errors: map[string]error{
			"systemctl is-active sbox@edge-us.service": errors.New("exit status 3"),
		},
	}
	manager, err := NewManager(Options{Kind: KindSystemd, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	running, err := manager.IsRunning(context.Background(), "sbox@edge-us.service")
	if err != nil {
		t.Fatalf("is running should ignore inactive: %v", err)
	}
	if running {
		t.Fatal("inactive service should not be running")
	}
}

// TestLaunchdStartBootstrapsThenKickstarts 验证 launchd start 先 bootstrap plist，再 kickstart label。
func TestLaunchdStartBootstrapsThenKickstarts(t *testing.T) {
	baseDir := t.TempDir()
	plistDir := filepath.Join(baseDir, "plist")
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindLaunchd, LaunchAgentDir: plistDir, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.Run(context.Background(), "start", []string{"com.sbox-manager.edge-us"}, false); err != nil {
		t.Fatalf("launchd start: %v", err)
	}
	got := runner.joined()
	for _, want := range []string{
		"launchctl bootstrap gui/",
		filepath.Join(plistDir, "com.sbox-manager.edge-us.plist"),
		"launchctl kickstart -k gui/",
		"/com.sbox-manager.edge-us",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("launchd start missing %q: %q", want, got)
		}
	}
}

// TestLaunchdEnableBootstrapsThenEnables 验证 launchd enable 会先加载 plist。
func TestLaunchdEnableBootstrapsThenEnables(t *testing.T) {
	baseDir := t.TempDir()
	plistDir := filepath.Join(baseDir, "plist")
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindLaunchd, LaunchAgentDir: plistDir, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.Run(context.Background(), "enable", []string{"com.sbox-manager.edge-us"}, false); err != nil {
		t.Fatalf("launchd enable: %v", err)
	}
	got := runner.joined()
	if !strings.Contains(got, "launchctl bootstrap gui/") || !strings.Contains(got, "launchctl enable gui/") {
		t.Fatalf("launchd enable should bootstrap then enable, got %q", got)
	}
}

// TestTrafficTimerLogsUseServiceUnits 验证 systemd timer 日志查看实际执行的 oneshot service。
func TestTrafficTimerLogsUseServiceUnits(t *testing.T) {
	runner := &streamingRecordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if _, err := manager.RunTrafficTimers(context.Background(), "logs", true); err != nil {
		t.Fatalf("traffic timer logs: %v", err)
	}
	for _, want := range []string{
		"journalctl -u sbox-traffic-hourly.service --no-pager -n 200 -f",
		"journalctl -u sbox-traffic-daily.service --no-pager -n 200 -f",
		"journalctl -u sbox-traffic-monthly.service --no-pager -n 200 -f",
	} {
		if !strings.Contains(runner.streamJoined(), want) {
			t.Fatalf("traffic timer logs missing %q: %q", want, runner.streamJoined())
		}
	}
	if strings.Contains(runner.streamJoined(), "journalctl -u sbox-traffic-hourly.timer") {
		t.Fatalf("traffic timer logs should not query timer unit: %q", runner.streamJoined())
	}
	if got := runner.joined(); got != "" {
		t.Fatalf("traffic timer follow logs should not use captured runner, got %q", got)
	}
}

// TestLaunchdTrafficTimerDisableDisablesAndBootsOut 验证 launchd disable 会停止已加载 job。
func TestLaunchdTrafficTimerDisableDisablesAndBootsOut(t *testing.T) {
	baseDir := t.TempDir()
	plistDir := filepath.Join(baseDir, "plist")
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindLaunchd, LaunchAgentDir: plistDir, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if _, err := manager.RunTrafficTimers(context.Background(), "disable", false); err != nil {
		t.Fatalf("disable traffic timers: %v", err)
	}
	got := runner.joined()
	for _, want := range []string{
		"launchctl disable gui/",
		"/com.sbox-manager.traffic.hourly",
		"launchctl bootout gui/",
		"/com.sbox-manager.traffic.monthly",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("launchd traffic disable missing %q: %q", want, got)
		}
	}
}

func serviceFixtureGlobal(baseDir string) domain.GlobalConfig {
	global := domain.DefaultGlobalConfig()
	global.Paths.Bin = filepath.Join(baseDir, "bin")
	global.Paths.Rules = filepath.Join(baseDir, "rules")
	global.Paths.Instances = filepath.Join(baseDir, "instances")
	global.Paths.Runtime = filepath.Join(baseDir, "runtime")
	global.Paths.Generated = filepath.Join(baseDir, "runtime", "generated")
	global.Paths.Publish = filepath.Join(baseDir, "publish")
	global.Paths.Traffic = filepath.Join(baseDir, "traffic")
	global.Paths.Downloads = filepath.Join(baseDir, "downloads")
	global.Paths.Logs = filepath.Join(baseDir, "logs")
	return global
}

type recordingRunner struct {
	calls []string
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	return nil, nil
}

func (r *recordingRunner) joined() string {
	return strings.Join(r.calls, "\n")
}

type streamingRecordingRunner struct {
	recordingRunner
	streamCalls []string
}

func (r *streamingRecordingRunner) Stream(ctx context.Context, name string, args ...string) error {
	r.streamCalls = append(r.streamCalls, strings.Join(append([]string{name}, args...), " "))
	return nil
}

func (r *streamingRecordingRunner) streamJoined() string {
	return strings.Join(r.streamCalls, "\n")
}

type commandResultRunner struct {
	recordingRunner
	outputs map[string][]byte
	errors  map[string]error
}

func (r *commandResultRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	r.calls = append(r.calls, call)
	return r.outputs[call], r.errors[call]
}

type missingTimerRunner struct {
	recordingRunner
}

func (r *missingTimerRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	r.calls = append(r.calls, call)
	if strings.Contains(call, "disable --now sbox-traffic-") {
		return []byte("Unit not loaded"), errors.New("unit not found")
	}
	return nil, nil
}
