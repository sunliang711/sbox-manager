package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// TestStrictLoadRejectsUnknownFields 验证核心 schema 默认拒绝未知字段。
func TestStrictLoadRejectsUnknownFields(t *testing.T) {
	baseDir := t.TempDir()
	global := domain.DefaultGlobalConfig()

	tests := []struct {
		name    string
		file    string
		content string
		load    func(string) error
		schema  string
	}{
		{
			name:    "global",
			file:    "config.yaml",
			content: "version: 1\nunexpected: true\n",
			load: func(path string) error {
				_, err := LoadGlobalConfig(path, baseDir)
				return err
			},
			schema: "GlobalConfig",
		},
		{
			name: "instance",
			file: "edge-us.yaml",
			content: `
name: edge-us
inbounds:
  - name: vmess-main
    type: vmess
    port: 24000
    users:
      - name: alice
        uuid: 11111111-1111-4111-8111-111111111111
outbounds:
  - name: direct
    type: direct
route:
  default: direct
unexpected: true
`,
			load: func(path string) error {
				_, err := LoadInstance(path, global)
				return err
			},
			schema: "Instance",
		},
		{
			name:    "sub",
			file:    "sub.yaml",
			content: "version: 1\nunexpected: true\n",
			load: func(path string) error {
				_, err := LoadSubConfig(path, baseDir)
				return err
			},
			schema: "SubConfig",
		},
		{
			name:    "traffic",
			file:    "traffic.yaml",
			content: "version: 1\nunexpected: true\n",
			load: func(path string) error {
				_, err := LoadTrafficConfig(path)
				return err
			},
			schema: "TrafficConfig",
		},
		{
			name: "subscription input",
			file: "input.yaml",
			content: `
input_schema: sbox.subscription-input
input_version: 1
source: edge-us
generated_at: "2026-06-28T12:00:00+08:00"
nodes: []
unexpected: true
`,
			load: func(path string) error {
				_, err := LoadSubscriptionInput(path)
				return err
			},
			schema: "SubscriptionInput",
		},
		{
			name:    "bundle manifest",
			file:    "manifest.json",
			content: `{"bundle_schema":"sbox.sub-bundle","bundle_version":1,"source":"all","generated_at":"2026-06-28T12:00:00+08:00","inputs_sha256":{},"template_version":"builtin-v1","access":{"type":"none"},"unexpected":true}`,
			load: func(path string) error {
				_, err := LoadBundleManifest(path)
				return err
			},
			schema: "BundleManifest",
		},
		{
			name:    "backup manifest",
			file:    "backup_manifest.json",
			content: `{"backup_schema":"sbox.agent-backup","backup_version":1,"generated_at":"2026-06-28T12:00:00+08:00","files_sha256":{},"unexpected":true}`,
			load: func(path string) error {
				_, err := LoadBackupManifest(path)
				return err
			},
			schema: "BackupManifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, baseDir, tt.file, tt.content)
			err := tt.load(path)
			if err == nil {
				t.Fatal("expected unknown field error")
			}
			if !strings.Contains(err.Error(), tt.schema) {
				t.Fatalf("expected schema name %q in error: %v", tt.schema, err)
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("expected file path in error: %v", err)
			}
		})
	}
}

// TestStrictDecodeRejectsTrailingDocuments 验证严格解码拒绝多文档和尾随对象。
func TestStrictDecodeRejectsTrailingDocuments(t *testing.T) {
	baseDir := t.TempDir()

	tests := []struct {
		name    string
		file    string
		content string
		load    func(string) error
		schema  string
	}{
		{
			name:    "yaml multi document",
			file:    "config.yaml",
			content: "version: 1\n---\nunexpected: true\n",
			load: func(path string) error {
				_, err := LoadGlobalConfig(path, baseDir)
				return err
			},
			schema: "GlobalConfig",
		},
		{
			name:    "json trailing object",
			file:    "config.json",
			content: `{"version":1}{"unexpected":true}`,
			load: func(path string) error {
				_, err := LoadGlobalConfig(path, baseDir)
				return err
			},
			schema: "GlobalConfig",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, baseDir, tt.file, tt.content)
			err := tt.load(path)
			if err == nil {
				t.Fatal("expected trailing document error")
			}
			if !strings.Contains(err.Error(), tt.schema) {
				t.Fatalf("expected schema name %q in error: %v", tt.schema, err)
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("expected file path in error: %v", err)
			}
		})
	}
}

