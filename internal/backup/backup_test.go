package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// TestBuildBackupIncludesOnlyConfigFiles 验证 agent backup 只包含新格式配置文件。
func TestBuildBackupIncludesOnlyConfigFiles(t *testing.T) {
	baseDir := writeAgentBackupFixture(t)
	for _, name := range []string{
		"runtime/manifest.json",
		"runtime/generated/sing-box/edge-us.json",
		"traffic/traffic.db",
		"downloads/cache.bin",
		"logs/sbox.log",
	} {
		path := filepath.Join(baseDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte("runtime data"), 0640); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	result, err := Build(baseDir, time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build backup: %v", err)
	}
	members := zipMembers(t, result.Data)
	for _, want := range []string{ManifestName, "config.yaml", "instances/edge-us.yaml"} {
		if _, ok := members[want]; !ok {
			t.Fatalf("backup missing %s, members=%v", want, memberNames(members))
		}
	}
	for _, excluded := range []string{"runtime/manifest.json", "traffic/traffic.db", "downloads/cache.bin", "logs/sbox.log"} {
		if _, ok := members[excluded]; ok {
			t.Fatalf("backup should exclude %s", excluded)
		}
	}
	sum := sha256.Sum256(members["config.yaml"])
	if got := result.Manifest.FilesSHA256["config.yaml"]; got != hex.EncodeToString(sum[:]) {
		t.Fatalf("manifest hash mismatch: %s", got)
	}
}

// TestReadRejectsUnsafeBackupMember 验证导入前拒绝路径穿越成员。
func TestReadRejectsUnsafeBackupMember(t *testing.T) {
	backupPath := writeBackupZip(t, map[string][]byte{
		ManifestName: []byte(`{"backup_schema":"sbox.agent-backup","backup_version":1,"generated_at":"2026-06-28T12:00:00Z","files_sha256":{"config.yaml":"` + strings.Repeat("0", 64) + `"}}`),
		"../evil":    []byte("bad"),
	})
	_, _, err := Read(backupPath)
	if err == nil {
		t.Fatal("expected unsafe path error")
	}
	if !strings.Contains(err.Error(), "路径") && !strings.Contains(err.Error(), "成员") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestImportRejectsWrongPackages 验证 import 拒绝订阅 bundle 和 hash 错误包。
func TestImportRejectsWrongPackages(t *testing.T) {
	t.Run("subscription bundle", func(t *testing.T) {
		path := writeBackupZip(t, map[string][]byte{
			"manifest.json": []byte(`{"bundle_schema":"sbox.sub-bundle"}`),
		})
		_, err := Import(t.TempDir(), path, true)
		if err == nil {
			t.Fatal("expected subscription bundle rejection")
		}
		if !strings.Contains(err.Error(), "订阅 bundle") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("hash mismatch", func(t *testing.T) {
		files := validBackupFiles(t)
		manifest := validManifestForFiles(t, files)
		manifest.FilesSHA256["config.yaml"] = strings.Repeat("0", 64)
		files[ManifestName] = mustMarshalManifest(t, manifest)
		path := writeBackupZip(t, files)
		_, err := Import(t.TempDir(), path, true)
		if err == nil {
			t.Fatal("expected hash mismatch")
		}
		if !strings.Contains(err.Error(), "hash mismatch") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestImportRequiresForceForExistingConfig 验证未传 force 时不得覆盖已有配置。
func TestImportRequiresForceForExistingConfig(t *testing.T) {
	path := writeBackupZip(t, validZipMembers(t))
	target := writeAgentBackupFixture(t)

	_, err := Import(target, path, false)
	if err == nil {
		t.Fatal("expected force error")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestImportRejectsInstancesPathOutsideBase 验证备份配置不得把 instance 写出 base dir。
func TestImportRejectsInstancesPathOutsideBase(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside")
	files := validBackupFiles(t)
	files["config.yaml"] = []byte(fmt.Sprintf(`version: 1
external_host: proxy.example.com
paths:
  instances: %s
`, outside))
	files[ManifestName] = mustMarshalManifest(t, validManifestForFiles(t, files))
	path := writeBackupZip(t, files)

	_, err := Import(t.TempDir(), path, true)
	if err == nil {
		t.Fatal("expected outside base-dir path error")
	}
	if !strings.Contains(err.Error(), "base dir") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside path should not be created, stat err: %v", err)
	}
}

// TestImportKeepsConfigWhenInstancePrepareFails 验证导入预写失败时不会替换旧 config。
func TestImportKeepsConfigWhenInstancePrepareFails(t *testing.T) {
	path := writeBackupZip(t, validZipMembers(t))
	target := t.TempDir()
	oldConfig := []byte("version: 1\nexternal_host: old.example.com\n")
	if err := os.WriteFile(filepath.Join(target, "config.yaml"), oldConfig, 0640); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "instances"), []byte("not a directory"), 0640); err != nil {
		t.Fatalf("write blocking instances path: %v", err)
	}

	_, err := Import(target, path, true)
	if err == nil {
		t.Fatal("expected instance prepare failure")
	}
	got, readErr := os.ReadFile(filepath.Join(target, "config.yaml"))
	if readErr != nil {
		t.Fatalf("read old config: %v", readErr)
	}
	if string(got) != string(oldConfig) {
		t.Fatalf("config should remain unchanged:\n%s", got)
	}
}

func writeAgentBackupFixture(t *testing.T) string {
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
	if err := os.WriteFile(filepath.Join(baseDir, "instances", "edge-us.yaml"), validInstanceYAML(), 0640); err != nil {
		t.Fatalf("write instance: %v", err)
	}
	return baseDir
}

func validBackupFiles(t *testing.T) map[string][]byte {
	t.Helper()

	return map[string][]byte{
		"config.yaml":            []byte("version: 1\nexternal_host: proxy.example.com\n"),
		"instances/edge-us.yaml": validInstanceYAML(),
	}
}

func validZipMembers(t *testing.T) map[string][]byte {
	t.Helper()

	files := validBackupFiles(t)
	files[ManifestName] = mustMarshalManifest(t, validManifestForFiles(t, files))
	return files
}

func validManifestForFiles(t *testing.T, files map[string][]byte) domain.BackupManifest {
	t.Helper()

	manifest := domain.BackupManifest{
		BackupSchema:  "sbox.agent-backup",
		BackupVersion: 1,
		GeneratedAt:   "2026-06-28T12:00:00Z",
		FilesSHA256:   make(map[string]string, len(files)),
	}
	for name, data := range files {
		if name == ManifestName {
			continue
		}
		sum := sha256.Sum256(data)
		manifest.FilesSHA256[name] = hex.EncodeToString(sum[:])
	}
	return manifest
}

func mustMarshalManifest(t *testing.T, manifest domain.BackupManifest) []byte {
	t.Helper()

	data, err := marshalStable(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return data
}

func writeBackupZip(t *testing.T, members map[string][]byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "backup.zip")
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range members {
		member, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create member %s: %v", name, err)
		}
		if _, err := member.Write(data); err != nil {
			t.Fatalf("write member %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0640); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	return path
}

func zipMembers(t *testing.T, data []byte) map[string][]byte {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	members := make(map[string][]byte)
	for _, file := range reader.File {
		readCloser, err := file.Open()
		if err != nil {
			t.Fatalf("open member %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(readCloser)
		readCloser.Close()
		if err != nil {
			t.Fatalf("read member %s: %v", file.Name, err)
		}
		members[file.Name] = content
	}
	return members
}

func memberNames(members map[string][]byte) []string {
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	return names
}

func validInstanceYAML() []byte {
	return []byte(`name: edge-us
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
outbounds:
  - name: direct
    type: direct
route:
  default: direct
`)
}
