package cli

import (
	"bytes"
	"context"
	"fmt"
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
		"Version",
		"v1.2.3",
		"Commit",
		"abc1234",
		"Build time",
		"2026-06-28T00:00:00Z",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("version output missing %q: %s", want, output)
		}
	}
}

// TestDraftPathAppendsDraftSuffix 验证编辑草稿名保留原配置文件名并追加 draft 后缀。
func TestDraftPathAppendsDraftSuffix(t *testing.T) {
	if got := draftPath("/opt/sbox-manager/instances/usa4.yaml"); got != "/opt/sbox-manager/instances/usa4.yaml.draft" {
		t.Fatalf("draft path = %q", got)
	}
}

// TestSboxctlComponentVersionOutput 验证 sboxctl version 可查询受管组件。
func TestSboxctlComponentVersionOutput(t *testing.T) {
	baseDir := writeAgentFixture(t)
	binDir := filepath.Join(baseDir, "bin")
	rulesDir := filepath.Join(baseDir, "rules")
	if err := os.MkdirAll(binDir, 0750); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.MkdirAll(rulesDir, 0750); err != nil {
		t.Fatalf("mkdir rules: %v", err)
	}
	singBox := filepath.Join(binDir, "sing-box")
	if err := os.WriteFile(singBox, []byte("#!/bin/sh\necho 'sing-box test-version'\n"), 0755); err != nil {
		t.Fatalf("write sing-box fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, rulesManagedMarkerName), []byte("geoip.db abc123\n"), 0640); err != nil {
		t.Fatalf("write rules marker: %v", err)
	}

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "version", "sing-box")
	if err != nil {
		t.Fatalf("execute sboxctl version sing-box: %v\n%s", err, output)
	}
	if !strings.Contains(output, "sing-box test-version") {
		t.Fatalf("sing-box version output missing fixture version: %s", output)
	}

	output, err = executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "version", "rules")
	if err != nil {
		t.Fatalf("execute sboxctl version rules: %v\n%s", err, output)
	}
	for _, want := range []string{"Directory:", rulesDir, "geoip.db abc123"} {
		if !strings.Contains(output, want) {
			t.Fatalf("rules version output missing %q: %s", want, output)
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
		"Version",
		"v2.0.0",
		"Commit",
		"def5678",
		"Build time",
		"2026-06-28T01:00:00Z",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("version output missing %q: %s", want, output)
		}
	}
}