// TestLoadInstanceSupportsVMessWebSocketTLS 验证项目原生 VMess WebSocket + TLS 出站配置可严格加载。
func TestLoadInstanceSupportsVMessWebSocketTLS(t *testing.T) {
	baseDir := t.TempDir()
	global := domain.DefaultGlobalConfig()
	path := writeTempFile(t, baseDir, "usa1.yaml", `
name: usa1
inbounds:
  - name: vmess-main
    type: vmess
    port: 24000
    users:
      - name: alice
        uuid: 11111111-1111-4111-8111-111111111111
outbounds:
  - name: vmess-upstream
    type: vmess
    server: example.cc
    port: 443
    uuid: 244a79b1-522f-4f43-8d58-69c88ef732fe
    alter_id: 0
    security: auto
    tls:
      enabled: true
      server_name: example.cc
      insecure: false
      alpn: [h2, http/1.1]
    transport:
      type: ws
      path: /vmess-websocket
      headers:
        Host: example.cc
route:
  default: vmess-upstream
`)

	instance, err := LoadInstance(path, global)
	if err != nil {
		t.Fatalf("load vmess websocket tls instance: %v", err)
	}
	outbound := instance.Outbounds[0]
	if outbound.TLS.ServerName != "example.cc" || outbound.Transport.Type != "ws" || outbound.Transport.Path != "/vmess-websocket" || outbound.Transport.Headers["Host"] != "example.cc" {
		t.Fatalf("unexpected vmess outbound: %+v", outbound)
	}
}

// TestPortRangeParsing 验证端口范围字符串和对象格式。
func TestPortRangeParsing(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantErr bool
	}{
		{name: "valid", text: "24000-24999"},
		{name: "reversed", text: "24999-24000", wantErr: true},
		{name: "too low", text: "0-10", wantErr: true},
		{name: "too high", text: "1-65536", wantErr: true},
		{name: "invalid shape", text: "24000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePortRange(tt.text)
			if tt.wantErr && err == nil {
				t.Fatal("expected parse error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
		})
	}
}

// TestGlobalConfigLoadsObjectPortRangeAndNormalizesPaths 验证对象端口范围和路径规范化。
func TestGlobalConfigLoadsObjectPortRangeAndNormalizesPaths(t *testing.T) {
	baseDir := t.TempDir()
	path := writeTempFile(t, baseDir, "config.yaml", `
version: 1
port_ranges:
  inbound:
    start: 24001
    end: 24002
`)

	config, err := LoadGlobalConfig(path, baseDir)
	if err != nil {
		t.Fatalf("load global config: %v", err)
	}
	if config.PortRanges.Inbound.Start != 24001 || config.PortRanges.Inbound.End != 24002 {
		t.Fatalf("unexpected inbound port range: %+v", config.PortRanges.Inbound)
	}
	if !filepath.IsAbs(config.Paths.Generated) {
		t.Fatalf("expected generated path to be absolute: %s", config.Paths.Generated)
	}
	if !strings.HasPrefix(config.Paths.Generated, filepath.Join(baseDir, "runtime")) {
		t.Fatalf("expected generated under runtime: %s", config.Paths.Generated)
	}
}

