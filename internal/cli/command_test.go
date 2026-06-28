package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunliang711/sbox-manager/internal/domain"
	installer "github.com/sunliang711/sbox-manager/internal/install"
	runtimeplan "github.com/sunliang711/sbox-manager/internal/runtime"
	"github.com/sunliang711/sbox-manager/internal/service"
	"github.com/sunliang711/sbox-manager/internal/version"
)

// TestSboxctlVersionOutput 验证 sboxctl version 输出 ldflags 可注入字段。
func TestSboxctlVersionOutput(t *testing.T) {
	restoreVersion(t)
	version.Version = "v1.2.3"
	version.Commit = "abc1234"
	version.BuildTime = "2026-06-28T00:00:00Z"

	output, err := executeCommand(newSboxctlCommand(), "version")
	if err != nil {
		t.Fatalf("execute sboxctl version: %v", err)
	}

	for _, want := range []string{
		"Version: v1.2.3",
		"Commit: abc1234",
		"BuildTime: 2026-06-28T00:00:00Z",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("version output missing %q: %s", want, output)
		}
	}
}

// TestSboxsubVersionOutput 验证 sboxsub version 输出 ldflags 可注入字段。
func TestSboxsubVersionOutput(t *testing.T) {
	restoreVersion(t)
	version.Version = "v2.0.0"
	version.Commit = "def5678"
	version.BuildTime = "2026-06-28T01:00:00Z"

	output, err := executeCommand(newSboxsubCommand(), "--listen", "127.0.0.1:8080", "version")
	if err != nil {
		t.Fatalf("execute sboxsub version: %v", err)
	}

	for _, want := range []string{
		"Version: v2.0.0",
		"Commit: def5678",
		"BuildTime: 2026-06-28T01:00:00Z",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("version output missing %q: %s", want, output)
		}
	}
}

// TestStubCommandReturnsNotImplemented 验证占位命令返回统一未实现错误。
func TestStubCommandReturnsNotImplemented(t *testing.T) {
	_, err := executeCommand(newSboxctlCommand(), "doctor")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

// TestSboxctlCheckIsReadOnly 验证 check 输出 plan 且不写 runtime。
func TestSboxctlCheckIsReadOnly(t *testing.T) {
	baseDir := writeAgentFixture(t)

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "check")
	if err != nil {
		t.Fatalf("execute check: %v", err)
	}
	if !strings.Contains(output, "create edge-us generated/sing-box/edge-us.json") {
		t.Fatalf("unexpected check output: %s", output)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "runtime", "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("check should not write manifest, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "runtime", "generated")); !os.IsNotExist(err) {
		t.Fatalf("check should not write generated dir, stat err: %v", err)
	}
}

// TestSboxctlRenderCommands 验证 render sing-box 和 render sub 的基础路径。
func TestSboxctlRenderCommands(t *testing.T) {
	baseDir := writeAgentFixture(t)
	restoreNow(t)
	cliNow = func() time.Time {
		return time.Date(2026, 6, 28, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	}

	singBoxOutput, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "render", "sing-box", "edge-us")
	if err != nil {
		t.Fatalf("execute render sing-box: %v", err)
	}
	for _, want := range []string{`"inbounds"`, `"outbounds"`, `"final": "direct"`} {
		if !strings.Contains(singBoxOutput, want) {
			t.Fatalf("render sing-box output missing %q: %s", want, singBoxOutput)
		}
	}

	subOutput, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "render", "sub")
	if err != nil {
		t.Fatalf("execute render sub: %v", err)
	}
	for _, want := range []string{`"input_schema": "sbox.subscription-input"`, `"external_host": "proxy.example.com"`, `"user": "alice"`} {
		if !strings.Contains(subOutput, want) {
			t.Fatalf("render sub output missing %q: %s", want, subOutput)
		}
	}
}

// TestSboxctlStartGeneratesChecksAndStartsService 验证 start 会先生成/check runtime 再调用服务管理器。
func TestSboxctlStartGeneratesChecksAndStartsService(t *testing.T) {
	baseDir := writeAgentFixture(t)
	runner := &cliRecordingRunner{}
	checker := &cliFakeChecker{}
	restoreRuntimeHooks(t)
	newRuntimeConfigChecker = func(*rootOptions, domain.GlobalConfig) runtimeplan.ConfigChecker {
		return checker
	}
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: filepath.Join(t.TempDir(), "units"), Runner: runner})
	}

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "start")
	if err != nil {
		t.Fatalf("execute start: %v\n%s", err, output)
	}
	if checker.calls != 1 {
		t.Fatalf("expected one check call, got %d", checker.calls)
	}
	if !strings.Contains(runner.joined(), "systemctl start sbox@edge-us.service") {
		t.Fatalf("expected systemctl start, got %q", runner.joined())
	}
	if _, err := os.Stat(filepath.Join(baseDir, "runtime", "manifest.json")); err != nil {
		t.Fatalf("start should write manifest: %v", err)
	}
}