// TestSboxctlExampleUsesKindAndType 验证 example 会按 kind 和 TYPE 输出对应的完整注释片段。
func TestSboxctlExampleUsesKindAndType(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		notWant []string
	}{
		{
			name: "global",
			args: []string{"example", "global"},
			want: []string{"paths:", "port_ranges:", "defaults:", "security:"},
		},
		{
			name:    "inbound vmess",
			args:    []string{"example", "inbound", "vmess"},
			want:    []string{"# VMess inbound", "type: vmess", "uuid:", "subscription:"},
			notWant: []string{"type: shadowsocks"},
		},
		{
			name: "inbound all protocols",
			args: []string{"example", "inbound"},
			want: []string{"VMess inbound（raw）", "VMess WebSocket inbound", "VMess gRPC inbound", "VMess HTTP inbound", "VMess QUIC inbound", "VMess HTTPUpgrade inbound", "VLESS inbound（WebSocket）", "VLESS REALITY Vision inbound", "type: anytls", "type: shadowsocks", "type: socks5", "type: http"},
		},
		{
			name: "inbound vmess websocket",
			args: []string{"example", "inbound", "vmess-websocket"},
			want: []string{"# VMess WebSocket inbound", "type: vmess", "type: ws", "/vmess-websocket"},
		},
		{
			name:    "inbound vless",
			args:    []string{"example", "inbound", "vless"},
			want:    []string{"# VLESS inbound", "type: vless", "uuid:", "transport:"},
			notWant: []string{"type: anytls"},
		},
		{
			name: "inbound vless reality vision",
			args: []string{"example", "inbound", "vless-reality-vision"},
			want: []string{"# VLESS REALITY Vision inbound", "flow: xtls-rprx-vision", "reality:", "private_key:", "short_ids:"},
		},
		{
			name:    "inbound anytls",
			args:    []string{"example", "inbound", "anytls"},
			want:    []string{"# AnyTLS inbound", "type: anytls", "password:", "tls:", "enabled: true"},
			notWant: []string{"type: vless"},
		},
		{
			name: "inbound shadowsocks22 alias",
			args: []string{"example", "inbound", "shadowsocks22"},
			want: []string{"type: shadowsocks", "2022-blake3-aes-256-gcm", "password:", "shadowsocks22 是示例别名"},
		},
		{
			name: "outbound all protocols",
			args: []string{"example", "outbound"},
			want: []string{"type: direct", "type: block", "type: ref", "type: shadowsocks", "VMess raw outbound", "VMess outbound（WebSocket）", "VMess gRPC outbound", "VMess HTTP outbound", "VMess QUIC outbound", "VMess HTTPUpgrade outbound", "VLESS outbound（WebSocket）", "VLESS REALITY Vision outbound", "type: anytls", "type: trojan", "type: hysteria2", "type: socks5", "type: http"},
		},
		{
			name: "outbound vmess grpc",
			args: []string{"example", "outbound", "vmess-grpc"},
			want: []string{"# VMess gRPC outbound", "type: vmess", "type: grpc", "service_name:"},
		},
		{
			name: "outbound ref",
			args: []string{"example", "outbound", "ref"},
			want: []string{"# Ref outbound", "type: ref", "ref: edge-us.local-socks"},
		},
		{
			name:    "outbound vmess",
			args:    []string{"example", "outbound", "vmess"},
			want:    []string{"# VMess outbound", "uuid:", "alter_id:", "security: auto", "tls:", "server_name:", "insecure:", "alpn:", "network: tcp", "type: ws", "headers:"},
			notWant: []string{"type: shadowsocks"},
		},
		{
			name:    "outbound vless",
			args:    []string{"example", "outbound", "vless"},
			want:    []string{"# VLESS outbound", "type: vless", "uuid:", "transport:"},
			notWant: []string{"type: vmess"},
		},
		{
			name: "outbound vless reality alias",
			args: []string{"example", "outbound", "vless-reality"},
			want: []string{"# VLESS REALITY Vision outbound", "flow: xtls-rprx-vision", "utls:", "fingerprint: chrome", "public_key:", "short_id:"},
		},
		{
			name:    "outbound anytls",
			args:    []string{"example", "outbound", "anytls"},
			want:    []string{"# AnyTLS outbound", "type: anytls", "password:", "enabled: true"},
			notWant: []string{"type: trojan"},
		},
		{
			name: "outbound socks alias",
			args: []string{"example", "outbound", "socks"},
			want: []string{"type: socks5", "auth:", "username:", "password:"},
		},
		{
			name: "group urltest",
			args: []string{"example", "group", "urltest"},
			want: []string{"type: urltest", "outbounds:", "url:", "tolerance:"},
		},
		{
			name: "route geosite",
			args: []string{"example", "route", "geosite"},
			want: []string{"type: geosite", "category-ads-all", "outbound: auto"},
		},
		{
			name: "instance urltest ref members",
			args: []string{"example", "instance", "urltest"},
			want: []string{"--members edge-us,relay-us", "type: ref", "ref: edge-us.local-socks", "ref: relay-us.local-socks", "outbounds: [edge-us-local-socks, relay-us-local-socks]"},
		},
		{
			name:    "instance relay",
			args:    []string{"example", "instance", "relay"},
			want:    []string{"role: relay", "type: shadowsocks", "type: vmess", "default: vmess-upstream", "traffic:"},
			notWant: []string{"role: edge"},
		},
		{
			name: "traffic",
			args: []string{"example", "traffic"},
			want: []string{"retention_days:", "timer:", "scopes: [user, inbound, outbound]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := executeCommand(newSboxctlCommand(), tt.args...)
			if err != nil {
				t.Fatalf("execute %v: %v\n%s", tt.args, err, output)
			}
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Fatalf("example output missing %q:\n%s", want, output)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(output, notWant) {
					t.Fatalf("example output should not contain %q:\n%s", notWant, output)
				}
			}
		})
	}
}

// TestSboxctlExampleRejectsUnsupportedType 验证未知 TYPE 会返回带支持列表的错误。
func TestSboxctlExampleRejectsUnsupportedType(t *testing.T) {
	output, err := executeCommand(newSboxctlCommand(), "example", "outbound", "wireguard")
	if err == nil {
		t.Fatalf("unsupported example type should fail:\n%s", output)
	}
	if !strings.Contains(err.Error(), "wireguard") || !strings.Contains(err.Error(), "shadowsocks22") {
		t.Fatalf("unsupported example type error should include type and supported list, got %v\n%s", err, output)
	}
}

// TestSboxctlSetupLocalPreparesBaseAndEnablesTrafficTimers 验证 local 阶段准备本机文件并启用 timer。
func TestSboxctlSetupLocalPreparesBaseAndEnablesTrafficTimers(t *testing.T) {
	baseDir := t.TempDir()
	unitDir := filepath.Join(t.TempDir(), "units")
	runner := &cliRecordingRunner{}
	restoreRuntimeHooks(t)
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: unitDir, Runner: runner})
	}

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "setup", "local", "--external-host", "proxy.example.com")
	if err != nil {
		t.Fatalf("execute setup local: %v\n%s", err, output)
	}
	for _, want := range []string{
		"Traffic timers enabled.",
		"Local setup completed.",
		"Base dir:",
		baseDir,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("setup local output missing %q:\n%s", want, output)
		}
	}
	if _, err := os.Stat(filepath.Join(baseDir, "config.yaml")); err != nil {
		t.Fatalf("setup local should write config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "sbox@.service")); err != nil {
		t.Fatalf("setup local should write template service: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unitDir, service.TrafficSystemdTimerName("hourly"))); err != nil {
		t.Fatalf("setup local should write traffic timer: %v", err)
	}
	if !strings.Contains(runner.joined(), "systemctl enable --now sbox-traffic-hourly.timer") {
		t.Fatalf("setup local should enable traffic timers, got %q", runner.joined())
	}
}

// TestSboxsubInitPrintsNextSteps 验证subscription service init 后会提示下一步操作。
func TestSboxsubInitPrintsNextSteps(t *testing.T) {
	baseDir := t.TempDir()
	output, err := executeCommand(newSboxsubCommand(), "--base-dir", baseDir, "init")
	if err != nil {
		t.Fatalf("execute sboxsub init: %v\n%s", err, output)
	}
	for _, want := range []string{
		"Subscription service initialized.",
		"Base dir:",
		baseDir,
		"Import a subscription bundle exported by the agent",
		"sboxsub --base-dir " + baseDir + " import /path/to/sbox-sub-bundle.zip",
		"sudo sboxsub --base-dir " + baseDir + " service install",
		"sudo sboxsub --base-dir " + baseDir + " start",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("sub init output missing %q:\n%s", want, output)
		}
	}
}

