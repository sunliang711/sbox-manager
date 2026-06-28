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

// TestRenderSystemdUnitFields 验证 systemd unit 关键字段符合数据规格。
func TestRenderSystemdUnitFields(t *testing.T) {
	baseDir := t.TempDir()
	data := string(RenderSystemdUnit(baseDir, filepath.Join(baseDir, "bin"), filepath.Join(baseDir, "runtime", "generated"), filepath.Join(baseDir, "traffic"), filepath.Join(baseDir, "logs"), "edge-us"))

	for _, want := range []string{
		"User=sbox",
		"Group=sbox",
		"ExecStart=" + filepath.Join(baseDir, "bin", "sing-box") + " run -c " + filepath.Join(baseDir, "runtime", "generated", "sing-box", "edge-us.json"),
		"WorkingDirectory=" + baseDir,
		"Restart=on-failure",
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
		"ProtectHome=true",
		"PrivateTmp=true",
		"ReadWritePaths=" + filepath.Join(baseDir, "runtime") + " " + filepath.Join(baseDir, "traffic") + " " + filepath.Join(baseDir, "logs"),
		"SyslogIdentifier=sbox-edge-us",
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

// TestInstallSystemdWritesUnitAndOnlyReloads 验证 service install 写 unit 且不启动服务。
func TestInstallSystemdWritesUnitAndOnlyReloads(t *testing.T) {
	baseDir := t.TempDir()
	unitDir := filepath.Join(baseDir, "units")
	runner := &recordingRunner{}
	manager, err := NewManager(Options{Kind: KindSystemd, UnitDir: unitDir, Runner: runner})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	global := serviceFixtureGlobal(baseDir)
	instances := []domain.Instance{{Name: "edge-us", Enabled: true}}

	if err := manager.Install(context.Background(), baseDir, global, instances, ""); err != nil {
		t.Fatalf("install: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(unitDir, "sbox@edge-us.service"))
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if !strings.Contains(string(data), "ExecStart="+filepath.Join(baseDir, "bin", "sing-box")) {
		t.Fatalf("unexpected unit:\n%s", data)
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
	if _, err := manager.Run(context.Background(), "logs", []string{"sbox@edge-us.service"}, true); err != nil {
		t.Fatalf("run logs: %v", err)
	}
	want := "journalctl -u sbox@edge-us.service --no-pager -n 200 -f"
	if got := runner.joined(); got != want {
		t.Fatalf("unexpected command: got %q want %q", got, want)
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