// TestSboxctlStopDoesNotWriteRuntime 验证 stop 只调用服务管理器，不写 runtime。
func TestSboxctlStopDoesNotWriteRuntime(t *testing.T) {
	baseDir := writeAgentFixture(t)
	runner := &cliRecordingRunner{}
	restoreRuntimeHooks(t)
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: filepath.Join(t.TempDir(), "units"), Runner: runner})
	}

	if _, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "stop"); err != nil {
		t.Fatalf("execute stop: %v", err)
	}
	if got := runner.joined(); got != "systemctl stop sbox@edge-us.service" {
		t.Fatalf("unexpected stop command: %q", got)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "runtime", "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("stop should not write manifest, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "runtime", "generated")); !os.IsNotExist(err) {
		t.Fatalf("stop should not write generated dir, stat err: %v", err)
	}
}

// TestSboxctlServiceInstallDoesNotStart 验证 service install 只写服务文件并 reload，不启动服务。
func TestSboxctlServiceInstallDoesNotStart(t *testing.T) {
	baseDir := writeAgentFixture(t)
	unitDir := filepath.Join(t.TempDir(), "units")
	runner := &cliRecordingRunner{}
	restoreRuntimeHooks(t)
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: unitDir, Runner: runner})
	}

	if _, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "service", "install"); err != nil {
		t.Fatalf("execute service install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "sbox@edge-us.service")); err != nil {
		t.Fatalf("unit should be written: %v", err)
	}
	if got := runner.joined(); got != "systemctl daemon-reload" {
		t.Fatalf("service install should not start service, got %q", got)
	}
}

// TestSboxctlUninstallPurgeRemovesServiceFileAndManagedDirs 验证 uninstall --purge 删除服务文件和受管资源目录。
func TestSboxctlUninstallPurgeRemovesServiceFileAndManagedDirs(t *testing.T) {
	baseDir := writeAgentFixture(t)
	unitDir := filepath.Join(t.TempDir(), "units")
	runner := &cliRecordingRunner{}
	restoreRuntimeHooks(t)
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: unitDir, Runner: runner})
	}
	if _, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "service", "install"); err != nil {
		t.Fatalf("execute service install: %v", err)
	}
	for _, dir := range []string{"bin", "rules", "downloads", "runtime"} {
		path := filepath.Join(baseDir, dir)
		if err := os.MkdirAll(path, 0750); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(path, "managed"), []byte("data"), 0640); err != nil {
			t.Fatalf("write managed file: %v", err)
		}
	}

	if _, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "uninstall", "all", "--purge"); err != nil {
		t.Fatalf("execute uninstall purge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "sbox@edge-us.service")); !os.IsNotExist(err) {
		t.Fatalf("service file should be removed, stat err: %v", err)
	}
	for _, dir := range []string{"bin", "rules", "downloads", "runtime"} {
		path := filepath.Join(baseDir, dir)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("managed dir should be removed %s, stat err: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(baseDir, "config.yaml")); err != nil {
		t.Fatalf("config should be preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "instances", "edge-us.yaml")); err != nil {
		t.Fatalf("instance config should be preserved: %v", err)
	}
}

// TestSboxctlSetupOrder 验证 setup 依次执行 init、install all、service install。
func TestSboxctlSetupOrder(t *testing.T) {
	baseDir := writeAgentFixture(t)
	order := []string{}
	runner := &cliRecordingRunner{onRun: func(call string) {
		order = append(order, call)
	}}
	restoreRuntimeHooks(t)
	restoreResourceInstaller(t)
	newResourceInstaller = func() resourceInstallerRunner {
		return cliFakeInstaller{run: func(options installer.Options) error {
			if _, err := os.Stat(filepath.Join(baseDir, "config.yaml")); err != nil {
				t.Fatalf("init should run before install all: %v", err)
			}
			order = append(order, "install:"+options.Resource)
			return nil
		}}
	}
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: filepath.Join(t.TempDir(), "units"), Runner: runner})
	}

	if _, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "setup"); err != nil {
		t.Fatalf("execute setup: %v", err)
	}
	got := strings.Join(order, "|")
	want := "install:all|systemctl daemon-reload"
	if got != want {
		t.Fatalf("unexpected setup order: got %q want %q", got, want)
	}
}

// TestInvalidServiceManager 验证 service-manager 全局参数只接受约定值。
func TestInvalidServiceManager(t *testing.T) {
	_, err := executeCommand(newSboxsubCommand(), "--service-manager", "noop", "version")
	if err == nil {
		t.Fatal("expected invalid service-manager error")
	}
	if !strings.Contains(err.Error(), "service-manager") {
		t.Fatalf("expected service-manager error, got %v", err)
	}
}

// TestInvalidListenAddress 验证 sboxsub 全局 listen 参数必须符合 HOST:PORT。
func TestInvalidListenAddress(t *testing.T) {
	_, err := executeCommand(newSboxsubCommand(), "--listen", "invalid", "version")
	if err == nil {
		t.Fatal("expected invalid listen error")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Fatalf("expected listen error, got %v", err)
	}
}