// TestSboxctlDoctorReturnsNonZeroOnIssue 验证 doctor 发现 ISSUE 时返回非零。
func TestSboxctlDoctorReturnsNonZeroOnIssue(t *testing.T) {
	baseDir := writeAgentFixture(t)

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "doctor")
	if err == nil {
		t.Fatal("expected doctor issue error")
	}
	if !strings.Contains(output, "ISSUE") {
		t.Fatalf("expected ISSUE output, got %s", output)
	}
	if !strings.Contains(err.Error(), "doctor found issues") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestSboxctlDoctorAfterSetupLocalSucceeds 验证 local setup 后的空环境 doctor 不会把未安装组件当成故障。
func TestSboxctlDoctorAfterSetupLocalSucceeds(t *testing.T) {
	baseDir := t.TempDir()
	unitDir := filepath.Join(t.TempDir(), "units")
	runner := &cliRecordingRunner{}
	restoreRuntimeHooks(t)
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: unitDir, Runner: runner})
	}
	if output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "setup", "local"); err != nil {
		t.Fatalf("setup local failed: %v\n%s", err, output)
	}

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "doctor")
	if err != nil {
		t.Fatalf("doctor after init should succeed: %v\n%s", err, output)
	}
	if strings.Contains(output, "ISSUE") {
		t.Fatalf("doctor after init should not contain ISSUE:\n%s", output)
	}
	for _, want := range []string{"sing-box.binary", "traffic.db", "traffic.timer.hourly"} {
		if !strings.Contains(output, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, output)
		}
	}
	if !strings.Contains(output, "setup local") {
		t.Fatalf("doctor output missing traffic timer hint:\n%s", output)
	}
}

// TestSboxctlCheckIsReadOnly 验证 check 输出 plan 且不写 runtime。
func TestSboxctlCheckIsReadOnly(t *testing.T) {
	baseDir := writeAgentFixture(t)

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "check")
	if err != nil {
		t.Fatalf("execute check: %v", err)
	}
	for _, want := range []string{"Plan", "create", "edge-us", "generated/sing-box/edge-us.json"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unexpected check output missing %q: %s", want, output)
		}
	}
	if _, err := os.Stat(filepath.Join(baseDir, "runtime", "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("check should not write manifest, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "runtime", "generated")); !os.IsNotExist(err) {
		t.Fatalf("check should not write generated dir, stat err: %v", err)
	}
}

// TestSboxctlListUsesStatusTable 验证 list 输出固定状态表。
func TestSboxctlListUsesStatusTable(t *testing.T) {
	baseDir := writeAgentFixture(t)
	generatedPath := filepath.Join(baseDir, "runtime", "generated", "sing-box", "edge-us.json")
	if err := os.MkdirAll(filepath.Dir(generatedPath), 0750); err != nil {
		t.Fatalf("mkdir generated dir: %v", err)
	}
	if err := os.WriteFile(generatedPath, []byte("{}"), 0640); err != nil {
		t.Fatalf("write generated config: %v", err)
	}
	runner := &cliRecordingRunner{outputs: map[string][]byte{
		"systemctl is-active sbox@edge-us.service": []byte("active\n"),
	}}
	restoreRuntimeHooks(t)
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, Runner: runner})
	}

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "list")
	if err != nil {
		t.Fatalf("execute list: %v\n%s", err, output)
	}
	for _, want := range []string{
		"Name",
		"Role",
		"Enabled",
		"Running",
		"Generated",
		"Ports",
		"------",
		"edge-us",
		"edge",
		"yes",
		"vmess-main=24100",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("list output missing %q:\n%s", want, output)
		}
	}
	if !strings.Contains(runner.joined(), "systemctl is-active sbox@edge-us.service") {
		t.Fatalf("list should check service status, got %q", runner.joined())
	}
}

