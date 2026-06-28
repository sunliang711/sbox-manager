package service

import (
	"context"
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
	if got := runner.joined(); got != "systemctl daemon-reload" {
		t.Fatalf("service install should only daemon-reload, got %q", got)
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
