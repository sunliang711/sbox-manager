package instance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/generator/singbox"
	"github.com/sunliang711/sbox-manager/internal/subscription"
)

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