// TestSboxctlRenderGroupShowsHelp 验证 render 分组直接执行时展示帮助，避免误报未实现。
func TestSboxctlRenderGroupShowsHelp(t *testing.T) {
	output, err := executeCommand(newSboxctlCommand(), "render")
	if err != nil {
		t.Fatalf("execute render group: %v\n%s", err, output)
	}
	for _, want := range []string{"Render model, sing-box configuration, or subscription input", "sing-box", "sub"} {
		if !strings.Contains(output, want) {
			t.Fatalf("render help missing %q:\n%s", want, output)
		}
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

// TestSboxctlRenderSubHelpDoesNotAdvertiseInputDir 验证 render sub 不展示无效 input-dir 参数。
func TestSboxctlRenderSubHelpDoesNotAdvertiseInputDir(t *testing.T) {
	output, err := executeCommand(newSboxctlCommand(), "render", "sub", "--help")
	if err != nil {
		t.Fatalf("execute render sub help: %v\n%s", err, output)
	}
	if strings.Contains(output, "input-dir") {
		t.Fatalf("render sub help should not advertise unused input-dir flag:\n%s", output)
	}
}

// TestSboxctlSubExportDryRunAndSummaryDoNotWritePublish 验证 dry-run 和 summary 不写 publish。
func TestSboxctlSubExportDryRunAndSummaryDoNotWritePublish(t *testing.T) {
	for _, args := range [][]string{
		{"sub", "export", "--dry-run"},
		{"sub", "export", "--summary"},
	} {
		baseDir := writeAgentFixture(t)
		restoreNow(t)
		cliNow = func() time.Time {
			return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
		}

		commandArgs := append([]string{"--base-dir", baseDir}, args...)
		output, err := executeCommand(newSboxctlCommand(), commandArgs...)
		if err != nil {
			t.Fatalf("execute %v: %v\n%s", args, err, output)
		}
		if !strings.Contains(output, "Subscription bundle") {
			t.Fatalf("expected summary output, got %s", output)
		}
		if args[len(args)-1] == "--dry-run" && !strings.Contains(output, "Dry run only; no files will be written.") {
			t.Fatalf("expected dry-run output, got %s", output)
		}
		if _, err := os.Stat(filepath.Join(baseDir, "publish")); !os.IsNotExist(err) {
			t.Fatalf("%v should not create publish, stat err: %v", args, err)
		}
	}
}

// TestSboxsubServiceInstallDoesNotStart 验证 sboxsub service install 只写自身服务文件并 reload。
func TestSboxsubServiceInstallDoesNotStart(t *testing.T) {
	baseDir := t.TempDir()
	unitDir := filepath.Join(t.TempDir(), "units")
	runner := &cliRecordingRunner{}
	restoreSboxsubServiceManager(t)
	newSboxsubServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: unitDir, Runner: runner})
	}

	if _, err := executeCommand(newSboxsubCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "service", "install"); err != nil {
		t.Fatalf("execute sboxsub service install: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(unitDir, "sboxsub.service"))
	if err != nil {
		t.Fatalf("read sboxsub unit: %v", err)
	}
	if !strings.Contains(string(data), "--base-dir "+baseDir+" serve") {
		t.Fatalf("unit should start sboxsub serve with sub base dir:\n%s", data)
	}
	got := runner.joined()
	for _, want := range []string{"getent group sbox", "id -u sbox", "chown -R sbox:sbox " + baseDir, "systemctl daemon-reload"} {
		if !strings.Contains(got, want) {
			t.Fatalf("service install missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "systemctl start") {
		t.Fatalf("service install should not start service, got %q", got)
	}
}

// TestSboxsubConfigShowMasksSecrets 验证 config show 默认脱敏，show-secrets 才显示敏感字段。
func TestSboxsubConfigShowMasksSecrets(t *testing.T) {
	baseDir := writeSubConfigFixture(t, "super-secret-token")

	output, err := executeCommand(newSboxsubCommand(), "--base-dir", baseDir, "config", "show")
	if err != nil {
		t.Fatalf("execute config show: %v", err)
	}
	if strings.Contains(output, "super-secret-token") {
		t.Fatalf("config show leaked token: %s", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("config show should redact token: %s", output)
	}

	output, err = executeCommand(newSboxsubCommand(), "--base-dir", baseDir, "config", "show", "--show-secrets")
	if err != nil {
		t.Fatalf("execute config show --show-secrets: %v", err)
	}
	if !strings.Contains(output, "super-secret-token") {
		t.Fatalf("show-secrets should reveal token: %s", output)
	}
}

// TestSboxsubInitWritesCommentedConfig 验证 sboxsub init 生成带字段说明的配置文件。
func TestSboxsubInitWritesCommentedConfig(t *testing.T) {
	baseDir := t.TempDir()
	if output, err := executeCommand(newSboxsubCommand(), "--base-dir", baseDir, "init"); err != nil {
		t.Fatalf("execute init: %v\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(baseDir, "config.yaml"))
	if err != nil {
		t.Fatalf("read sub config: %v", err)
	}
	for _, want := range []string{"# listen:", "# access:", "# templates_dir:", "# managed_config:"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("sub config missing comment %q:\n%s", want, data)
		}
	}
	if output, err := executeCommand(newSboxsubCommand(), "--base-dir", baseDir, "config", "check"); err != nil {
		t.Fatalf("commented config should pass check: %v\n%s", err, output)
	}
}

// TestSboxsubInputEditUsesEditorAndValidates 验证 input edit 通过草稿编辑后写回。
func TestSboxsubInputEditUsesEditorAndValidates(t *testing.T) {
	baseDir := writeSubInputFixture(t)
	editor := writeReplaceEditor(t, "US VMess", "US Edited")

	if _, err := executeCommand(newSboxsubCommand(), "--base-dir", baseDir, "input", "edit", "edge-us.json", "--editor", editor); err != nil {
		t.Fatalf("execute input edit: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(baseDir, "inputs", "edge-us.json"))
	if err != nil {
		t.Fatalf("read edited input: %v", err)
	}
	if !strings.Contains(string(data), "US Edited") {
		t.Fatalf("input edit did not apply editor:\n%s", data)
	}
}

// TestSboxsubInputCloneRetargetsUniqueFields 验证 input clone 默认生成可通过整体验证的新 input。
func TestSboxsubInputCloneRetargetsUniqueFields(t *testing.T) {
	baseDir := writeSubInputFixture(t)

	output, err := executeCommand(newSboxsubCommand(), "--base-dir", baseDir, "input", "clone", "edge-us.json", "edge-copy.json", "--editor", "true")
	if err != nil {
		t.Fatalf("execute input clone: %v\n%s", err, output)
	}
	output, err = executeCommand(newSboxsubCommand(), "--base-dir", baseDir, "input", "validate")
	if err != nil {
		t.Fatalf("validate cloned inputs: %v\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(baseDir, "inputs", "edge-copy.json"))
	if err != nil {
		t.Fatalf("read cloned input: %v", err)
	}
	for _, want := range []string{`"source": "edge-copy"`, `"id": "edge-copy:alice:vmess-main"`, `"tag": "edge-copy-vmess-main"`, `"remark": "edge-copy US VMess"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("cloned input missing %q:\n%s", want, data)
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
	if strings.Contains(runner.joined(), "systemctl enable --now sbox-traffic-hourly.timer") {
		t.Fatalf("start should not enable traffic timers, got %q", runner.joined())
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

// TestSboxctlServiceInstallEnablesTrafficTimersDoesNotStartInstances 验证 service install 写服务和 timer 文件并启用 timer，但不启动实例服务。
func TestSboxctlServiceInstallEnablesTrafficTimersDoesNotStartInstances(t *testing.T) {
	baseDir := writeAgentFixture(t)
	unitDir := filepath.Join(t.TempDir(), "units")
	runner := &cliRecordingRunner{}
	restoreRuntimeHooks(t)
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: unitDir, Runner: runner})
	}

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "auto", "service", "install")
	if err != nil {
		t.Fatalf("execute service install: %v", err)
	}
	if !strings.Contains(output, "Traffic timers enabled.") || !strings.Contains(output, "Instance service files installed.") || !strings.Contains(output, "Service manager:") || !strings.Contains(output, "systemd") {
		t.Fatalf("service install should print resolved manager, got %q", output)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "sbox@.service")); err != nil {
		t.Fatalf("template unit should be written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unitDir, "sbox@edge-us.service")); !os.IsNotExist(err) {
		t.Fatalf("per-instance unit should not be written, stat err: %v", err)
	}
	for _, period := range service.TrafficPeriods() {
		for _, name := range []string{
			service.TrafficSystemdServiceName(period),
			service.TrafficSystemdTimerName(period),
		} {
			if _, err := os.Stat(filepath.Join(unitDir, name)); err != nil {
				t.Fatalf("traffic timer unit should be written %s: %v", name, err)
			}
		}
	}
	got := runner.joined()
	for _, want := range []string{"getent group sbox", "id -u sbox", "chown -R sbox:sbox " + baseDir, "systemctl daemon-reload"} {
		if !strings.Contains(got, want) {
			t.Fatalf("service install missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "systemctl start") {
		t.Fatalf("service install should not start service, got %q", got)
	}
	if !strings.Contains(got, "systemctl enable --now sbox-traffic-hourly.timer") {
		t.Fatalf("service install should enable traffic timers, got %q", got)
	}
}

// TestSboxctlUninstallPurgeStopsServicesAndRemovesBaseDir 验证 uninstall all --purge 停服务、删服务文件和 base-dir。
func TestSboxctlUninstallPurgeStopsServicesAndRemovesBaseDir(t *testing.T) {
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
	for _, period := range service.TrafficPeriods() {
		for _, name := range []string{
			service.TrafficSystemdServiceName(period),
			service.TrafficSystemdTimerName(period),
		} {
			if err := os.WriteFile(filepath.Join(unitDir, name), []byte("unit"), 0644); err != nil {
				t.Fatalf("write traffic unit %s: %v", name, err)
			}
		}
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
	if _, err := os.Stat(filepath.Join(unitDir, "sbox@.service")); !os.IsNotExist(err) {
		t.Fatalf("template service file should be removed, stat err: %v", err)
	}
	for _, period := range service.TrafficPeriods() {
		for _, name := range []string{
			service.TrafficSystemdServiceName(period),
			service.TrafficSystemdTimerName(period),
		} {
			if _, err := os.Stat(filepath.Join(unitDir, name)); !os.IsNotExist(err) {
				t.Fatalf("traffic service file should be removed %s, stat err: %v", name, err)
			}
		}
	}
	if _, err := os.Stat(baseDir); !os.IsNotExist(err) {
		t.Fatalf("base dir should be removed, stat err: %v", err)
	}
	got := runner.joined()
	stopIndex := strings.Index(got, "systemctl stop sbox@edge-us.service")
	disableIndex := strings.Index(got, "systemctl disable --now sbox-traffic-hourly.timer")
	if stopIndex < 0 {
		t.Fatalf("uninstall purge should stop instance service first, got %q", got)
	}
	if disableIndex < 0 {
		t.Fatalf("uninstall purge should disable traffic timer, got %q", got)
	}
	if stopIndex > disableIndex {
		t.Fatalf("instance stop should run before timer cleanup, got %q", got)
	}
	runner.calls = nil
	if _, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "uninstall", "all", "--purge"); err != nil {
		t.Fatalf("execute second uninstall purge: %v", err)
	}
	if got := runner.joined(); got != "" {
		t.Fatalf("second uninstall purge should not rerun service commands, got %q", got)
	}
}

// TestSboxctlInstallPrintsProgress 验证资源安装命令会输出阶段性进度日志。
func TestSboxctlInstallPrintsProgress(t *testing.T) {
	baseDir := writeAgentFixture(t)
	restoreResourceInstaller(t)
	newResourceInstaller = func() resourceInstallerRunner {
		return cliFakeInstaller{run: func(options installer.Options) error {
			options.Progress("install: prepare sing-box")
			options.Progress("download: start sing-box.tar.gz")
			options.Progress("download: progress sing-box.tar.gz 1.0 MiB / 2.0 MiB (50%)")
			options.Progress("download: progress sing-box.tar.gz 2.0 MiB / 2.0 MiB (100%)")
			options.Progress("verify: passed sing-box.tar.gz")
			options.Progress("install: complete sing-box")
			return nil
		}}
	}

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "install", "all")
	if err != nil {
		t.Fatalf("execute install all: %v\n%s", err, output)
	}
	for _, want := range []string{
		"install: prepare sing-box",
		"download: start sing-box.tar.gz",
		"\r\033[2Kdownload: progress sing-box.tar.gz 1.0 MiB / 2.0 MiB (50%)",
		"\r\033[2Kdownload: progress sing-box.tar.gz 2.0 MiB / 2.0 MiB (100%)",
		"100%)\nverify: passed sing-box.tar.gz",
		"verify: passed sing-box.tar.gz",
		"install: complete sing-box",
		"Resource operation completed.",
		"Operation:",
		"Resource:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("install output missing %q:\n%s", want, output)
		}
	}
}

// TestSboxctlSetupOrder 验证 setup 默认依次执行 local 和 binary。
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
				t.Fatalf("local setup should run before binary setup: %v", err)
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
	for _, want := range []string{
		"systemctl daemon-reload",
		"systemctl enable --now sbox-traffic-hourly.timer",
		"systemctl enable --now sbox-traffic-monthly.timer",
		"install:all",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("setup order missing %q: %q", want, got)
		}
	}
	if strings.Index(got, "systemctl enable --now sbox-traffic-monthly.timer") > strings.Index(got, "install:all") {
		t.Fatalf("binary setup should run after local setup, got %q", got)
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

// TestTrafficExportDateFlagOutputsCSV 验证 traffic export 周期命令接受规格中的 date 参数。
func TestTrafficExportDateFlagOutputsCSV(t *testing.T) {
	for _, period := range []string{"hourly", "daily", "monthly"} {
		baseDir := writeAgentFixture(t)
		output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "traffic", "export", period, "--date", "2026-06-28", "--instance", "ALL")
		if err != nil {
			t.Fatalf("traffic export %s should parse --date and output CSV: %v\n%s", period, err, output)
		}
		if !strings.Contains(output, "instance,server,period,start_time,end_time,scope,name,direction,bytes,created_at") {
			t.Fatalf("traffic export %s missing CSV header: %s", period, output)
		}
	}
}

// TestTrafficCollectAllSkipsWhenNoTargets 验证 ALL 没有可采集实例时 timer 任务成功跳过。
func TestTrafficCollectAllSkipsWhenNoTargets(t *testing.T) {
	baseDir := writeAgentFixture(t)
	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "traffic", "collect", "hourly", "--instance", "ALL")
	if err != nil {
		t.Fatalf("traffic collect should skip empty ALL targets: %v\n%s", err, output)
	}
	for _, want := range []string{"Traffic collection skipped.", "No traffic-enabled instances found."} {
		if !strings.Contains(output, want) {
			t.Fatalf("traffic collect skip output missing %q:\n%s", want, output)
		}
	}
}

// TestTrafficShowDoesNotCreateDBWhenNoRecords 验证只读查询不会隐式创建 traffic DB。
func TestTrafficShowDoesNotCreateDBWhenNoRecords(t *testing.T) {
	baseDir := writeAgentFixture(t)
	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "traffic", "show", "hourly", "--instance", "ALL", "--date", "2026-06-28")
	if err != nil {
		t.Fatalf("traffic show hourly should succeed without DB: %v\n%s", err, output)
	}
	if !strings.Contains(output, "No traffic records found.") {
		t.Fatalf("expected empty result output, got %s", output)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "traffic", "traffic.db")); !os.IsNotExist(err) {
		t.Fatalf("traffic show should not create DB, stat err: %v", err)
	}
}

// TestReadOnlyCommandsDoNotWriteManagedOutputs 验证 validate/render/doctor 不写 runtime、publish 或 traffic 数据。
func TestReadOnlyCommandsDoNotWriteManagedOutputs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantError string
		wantText  string
	}{
		{name: "validate", args: []string{"validate"}, wantText: "Configuration validation passed."},
		{name: "render model", args: []string{"render", "model"}, wantText: `"instances"`},
		{name: "render sing-box", args: []string{"render", "sing-box", "edge-us"}, wantText: `"inbounds"`},
		{name: "render sub", args: []string{"render", "sub"}, wantText: `"input_schema": "sbox.subscription-input"`},
		{name: "doctor issue", args: []string{"doctor"}, wantError: "doctor found issues", wantText: "ISSUE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := writeAgentFixture(t)
			commandArgs := append([]string{"--base-dir", baseDir, "--service-manager", "systemd"}, tt.args...)
			output, err := executeCommand(newSboxctlCommand(), commandArgs...)
			if tt.wantError == "" && err != nil {
				t.Fatalf("read-only command failed: %v\n%s", err, output)
			}
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error %q, got %v\n%s", tt.wantError, err, output)
				}
			}
			if !strings.Contains(output, tt.wantText) {
				t.Fatalf("output missing %q:\n%s", tt.wantText, output)
			}
			assertNoManagedOutputs(t, baseDir)
		})
	}
}

// TestSboxctlE2EFakeLifecycle 覆盖 setup local/add/check/start/status/logs/stop 的 fake 端到端路径。
func TestSboxctlE2EFakeLifecycle(t *testing.T) {
	baseDir := t.TempDir()
	runner := &cliRecordingRunner{}
	checker := &cliFakeChecker{}
	restoreRuntimeHooks(t)
	newRuntimeConfigChecker = func(*rootOptions, domain.GlobalConfig) runtimeplan.ConfigChecker {
		return checker
	}
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: filepath.Join(baseDir, "units"), Runner: runner})
	}

	if output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "setup", "local", "--external-host", "proxy.example.com"); err != nil {
		t.Fatalf("setup local failed: %v\n%s", err, output)
	}
	if output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "add", "edge-us", "--no-edit"); err != nil {
		t.Fatalf("add failed: %v\n%s", err, output)
	}
	if output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "check"); err != nil {
		t.Fatalf("check failed: %v\n%s", err, output)
	}
	if output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "start"); err != nil {
		t.Fatalf("start failed: %v\n%s", err, output)
	}
	for _, args := range [][]string{
		{"status"},
		{"logs"},
		{"stop"},
	} {
		commandArgs := append([]string{"--base-dir", baseDir, "--service-manager", "systemd"}, args...)
		if output, err := executeCommand(newSboxctlCommand(), commandArgs...); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, output)
		}
	}
	if checker.calls != 1 {
		t.Fatalf("start should run sing-box check once, got %d", checker.calls)
	}
	for _, want := range []string{
		"systemctl start sbox@edge-us.service",
		"systemctl status --no-pager sbox@edge-us.service",
		"journalctl -u sbox@edge-us.service",
		"systemctl stop sbox@edge-us.service",
	} {
		if !strings.Contains(runner.joined(), want) {
			t.Fatalf("fake lifecycle missing %q:\n%s", want, runner.joined())
		}
	}
	if _, err := os.Stat(filepath.Join(baseDir, "runtime", "manifest.json")); err != nil {
		t.Fatalf("start should write runtime manifest in fake e2e: %v", err)
	}
}

// TestSboxctlConfigInstanceRestartsRunningService 验证实例配置变化且服务运行时自动刷新 runtime 并重启。
func TestSboxctlConfigInstanceRestartsRunningService(t *testing.T) {
	baseDir := writeAgentFixture(t)
	editor := filepath.Join(t.TempDir(), "append-comment.sh")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf '\\n# changed by test\\n' >> \"$1\"\n"), 0755); err != nil {
		t.Fatalf("write editor: %v", err)
	}
	runner := &cliRecordingRunner{outputs: map[string][]byte{
		"systemctl is-active sbox@edge-us.service": []byte("active\n"),
	}}
	checker := &cliFakeChecker{}
	restoreRuntimeHooks(t)
	newRuntimeConfigChecker = func(*rootOptions, domain.GlobalConfig) runtimeplan.ConfigChecker {
		return checker
	}
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: filepath.Join(baseDir, "units"), Runner: runner})
	}

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "config", "--editor", editor, "edge-us")
	if err != nil {
		t.Fatalf("config edit failed: %v\n%s", err, output)
	}
	for _, want := range []string{
		"Configuration updated.",
		"Running instance restarted automatically.",
		"edge-us",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("config output missing %q:\n%s", want, output)
		}
	}
	for _, want := range []string{
		"systemctl is-active sbox@edge-us.service",
		"systemctl restart sbox@edge-us.service",
	} {
		if !strings.Contains(runner.joined(), want) {
			t.Fatalf("config edit should run %q:\n%s", want, runner.joined())
		}
	}
	if checker.calls == 0 {
		t.Fatal("auto restart should check generated sing-box config")
	}
	if _, err := os.Stat(filepath.Join(baseDir, "runtime", "manifest.json")); err != nil {
		t.Fatalf("auto restart should write runtime manifest: %v", err)
	}
}

// TestSboxctlConfigInstanceNoChangeSkipsRestart 验证实例配置未变化时不会检查服务状态或重启。
func TestSboxctlConfigInstanceNoChangeSkipsRestart(t *testing.T) {
	baseDir := writeAgentFixture(t)
	runner := &cliRecordingRunner{outputs: map[string][]byte{
		"systemctl is-active sbox@edge-us.service": []byte("active\n"),
	}}
	restoreRuntimeHooks(t)
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: service.KindSystemd, UnitDir: filepath.Join(baseDir, "units"), Runner: runner})
	}

	output, err := executeCommand(newSboxctlCommand(), "--base-dir", baseDir, "--service-manager", "systemd", "config", "--editor", "true", "edge-us")
	if err != nil {
		t.Fatalf("config edit failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Configuration unchanged.") {
		t.Fatalf("config output should report no change:\n%s", output)
	}
	if got := runner.joined(); got != "" {
		t.Fatalf("no-change config edit should not run service commands:\n%s", got)
	}
}

// TestRootHelpShowsResponsibilities 验证根命令 help 能区分 agent 与 sub 职责。
func TestRootHelpShowsResponsibilities(t *testing.T) {
	sboxctlHelp, err := executeCommand(newSboxctlCommand(), "--help")
	if err != nil {
		t.Fatalf("execute sboxctl help: %v", err)
	}
	if !strings.Contains(sboxctlHelp, "agent") || !strings.Contains(sboxctlHelp, "instance lifecycle") {
		t.Fatalf("sboxctl help does not describe agent responsibility: %s", sboxctlHelp)
	}
	if !strings.Contains(sboxctlHelp, "Usage:") || !strings.Contains(sboxctlHelp, "Options:") || !strings.Contains(sboxctlHelp, "Show command help") {
		t.Fatalf("sboxctl help is not English enough: %s", sboxctlHelp)
	}

	sboxsubHelp, err := executeCommand(newSboxsubCommand(), "--help")
	if err != nil {
		t.Fatalf("execute sboxsub help: %v", err)
	}
	if !strings.Contains(sboxsubHelp, "subscription service") || !strings.Contains(sboxsubHelp, "does not read agent configuration") {
		t.Fatalf("sboxsub help does not describe sub responsibility: %s", sboxsubHelp)
	}
	if !strings.Contains(sboxsubHelp, "Usage:") || !strings.Contains(sboxsubHelp, "Options:") || !strings.Contains(sboxsubHelp, "Show command help") {
		t.Fatalf("sboxsub help is not English enough: %s", sboxsubHelp)
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

// restoreSboxsubServiceManager 在测试结束后恢复subscription service管理器注入点。
func restoreSboxsubServiceManager(t *testing.T) {
	t.Helper()

	original := newSboxsubServiceManager
	t.Cleanup(func() {
		newSboxsubServiceManager = original
	})
}

// writeSubConfigFixture 写入 sboxsub config show 测试所需配置。
func writeSubConfigFixture(t *testing.T, token string) string {
	t.Helper()

	baseDir := t.TempDir()
	content := fmt.Sprintf(`version: 1
listen: 127.0.0.1:3003
access:
  type: token
  token: %s
templates_dir: templates
watch_interval: 2s
watch_debounce: 300ms
managed_config:
  enabled: true
  interval: 86400
  strict: true
`, token)
	if err := os.WriteFile(filepath.Join(baseDir, "config.yaml"), []byte(content), 0640); err != nil {
		t.Fatalf("write sub config: %v", err)
	}
	return baseDir
}

// writeSubInputFixture 写入 sboxsub input 编辑测试所需文件。
func writeSubInputFixture(t *testing.T) string {
	t.Helper()

	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "inputs"), 0750); err != nil {
		t.Fatalf("mkdir inputs: %v", err)
	}
	data := []byte(`{
  "input_schema": "sbox.subscription-input",
  "input_version": 1,
  "source": "edge-us",
  "generated_at": "2026-06-28T12:00:00Z",
  "external_host": "proxy.example.com",
  "nodes": [
    {
      "id": "edge-us:alice:vmess-main",
      "user": "alice",
      "protocol": "vmess",
      "server": "proxy.example.com",
      "port": 24100,
      "tag": "edge-us-vmess-main",
      "remark": "US VMess",
      "uuid": "11111111-1111-4111-8111-111111111111",
      "network": "tcp"
    }
  ]
}
`)
	if err := os.WriteFile(filepath.Join(baseDir, "inputs", "edge-us.json"), data, 0640); err != nil {
		t.Fatalf("write sub input: %v", err)
	}
	return baseDir
}

// writeReplaceEditor 写入一个把文件内容中 old 替换为 new 的测试 editor。
func writeReplaceEditor(t *testing.T, old string, new string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "replace-editor.sh")
	script := fmt.Sprintf("#!/bin/sh\nsed 's/%s/%s/g' \"$1\" > \"$1.tmp\" && mv \"$1.tmp\" \"$1\"\n", old, new)
	if err := os.WriteFile(path, []byte(script), 0750); err != nil {
		t.Fatalf("write editor: %v", err)
	}
	return path
}

// assertNoManagedOutputs 验证只读命令没有创建运行、发布或统计输出。
func assertNoManagedOutputs(t *testing.T, baseDir string) {
	t.Helper()

	for _, target := range []string{
		filepath.Join(baseDir, "runtime", "manifest.json"),
		filepath.Join(baseDir, "runtime", "generated"),
		filepath.Join(baseDir, "publish"),
		filepath.Join(baseDir, "traffic", "traffic.db"),
	} {
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("read-only command should not create %s, stat err: %v", target, err)
		}
	}
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
	calls   []string
	outputs map[string][]byte
	errors  map[string]error
	onRun   func(string)
}

func (r *cliRecordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	r.calls = append(r.calls, call)
	if r.onRun != nil {
		r.onRun(call)
	}
	return r.outputs[call], r.errors[call]
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
