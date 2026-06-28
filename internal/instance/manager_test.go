package instance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/generator/singbox"
	"github.com/sunliang711/sbox-manager/internal/subscription"
)

// TestInitWritesCommentedGlobalConfig 验证 init 生成的全局配置带字段说明且仍可加载。
func TestInitWritesCommentedGlobalConfig(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(baseDir, "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	for _, want := range []string{"# version:", "# external_host:", "# paths:", "# port_ranges:"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("config missing comment %q:\n%s", want, data)
		}
	}
	if _, err := config.LoadGlobalConfig(filepath.Join(baseDir, "config.yaml"), baseDir); err != nil {
		t.Fatalf("load commented config: %v", err)
	}
}

// TestAddWritesCommentedInstanceConfig 验证新增实例配置包含模板说明注释且仍可加载。
func TestAddWritesCommentedInstanceConfig(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge-smoke", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add edge: %v", err)
	}
	path := filepath.Join(baseDir, "instances", "edge-smoke.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read instance: %v", err)
	}
	for _, want := range []string{"# 可用模板:", "# api:", "# inbounds:", "# 订阅字段示例:"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("instance missing comment %q:\n%s", want, data)
		}
	}
	global, err := config.LoadGlobalConfig(filepath.Join(baseDir, "config.yaml"), baseDir)
	if err != nil {
		t.Fatalf("load global: %v", err)
	}
	if _, err := config.LoadInstance(path, *global); err != nil {
		t.Fatalf("load commented instance: %v", err)
	}
}

// TestCloneAllocatesPortsWithoutMutatingSource 验证克隆重新分配端口不会污染源实例并触发自身冲突。
func TestCloneAllocatesPortsWithoutMutatingSource(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	edge, err := Add(baseDir, AddOptions{Name: "edge-smoke", Template: "edge", AllocatePorts: true})
	if err != nil {
		t.Fatalf("add edge: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "urltest-smoke", Template: "urltest", AllocatePorts: true}); err != nil {
		t.Fatalf("add urltest: %v", err)
	}
	cloned, err := Clone(baseDir, CloneOptions{Source: "edge-smoke", Target: "edge-clone", AllocatePorts: true})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if cloned.Inbounds[0].Port == edge.Inbounds[0].Port {
		t.Fatalf("clone should allocate a new port, got %d", cloned.Inbounds[0].Port)
	}
	if cloned.Inbounds[0].Subscription.Remark != "edge-clone" {
		t.Fatalf("clone subscription remark = %q, want edge-clone", cloned.Inbounds[0].Subscription.Remark)
	}

	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		t.Fatalf("load config set: %v", err)
	}
	source, ok := set.FindInstance("edge-smoke")
	if !ok {
		t.Fatal("source instance missing")
	}
	if source.Inbounds[0].Port != edge.Inbounds[0].Port {
		t.Fatalf("source port mutated: got %d want %d", source.Inbounds[0].Port, edge.Inbounds[0].Port)
	}
	generatedAt := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	inputs, err := singbox.BuildSubscriptionInputs(set.Global, set.Instances, generatedAt)
	if err != nil {
		t.Fatalf("build subscription inputs: %v", err)
	}
	if _, err := subscription.BuildBundle(inputs, generatedAt); err != nil {
		t.Fatalf("build subscription bundle: %v", err)
	}
}

// TestCloneAllocatesAPIPort 验证克隆启用 stats API 的实例时会重分配 API 端口。
func TestCloneAllocatesAPIPort(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	edge, err := Add(baseDir, AddOptions{Name: "edge-api", Template: "edge", AllocatePorts: true})
	if err != nil {
		t.Fatalf("add edge: %v", err)
	}
	path := filepath.Join(baseDir, "instances", "edge-api.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read instance: %v", err)
	}
	updated := strings.Replace(string(data), "enabled: false", "enabled: true", 1)
	if err := os.WriteFile(path, []byte(updated), 0640); err != nil {
		t.Fatalf("enable api: %v", err)
	}

	cloned, err := Clone(baseDir, CloneOptions{Source: "edge-api", Target: "edge-api-copy", AllocatePorts: true})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if cloned.API.Listen == edge.API.Listen {
		t.Fatalf("clone API listen should be reallocated, got %s", cloned.API.Listen)
	}
	if !strings.HasSuffix(cloned.API.Listen, ":10000") {
		t.Fatalf("clone API listen = %s, want first API range port 10000", cloned.API.Listen)
	}
	if _, err := config.LoadAgentConfigSet(baseDir); err != nil {
		t.Fatalf("load config set after clone: %v", err)
	}
}

// TestTemplateSubscriptionRemarksAreUnique 验证多个默认模板可直接导出订阅 bundle。
func TestTemplateSubscriptionRemarksAreUnique(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge-smoke", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add edge: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "urltest-smoke", Template: "urltest", AllocatePorts: true}); err != nil {
		t.Fatalf("add urltest: %v", err)
	}
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		t.Fatalf("load config set: %v", err)
	}
	generatedAt := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	inputs, err := singbox.BuildSubscriptionInputs(set.Global, set.Instances, generatedAt)
	if err != nil {
		t.Fatalf("build subscription inputs: %v", err)
	}
	if _, err := subscription.BuildBundle(inputs, generatedAt); err != nil {
		t.Fatalf("build subscription bundle: %v", err)
	}
}

// TestEditFileWithCommandFindsDefaultEditor 验证未指定 editor 时按预设顺序查找系统编辑器。
func TestEditFileWithCommandFindsDefaultEditor(t *testing.T) {
	tests := []struct {
		name      string
		commands  []string
		wantValue string
	}{
		{name: "uses vim first", commands: []string{"vim", "vi", "nvim", "nano"}, wantValue: "vim"},
		{name: "falls back to vi", commands: []string{"vi", "nvim", "nano"}, wantValue: "vi"},
		{name: "falls back to nvim", commands: []string{"nvim", "nano"}, wantValue: "nvim"},
		{name: "falls back to nano", commands: []string{"nano"}, wantValue: "nano"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			for _, command := range tt.commands {
				writeEditorCommand(t, binDir, command)
			}
			t.Setenv("EDITOR", "")
			t.Setenv("PATH", binDir)

			target := filepath.Join(t.TempDir(), "draft.json")
			if err := os.WriteFile(target, []byte("original"), 0640); err != nil {
				t.Fatalf("write draft: %v", err)
			}
			if err := EditFileWithCommand(target, ""); err != nil {
				t.Fatalf("edit file: %v", err)
			}
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("read draft: %v", err)
			}
			if string(data) != tt.wantValue {
				t.Fatalf("draft = %q, want %q", string(data), tt.wantValue)
			}
		})
	}
}

// writeEditorCommand 写入测试用编辑器命令，执行时把命令名写回目标文件。
func writeEditorCommand(t *testing.T, binDir string, name string) {
	t.Helper()

	path := filepath.Join(binDir, name)
	script := "#!/bin/sh\nprintf '" + name + "' > \"$1\"\n"
	if err := os.WriteFile(path, []byte(script), 0750); err != nil {
		t.Fatalf("write editor command: %v", err)
	}
}
