package instance

import (
	"os"
	"path/filepath"
	"testing"
)

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
