package subscription

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// TestImportBundleRejectsUnsafeMembersWithoutWriting 验证非法 zip 成员不会写入 inputs。
func TestImportBundleRejectsUnsafeMembersWithoutWriting(t *testing.T) {
	baseDir := t.TempDir()
	writeExistingInput(t, baseDir)
	bundlePath := filepath.Join(t.TempDir(), "bad-member.zip")
	writeZip(t, bundlePath, map[string][]byte{
		"manifest.json":         []byte(`{"bundle_schema":"sbox.sub-bundle","bundle_version":1,"source":"all","generated_at":"2026-06-28T12:00:00Z","inputs_sha256":{},"template_version":"builtin-v1","access":{"type":"none"}}`),
		"inputs/edge-us.json":   []byte(validInputJSON("edge-us", "US VMess")),
		"inputs/../escape.json": []byte("{}"),
	})

	if _, err := ImportBundle(baseDir, bundlePath, true); err == nil {
		t.Fatal("expected unsafe member error")
	}
	assertExistingInputPreserved(t, baseDir)
}

// TestImportBundleRejectsHashMismatchWithoutWriting 验证 hash mismatch 不写入任何 input。
func TestImportBundleRejectsHashMismatchWithoutWriting(t *testing.T) {
	baseDir := t.TempDir()
	writeExistingInput(t, baseDir)
	inputData := []byte(validInputJSON("edge-us", "US VMess"))
	manifest := domain.BundleManifest{
		BundleSchema:    "sbox.sub-bundle",
		BundleVersion:   1,
		Source:          "all",
		GeneratedAt:     "2026-06-28T12:00:00Z",
		InputsSHA256:    map[string]string{"edge-us.json": strings.Repeat("0", 64)},
		TemplateVersion: BuiltinTemplateVersion,
		Access:          domain.AccessConfig{Type: "none"},
	}
	manifestData, err := MarshalStable(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	bundlePath := filepath.Join(t.TempDir(), "hash-mismatch.zip")
	writeZip(t, bundlePath, map[string][]byte{
		"manifest.json":       manifestData,
		"inputs/edge-us.json": inputData,
	})

	if _, err := ImportBundle(baseDir, bundlePath, true); err == nil {
		t.Fatal("expected hash mismatch error")
	}
	assertExistingInputPreserved(t, baseDir)
}

// TestBundleAccessTokenIsRejectedAndConfigUntouched 验证 bundle access.token 不会导入到 sboxsub config。
func TestBundleAccessTokenIsRejectedAndConfigUntouched(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(baseDir, 0750); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	configPath := filepath.Join(baseDir, "config.yaml")
	configData := []byte("version: 1\naccess:\n  type: token\n  token: local-token\n")
	if err := os.WriteFile(configPath, configData, 0640); err != nil {
		t.Fatalf("write config: %v", err)
	}

	inputData := []byte(validInputJSON("edge-us", "US VMess"))
	sum := sha256.Sum256(inputData)
	manifest := domain.BundleManifest{
		BundleSchema:    "sbox.sub-bundle",
		BundleVersion:   1,
		Source:          "all",
		GeneratedAt:     "2026-06-28T12:00:00Z",
		InputsSHA256:    map[string]string{"edge-us.json": hex.EncodeToString(sum[:])},
		TemplateVersion: BuiltinTemplateVersion,
		Access:          domain.AccessConfig{Type: "none", Token: "bundle-token"},
	}
	manifestData, err := MarshalStable(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	bundlePath := filepath.Join(t.TempDir(), "token.zip")
	writeZip(t, bundlePath, map[string][]byte{
		"manifest.json":       manifestData,
		"inputs/edge-us.json": inputData,
	})

	if _, err := ImportBundle(baseDir, bundlePath, true); err == nil {
		t.Fatal("expected bundle token validation error")
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != string(configData) {
		t.Fatalf("config should not be modified:\n%s", got)
	}
}

// TestImportBundleMergeReplacesInputDir 验证默认导入会整体替换合并后的 inputs 目录。
func TestImportBundleMergeReplacesInputDir(t *testing.T) {
	baseDir := t.TempDir()
	writeExistingInput(t, baseDir)
	input := domain.SubscriptionInput{
		InputSchema:  "sbox.subscription-input",
		InputVersion: 1,
		Source:       "edge-us",
		GeneratedAt:  time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		ExternalHost: "proxy.example.com",
		Nodes: []domain.SubscriptionNode{
			{
				ID:       "edge-us:bob:vmess-main",
				User:     "bob",
				Protocol: "vmess",
				Server:   "proxy.example.com",
				Port:     24101,
				Tag:      "edge-us-vmess-main",
				Remark:   "US VMess",
				UUID:     "22222222-2222-4222-8222-222222222222",
				Network:  "tcp",
			},
		},
	}
	result, err := BuildBundle([]domain.SubscriptionInput{input}, time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	bundlePath := filepath.Join(t.TempDir(), "bundle.zip")
	if err := os.WriteFile(bundlePath, result.Data, 0640); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	if _, err := ImportBundle(baseDir, bundlePath, false); err != nil {
		t.Fatalf("import bundle: %v", err)
	}
	for _, name := range []string{"old.json", "edge-us.json"} {
		if _, err := os.Stat(filepath.Join(baseDir, "inputs", name)); err != nil {
			t.Fatalf("merged input missing %s: %v", name, err)
		}
	}
}

// writeZip 写入测试 zip 文件。
func writeZip(t *testing.T, path string, members map[string][]byte) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	for name, data := range members {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create member: %v", err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatalf("write member: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

// writeExistingInput 写入用于验证失败不覆盖的既有 input。
func writeExistingInput(t *testing.T, baseDir string) {
	t.Helper()

	if err := os.MkdirAll(InputsDir(baseDir), 0750); err != nil {
		t.Fatalf("mkdir inputs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "inputs", "old.json"), []byte(validInputJSON("old", "Old Node")), 0640); err != nil {
		t.Fatalf("write existing input: %v", err)
	}
}

// assertExistingInputPreserved 验证导入失败后既有 input 未被替换。
func assertExistingInputPreserved(t *testing.T, baseDir string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(baseDir, "inputs", "old.json"))
	if err != nil {
		t.Fatalf("existing input should remain: %v", err)
	}
	if !strings.Contains(string(data), "Old Node") {
		t.Fatalf("existing input changed: %s", data)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "inputs", "edge-us.json")); !os.IsNotExist(err) {
		t.Fatalf("new input should not be written, stat err: %v", err)
	}
}

// validInputJSON 返回一个合法订阅 input JSON。
func validInputJSON(source string, remark string) string {
	input := domain.SubscriptionInput{
		InputSchema:  "sbox.subscription-input",
		InputVersion: 1,
		Source:       source,
		GeneratedAt:  time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		ExternalHost: "proxy.example.com",
		Nodes: []domain.SubscriptionNode{
			{
				ID:       source + ":alice:vmess-main",
				User:     "alice",
				Protocol: "vmess",
				Server:   "proxy.example.com",
				Port:     24100,
				Tag:      source + "-vmess-main",
				Remark:   remark,
				Region:   "US",
				UUID:     "11111111-1111-4111-8111-111111111111",
				Network:  "tcp",
			},
		},
	}
	data, _ := MarshalStable(input)
	return string(data)
}