// TestGlobalConfigLoadsDraftSuffix 验证编辑草稿可按原始配置扩展名解析。
func TestGlobalConfigLoadsDraftSuffix(t *testing.T) {
	baseDir := t.TempDir()
	path := writeTempFile(t, baseDir, "config.yaml.draft", `
version: 1
`)

	if _, err := LoadGlobalConfig(path, baseDir); err != nil {
		t.Fatalf("load global draft config: %v", err)
	}
}

// TestLoadInstancesIgnoresDraftFiles 验证 instance 目录加载会忽略新旧草稿文件。
func TestLoadInstancesIgnoresDraftFiles(t *testing.T) {
	baseDir := t.TempDir()
	global := domain.DefaultGlobalConfig()
	instancesDir := filepath.Join(baseDir, "instances")
	if err := os.MkdirAll(instancesDir, 0750); err != nil {
		t.Fatalf("mkdir instances: %v", err)
	}
	writeTempFile(t, instancesDir, "edge-us.yaml", validLoadInstanceYAML("edge-us"))
	writeTempFile(t, instancesDir, "usa4.draft.yaml", validLoadInstanceYAML("usa4"))
	writeTempFile(t, instancesDir, "usa5.yaml.draft", validLoadInstanceYAML("usa5"))

	instances, err := LoadInstances(instancesDir, global)
	if err != nil {
		t.Fatalf("load instances: %v", err)
	}
	if len(instances) != 1 || instances[0].Name != "edge-us" {
		t.Fatalf("instances = %+v", instances)
	}
}

// TestGlobalConfigRejectsNestedUnknownPortRangeField 验证端口范围对象内未知字段会失败。
func TestGlobalConfigRejectsNestedUnknownPortRangeField(t *testing.T) {
	baseDir := t.TempDir()
	path := writeTempFile(t, baseDir, "config.yaml", `
version: 1
port_ranges:
  inbound:
    start: 24001
    end: 24002
    typo: 1
`)

	_, err := LoadGlobalConfig(path, baseDir)
	if err == nil {
		t.Fatal("expected nested unknown field error")
	}
	if !strings.Contains(err.Error(), "未知字段") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

// TestPathValidationRejectsTraversalAndGeneratedOutsideRuntime 验证路径穿越和 generated 越界会失败。
func TestPathValidationRejectsTraversalAndGeneratedOutsideRuntime(t *testing.T) {
	baseDir := t.TempDir()
	traversalPath := writeTempFile(t, baseDir, "traversal.yaml", `
version: 1
paths:
  instances: ../instances
`)
	if _, err := LoadGlobalConfig(traversalPath, baseDir); err == nil {
		t.Fatal("expected traversal path error")
	}

	generatedPath := writeTempFile(t, baseDir, "generated.yaml", `
version: 1
paths:
  runtime: runtime
  generated: generated
`)
	_, err := LoadGlobalConfig(generatedPath, baseDir)
	if err == nil {
		t.Fatal("expected generated outside runtime error")
	}
	if !strings.Contains(err.Error(), "paths.generated") {
		t.Fatalf("expected generated path error, got %v", err)
	}
}

// TestFirstAvailablePort 验证端口范围内的简单分配逻辑。
func TestFirstAvailablePort(t *testing.T) {
	port, err := FirstAvailablePort(domain.PortRange{Start: 1000, End: 1002}, map[int]struct{}{
		1000: {},
	})
	if err != nil {
		t.Fatalf("find first available port: %v", err)
	}
	if port != 1001 {
		t.Fatalf("expected port 1001, got %d", port)
	}
}

// validLoadInstanceYAML 返回用于配置加载测试的最小合法 instance YAML。
func validLoadInstanceYAML(name string) string {
	return fmt.Sprintf(`
name: %s
inbounds:
  - name: vmess-main
    type: vmess
    port: 24000
    users:
      - name: alice
        uuid: 11111111-1111-4111-8111-111111111111
outbounds:
  - name: direct
    type: direct
route:
  default: direct
`, name)
}

// writeTempFile 在测试临时目录中写入文件。
func writeTempFile(t *testing.T, dir string, name string, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0640); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