// TestListenAddressPortRange 验证 sboxsub listen 端口必须在有效范围内。
func TestListenAddressPortRange(t *testing.T) {
	_, err := executeCommand(newSboxsubCommand(), "--listen", "127.0.0.1:70000", "version")
	if err == nil {
		t.Fatal("expected listen port range error")
	}
	if !strings.Contains(err.Error(), "1-65535") {
		t.Fatalf("expected port range error, got %v", err)
	}
}

// TestTrafficExportAcceptsDateFlag 验证 traffic export 周期命令接受规格中的 date 参数。
func TestTrafficExportAcceptsDateFlag(t *testing.T) {
	for _, period := range []string{"hourly", "daily", "monthly"} {
		_, err := executeCommand(newSboxctlCommand(), "traffic", "export", period, "--date", "2026-06-28", "--instance", "ALL")
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("traffic export %s should parse --date and return ErrNotImplemented, got %v", period, err)
		}
	}
}

// TestRootHelpShowsResponsibilities 验证根命令 help 能区分 agent 与 sub 职责。
func TestRootHelpShowsResponsibilities(t *testing.T) {
	sboxctlHelp, err := executeCommand(newSboxctlCommand(), "--help")
	if err != nil {
		t.Fatalf("execute sboxctl help: %v", err)
	}
	if !strings.Contains(sboxctlHelp, "agent") || !strings.Contains(sboxctlHelp, "实例生命周期") {
		t.Fatalf("sboxctl help does not describe agent responsibility: %s", sboxctlHelp)
	}

	sboxsubHelp, err := executeCommand(newSboxsubCommand(), "--help")
	if err != nil {
		t.Fatalf("execute sboxsub help: %v", err)
	}
	if !strings.Contains(sboxsubHelp, "订阅服务") || !strings.Contains(sboxsubHelp, "不读取 agent 配置") {
		t.Fatalf("sboxsub help does not describe sub responsibility: %s", sboxsubHelp)
	}
}

// executeCommand 执行命令并返回标准输出和标准错误的合并内容。
func executeCommand(cmd *cobra.Command, args ...string) (string, error) {
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return output.String(), err
}

// restoreVersion 在测试结束后恢复全局版本变量。
func restoreVersion(t *testing.T) {
	t.Helper()

	originalVersion := version.Version
	originalCommit := version.Commit
	originalBuildTime := version.BuildTime
	t.Cleanup(func() {
		version.Version = originalVersion
		version.Commit = originalCommit
		version.BuildTime = originalBuildTime
	})
}

// restoreNow 在测试结束后恢复 CLI 当前时间函数。
func restoreNow(t *testing.T) {
	t.Helper()

	originalNow := cliNow
	t.Cleanup(func() {
		cliNow = originalNow
	})
}

// writeAgentFixture 写入 T03 CLI 测试所需的 agent 配置。
func writeAgentFixture(t *testing.T) string {
	t.Helper()

	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "instances"), 0750); err != nil {
		t.Fatalf("mkdir instances: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "config.yaml"), []byte(`version: 1
external_host: proxy.example.com
`), 0640); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "instances", "edge-us.yaml"), []byte(`name: edge-us
api:
  enabled: false
inbounds:
  - name: vmess-main
    type: vmess
    listen: 0.0.0.0
    port: 24100
    users:
      - name: alice
        uuid: 11111111-1111-4111-8111-111111111111
    subscription:
      enabled: true
      user: alice
      remark: US VMess
      region: US
outbounds:
  - name: direct
    type: direct
route:
  default: direct
`), 0640); err != nil {
		t.Fatalf("write instance: %v", err)
	}
	return baseDir
}

// restoreRuntimeHooks 在测试结束后恢复 CLI 生命周期注入点。
func restoreRuntimeHooks(t *testing.T) {
	t.Helper()

	originalChecker := newRuntimeConfigChecker
	originalRuntimeManager := newRuntimeServiceManager
	originalServiceManager := newSboxctlServiceManager
	t.Cleanup(func() {
		newRuntimeConfigChecker = originalChecker
		newRuntimeServiceManager = originalRuntimeManager
		newSboxctlServiceManager = originalServiceManager
	})
}

// restoreResourceInstaller 在测试结束后恢复资源安装器注入点。
func restoreResourceInstaller(t *testing.T) {
	t.Helper()

	original := newResourceInstaller
	t.Cleanup(func() {
		newResourceInstaller = original
	})
}

type cliFakeChecker struct {
	calls int
}

// Check 记录 sing-box check 调用次数。
func (f *cliFakeChecker) Check(ctx context.Context, instance string, data []byte) error {
	f.calls++
	return nil
}

type cliRecordingRunner struct {
	calls []string
	onRun func(string)
}

func (r *cliRecordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	r.calls = append(r.calls, call)
	if r.onRun != nil {
		r.onRun(call)
	}
	return nil, nil
}

func (r *cliRecordingRunner) joined() string {
	return strings.Join(r.calls, "\n")
}

type cliFakeInstaller struct {
	run func(installer.Options) error
}

func (f cliFakeInstaller) Run(ctx context.Context, global domain.GlobalConfig, options installer.Options) error {
	if f.run == nil {
		return nil
	}
	return f.run(options)
}
